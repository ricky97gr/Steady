"""财务数据采集器测试（mock akshare）"""
from datetime import date

import pandas as pd

from app.collectors import finance as fin_mod
from app.collectors.finance import FinanceCollector, build_rows, quarter_ends
from tests.helpers import multi_values


def make_yjbb():
    return pd.DataFrame([
        {"股票代码": "600519", "股票简称": "贵州茅台", "净资产收益率": 15.79,
         "净利润-同比增长": 12.3, "营业总收入-同比增长": 8.9,
         "销售毛利率": 91.5, "所处行业": "白酒",
         "最新公告日期": pd.Timestamp("2026-08-06")},
        {"股票代码": "000001", "股票简称": "平安银行", "净资产收益率": None,
         "净利润-同比增长": None, "营业总收入-同比增长": None,
         "销售毛利率": None, "所处行业": "银行",
         "最新公告日期": pd.Timestamp("2026-08-07")},
    ])


def make_zcfz():
    return pd.DataFrame([
        {"股票代码": "600519", "资产负债率": 20.5,
         "公告日期": pd.Timestamp("2026-08-06")},
        {"股票代码": "000001", "资产负债率": 91.2,
         "公告日期": None},
    ])


def test_quarter_ends():
    # 当前 2026-08：最近 4 期应为 20260630/20260331/20251231/20250930
    periods = quarter_ends(4)
    assert periods == ["20260630", "20260331", "20251231", "20250930"]
    assert len(quarter_ends(20)) == 20


def test_build_rows_mapping():
    rows = build_rows(make_yjbb(), make_zcfz(), "20260630")
    assert len(rows) == 2
    r0 = rows[0]
    assert r0["code"] == "600519"
    assert r0["report_date"] == date(2026, 6, 30)
    assert r0["roe"] == 15.79
    assert r0["profit_growth"] == 12.3
    assert r0["revenue_growth"] == 8.9
    assert r0["gross_margin"] == 91.5
    assert r0["debt_ratio"] == 20.5
    assert r0["announce_date"] == date(2026, 8, 6)
    assert r0["industry"] == "白酒"
    # pe/pb 留空（Sprint 4 补充）
    assert "pe" not in r0 or r0.get("pe") is None

    # 公告日缺失 → None；无资产负债行 → None
    r1 = rows[1]
    assert r1["debt_ratio"] == 91.2
    assert r1["roe"] is None


def test_build_rows_debt_ratio_key_always_present():
    """zcfz 缺失的股票：debt_ratio 键仍必须存在（None）——
    列带 Python default，multi-row INSERT 缺键会渲染 DEFAULT 被拒"""
    yjbb = make_yjbb()
    zcfz = make_zcfz().iloc[:0]  # 空资产负债表
    rows = build_rows(yjbb, zcfz, "20260630")
    assert all("debt_ratio" in r for r in rows)
    assert all(r["debt_ratio"] is None for r in rows)


def test_announce_takes_latest():
    """公告日取业绩报表与资产负债表较晚者（更保守，防未来函数）"""
    yjbb = make_yjbb()
    zcfz = make_zcfz()
    zcfz.loc[0, "公告日期"] = pd.Timestamp("2026-08-10")  # 资产负债表更晚
    rows = build_rows(yjbb, zcfz, "20260630")
    assert rows[0]["announce_date"] == date(2026, 8, 10)


def test_quarter_ends_in_progress_quarter():
    """8 月时当前季度（9 月底）未结束，应从 20260630 起算"""
    periods = quarter_ends(1)
    assert periods == ["20260630"]


class FakeSession:
    def __init__(self, known_codes=None):
        self.executed = []
        self.params = []
        self.known_codes = known_codes or {"600519", "000001"}

    def execute(self, stmt, params=None):
        self.executed.append(stmt)
        self.params.append(params)

        class _Scalars:
            def __init__(self, codes):
                self.codes = codes

            def scalars(self):
                return self

            def all(self):
                return list(self.codes)

        return _Scalars(self.known_codes)

    def commit(self):
        pass


def test_save_updates_industry_and_upserts(monkeypatch):
    from sqlalchemy.dialects.postgresql import dialect as pg_dialect

    db = FakeSession()
    rows = build_rows(make_yjbb(), make_zcfz(), "20260630")
    FinanceCollector(db).save(rows)
    # 3 次 execute：查询已有代码 + 行业回填 UPDATE + 财务 UPSERT
    assert len(db.executed) == 3
    fin_sql = str(db.executed[2].compile(dialect=pg_dialect()))
    assert "ON CONFLICT (code, report_date) DO UPDATE" in fin_sql
    # 冲突时有 WHERE 条件（announce_date 不早于库中行才覆盖）
    assert "excluded.announce_date" in fin_sql
    # 行业回填是 UPDATE（PG 的 ON CONFLICT 缺 name 会先死于 NOT NULL）
    ind_sql = str(db.executed[1].compile(dialect=pg_dialect()))
    assert "UPDATE stock_basic" in ind_sql and "industry" in ind_sql
    assert {p["code"]: p["industry"] for p in db.params[1]} == {
        "600519": "白酒", "000001": "银行"}


def test_save_skips_unknown_codes():
    """财务数据里的 B 股/北交所等非 A 股代码不入库：
    行业回填缺 name/market（ON CONFLICT 先死于 NOT NULL），
    财务指标有外键约束（stock_basic 无父行）"""
    db = FakeSession(known_codes={"600519"})
    rows = build_rows(make_yjbb(), make_zcfz(), "20260630")
    FinanceCollector(db).save(rows)
    # 查询 + 行业回填 + 财务 UPSERT
    assert len(db.executed) == 3
    assert {p["code"]: p["industry"] for p in db.params[1]} == {
        "600519": "白酒"}
    # 000001 不在 stock_basic，财务指标也被过滤（外键约束）
    fin_values = multi_values(db.executed[2])
    assert [r["code"] for r in fin_values] == ["600519"]

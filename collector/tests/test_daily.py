"""日行情采集器测试（mock akshare）"""
from datetime import date

import pandas as pd

from app.collectors import daily as daily_mod
from app.collectors.daily import (DailyCollector, build_rows, fetch_pair,
                                  normalize_sina, sina_symbol)
from tests.helpers import multi_values


def make_hist(closes=(10.0, 10.5, 11.0), start="2026-08-01"):
    dates = pd.date_range(start, periods=len(closes), freq="D")
    return pd.DataFrame({
        "日期": [d.strftime("%Y-%m-%d") for d in dates],
        "开盘": closes,
        "最高": [c + 0.5 for c in closes],
        "最低": [c - 0.5 for c in closes],
        "收盘": closes,
        "成交量": [10000, 12000, 13000],
        "成交额": [1e8, 1.2e8, 1.3e8],
    })


def test_build_rows_adj_factor_and_prev_close():
    raw = make_hist()
    hfq = make_hist(closes=(62.6, 65.7, 68.9))
    rows = build_rows("600519", raw, hfq)
    assert len(rows) == 3
    r0, r1 = rows[0], rows[1]
    # 复权因子 = 后复权收盘 / 不复权收盘
    assert r0["adj_factor"] == 6.26
    # prev_close 由序列内前一行收盘推出
    assert r1["prev_close"] == 10.0
    assert r0["prev_close"] is None
    # 日期转成 date 对象
    assert r0["trade_date"] == date(2026, 8, 1)


def test_build_rows_empty_raw():
    assert build_rows("600519", pd.DataFrame(), pd.DataFrame()) == []


def test_build_rows_missing_hfq():
    """后复权缺失时 adj_factor 为 None，但行情照常入库"""
    rows = build_rows("600519", make_hist(), pd.DataFrame())
    assert len(rows) == 3
    assert all(r["adj_factor"] is None for r in rows)


class FakeSession:
    def __init__(self):
        self.executed = []

    def execute(self, stmt):
        self.executed.append(stmt)

    def commit(self):
        pass


def test_fetch_calls_both_adjusts(monkeypatch):
    calls = []

    def fake_hist(symbol, period, start_date, end_date, adjust):
        calls.append(adjust)
        assert symbol == "600519"
        return make_hist()

    monkeypatch.setattr(daily_mod.ak, "stock_zh_a_hist", fake_hist)
    rows = DailyCollector(None).fetch("600519", "2026-08-01", "2026-08-20")
    assert calls == ["", "hfq"]
    assert len(rows) == 3


def test_sina_symbol():
    assert sina_symbol("600519") == "sh600519"
    assert sina_symbol("000001") == "sz000001"
    assert sina_symbol("300750") == "sz300750"
    assert sina_symbol("830001") == "bj830001"


def test_normalize_sina_volume_to_lots():
    df = pd.DataFrame({"date": ["2026-08-19"], "open": [10], "high": [11],
                       "low": [9], "close": [10.5], "volume": [3754751.0],
                       "amount": [4.8e9]})
    out = normalize_sina(df)
    assert out["成交量"].iloc[0] == 37548  # 股 → 手（/100）
    assert "date" not in out.columns
    assert "日期" in out.columns


def test_fetch_pair_fallback_to_sina(monkeypatch):
    """东财失败 → 自动降级新浪源，且带市场前缀"""
    from requests.exceptions import ConnectionError

    sina_calls = []

    def fake_hist(symbol, period, start_date, end_date, adjust):
        raise ConnectionError("em down")

    def fake_daily(symbol, start_date, end_date, adjust):
        sina_calls.append((symbol, adjust))
        return pd.DataFrame({"date": ["2026-08-19"], "open": [10], "high": [11],
                             "low": [9], "close": [10.5], "volume": [100.0],
                             "amount": [1e8]})

    monkeypatch.setattr(daily_mod.ak, "stock_zh_a_hist", fake_hist)
    monkeypatch.setattr(daily_mod.ak, "stock_zh_a_daily", fake_daily)
    raw, hfq = fetch_pair("600519", "20260801", "20260819")
    assert sina_calls == [("sh600519", ""), ("sh600519", "hfq")]
    assert not raw.empty


def test_save_upserts_clean_rows(monkeypatch):
    """save：质量校验（volume=0 丢弃）+ UPSERT 冲突列 (code, trade_date)"""
    from sqlalchemy.dialects.postgresql import dialect as pg_dialect

    from app.collectors.daily import DailyCollector

    db = FakeSession()
    data = [
        {"code": "600519", "trade_date": date(2026, 8, 1), "open": 10,
         "high": 11, "low": 9, "close": 10.5, "volume": 100,
         "amount": 1e8, "adj_factor": 1.0, "prev_close": None},
        {"code": "600519", "trade_date": date(2026, 8, 2), "open": 10,
         "high": 11, "low": 9, "close": 10.5, "volume": 0,
         "amount": 0, "adj_factor": 1.0, "prev_close": 10.5},
    ]
    DailyCollector(db).save(data)
    assert len(db.executed) == 1
    stmt = db.executed[0]
    values = multi_values(stmt)
    assert len(values) == 1  # volume=0 的行被丢弃
    assert values[0]["trade_date"] == date(2026, 8, 1)
    # 冲突判定列编译进 SQL
    sql = str(stmt.compile(dialect=pg_dialect()))
    assert "ON CONFLICT (code, trade_date) DO UPDATE" in sql

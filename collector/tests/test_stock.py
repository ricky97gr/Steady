"""股票列表采集器测试（mock akshare）"""
import pandas as pd
import pytest

from app.collectors import stock as stock_mod
from app.collectors.stock import StockCollector, infer_market, normalize_stock_rows
from tests.helpers import multi_values


def test_infer_market():
    assert infer_market("600519") == "SH"
    assert infer_market("688001") == "SH"
    assert infer_market("000001") == "SZ"
    assert infer_market("300001") == "SZ"
    assert infer_market("830001") == "BJ"
    assert infer_market("430001") == "BJ"


def test_normalize_stock_rows():
    df = pd.DataFrame(
        {"code": ["600519", "000001", "300750", "830001"],
         "name": ["贵州茅台", "平安银行", "宁德时代", "某退市股"]}
    )
    rows = normalize_stock_rows(df)
    assert rows[0] == {"code": "600519", "name": "贵州茅台",
                       "market": "SH", "status": "L"}
    # 退市标记
    assert rows[3]["status"] == "D"
    assert rows[3]["market"] == "BJ"


class FakeSession:
    """记录 execute 调用，验证入库语句（stmt 单参 = upsert，双参 = 原生 UPDATE）"""

    def __init__(self):
        self.executed = []

    def execute(self, stmt, params=None):
        self.executed.append(stmt)
        return None

    def commit(self):
        pass


def test_save_marks_universe(monkeypatch):
    """save：入库列表 → 标记 universe → 补全上市日期/行业"""
    called = []

    def fake_list():
        called.append("list")
        return pd.DataFrame({"code": ["600519", "000001"],
                             "name": ["贵州茅台", "平安银行"]})

    def fake_cons(symbol):
        called.append(symbol)
        return pd.DataFrame({"成分券代码": ["600519"]})

    monkeypatch.setattr(stock_mod.ak, "stock_info_a_code_name", fake_list)
    monkeypatch.setattr(stock_mod.ak, "index_stock_cons_csindex", fake_cons)
    monkeypatch.setattr(stock_mod, "fetch_list_dates",
                        lambda: {"600519": "2001-08-27"})
    monkeypatch.setattr(stock_mod, "fetch_industries",
                        lambda: {"600519": "白酒"})

    db = FakeSession()
    ok = StockCollector(db).run()
    assert ok
    # 4 次执行：upsert 列表 + upsert 股票池 + UPDATE 上市日期 + UPDATE 行业
    assert len(db.executed) == 4
    assert called == ["list", "000300", "000905"]


def test_universe_marking_values(monkeypatch):
    monkeypatch.setattr(
        stock_mod.ak, "stock_info_a_code_name",
        lambda: pd.DataFrame({"code": ["600519", "000001"],
                              "name": ["a", "b"]}),
    )
    monkeypatch.setattr(
        stock_mod.ak, "index_stock_cons_csindex",
        lambda symbol: pd.DataFrame({"成分券代码": ["600519"] if symbol == "000300" else []}),
    )
    monkeypatch.setattr(stock_mod, "fetch_list_dates", lambda: {})
    monkeypatch.setattr(stock_mod, "fetch_industries", lambda: {})
    db = FakeSession()
    StockCollector(db).run()
    # 第二次 upsert 语句的值应含 universe 标记
    values = multi_values(db.executed[1])
    by_code = {r["code"]: r for r in values}
    assert by_code["600519"]["universe"] == "hs300"
    assert by_code["000001"]["universe"] is None


def test_fetch_list(monkeypatch):
    monkeypatch.setattr(
        stock_mod.ak, "stock_info_a_code_name",
        lambda: pd.DataFrame({"code": ["600519"], "name": ["贵州茅台"]}),
    )
    rows = StockCollector(None).fetch()
    assert rows[0]["code"] == "600519"
    assert rows[0]["market"] == "SH"


def test_fetch_list_empty(monkeypatch):
    monkeypatch.setattr(
        stock_mod.ak, "stock_info_a_code_name",
        lambda: pd.DataFrame({"code": [], "name": []}),
    )
    assert StockCollector(None).fetch() == []


def test_pool_without_cons_api_failure(monkeypatch):
    """成分股接口失败时 run 应返回 False（基类重试后）"""
    monkeypatch.setattr(
        stock_mod.ak, "stock_info_a_code_name",
        lambda: pd.DataFrame({"code": ["600519"], "name": ["贵州茅台"]}),
    )
    monkeypatch.setattr(
        stock_mod.ak, "index_stock_cons_csindex",
        lambda symbol: (_ for _ in ()).throw(ConnectionError("boom")),
    )
    ok = StockCollector(FakeSession()).run()
    assert not ok

"""估值采集器测试（mock akshare）"""
from datetime import date

import pandas as pd

from app.collectors import valuation as val_mod
from app.collectors.valuation import ValuationCollector, to_rows
from tests.helpers import multi_values


def make_valuation_df():
    return pd.DataFrame(
        {
            "数据日期": ["2026-08-19", "2026-08-18", "2026-08-17"],
            "当日收盘价": [1307.88, 1297.99, None],
            "总市值": [1.63e12, 1.62e12, 1.61e12],
            "流通市值": [1.63e12, 1.62e12, 1.61e12],
            "PE(TTM)": [20.077, 19.925, None],
            "PE(静)": [19.86, 19.71, 19.70],
            "市净率": [6.507, 6.458, 6.45],
        }
    )


def test_to_rows_conversion():
    rows = to_rows("600519", make_valuation_df())
    assert len(rows) == 3
    assert rows[0] == {
        "code": "600519",
        "trade_date": date(2026, 8, 19),
        "close": 1307.88,
        "total_mv": 1.63e12,
        "float_mv": 1.63e12,
        "pe_ttm": 20.077,
        "pe_static": 19.86,
        "pb": 6.507,
    }
    # 缺失值 → None（入库为 NULL，正负判断留给因子侧）
    assert rows[2]["close"] is None
    assert rows[2]["pe_ttm"] is None


class FakeSession:
    """记录 execute 调用（单参 = upsert）"""

    def __init__(self):
        self.executed = []

    def execute(self, stmt, params=None):
        self.executed.append(stmt)
        return None

    def commit(self):
        pass


def test_run_upserts_valuation_rows(monkeypatch):
    monkeypatch.setattr(val_mod.ak, "stock_value_em",
                        lambda symbol: make_valuation_df())
    db = FakeSession()
    ok = ValuationCollector(db).run("600519")
    assert ok
    assert len(db.executed) == 1
    values = multi_values(db.executed[0])
    assert values[0]["code"] == "600519"
    assert values[0]["trade_date"] == date(2026, 8, 19)
    assert len(values) == 3

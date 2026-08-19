"""指数行情采集器测试（mock akshare）"""
from datetime import date

import pandas as pd

from app.collectors import index as idx_mod
from app.collectors.index import IndexCollector, build_rows
from tests.helpers import multi_values


def make_index_df():
    return pd.DataFrame({
        "date": ["2026-08-18", "2026-08-19"],
        "open": [4600.0, 4660.0],
        "high": [4650.0, 4674.0],
        "low": [4590.0, 4568.0],
        "close": [4640.0, 4588.0],
        "volume": [24407009000, 24407009001],
    })


def test_build_rows_and_date_filter():
    rows = build_rows("sh000300", make_index_df(),
                      start_date=date(2026, 8, 19))
    assert len(rows) == 1
    r = rows[0]
    assert r["code"] == "sh000300"
    assert r["trade_date"] == date(2026, 8, 19)
    assert r["close"] == 4588.0
    assert r["adj_factor"] is None


class FakeSession:
    def __init__(self):
        self.executed = []

    def execute(self, stmt):
        self.executed.append(stmt)

    def commit(self):
        pass


def test_save_inserts_index_stock_and_prices():
    from sqlalchemy.dialects.postgresql import dialect as pg_dialect

    db = FakeSession()
    data = build_rows("sh000300", make_index_df())
    IndexCollector(db).save(data)
    # 2 次 execute：stock_basic 伪股票 + daily_price
    assert len(db.executed) == 2
    stock_stmt = db.executed[0]
    stock_row = multi_values(stock_stmt)[0]
    assert stock_row["code"] == "sh000300"
    assert stock_row["market"] == "INDEX"
    assert stock_row["name"] == "沪深300"
    sql = str(db.executed[1].compile(dialect=pg_dialect()))
    assert "ON CONFLICT (code, trade_date) DO UPDATE" in sql

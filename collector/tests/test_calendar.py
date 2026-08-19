"""交易日历采集器测试（mock akshare）"""
from datetime import date

import pandas as pd

from app.collectors import calendar as cal_mod
from app.collectors.calendar import CalendarCollector


class FakeSession:
    def __init__(self):
        self.executed = []

    def execute(self, stmt):
        self.executed.append(stmt)

    def commit(self):
        pass


def test_fetch(monkeypatch):
    monkeypatch.setattr(
        cal_mod.ak, "tool_trade_date_hist_sina",
        lambda: pd.DataFrame({"trade_date": ["2026-08-19", "2026-08-20"]}),
    )
    rows = CalendarCollector(None).fetch()
    assert len(rows) == 2
    assert rows[0]["cal_date"] == date(2026, 8, 19)
    assert rows[0]["is_open"] is True
    assert rows[0]["exchange"] == "SSE"


def test_save_upserts_on_cal_date(monkeypatch):
    from sqlalchemy.dialects.postgresql import dialect as pg_dialect

    db = FakeSession()
    data = [{"cal_date": date(2026, 8, 19), "is_open": True, "exchange": "SSE"}]
    CalendarCollector(db).save(data)
    assert len(db.executed) == 1
    sql = str(db.executed[0].compile(dialect=pg_dialect()))
    assert "ON CONFLICT (cal_date) DO UPDATE" in sql

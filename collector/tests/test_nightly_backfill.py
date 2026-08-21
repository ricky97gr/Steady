"""夜间自动回填任务测试：交易日门控 + 调用参数（monkeypatch 采集器与 session）

job_nightly_backfill 在函数内 `from app.collectors.backfill import BackfillJob`，
故 patch 源模块 app.collectors.backfill.BackfillJob（而非 app.tasks.BackfillJob）。
"""
from datetime import date

from app.collectors import backfill as bf_mod

import app.tasks as tasks
from app.config import BACKFILL_START


class _ScalarResult:
    def __init__(self, value):
        self._value = value

    def scalar(self):
        return self._value


class FakeDB:
    """仅实现 job_nightly_backfill 用到的 execute().scalar()"""

    def __init__(self, is_open):
        self._is_open = is_open

    def execute(self, stmt):
        return _ScalarResult(self._is_open)


def run(is_open, monkeypatch):
    calls = []

    class FakeBackfill:
        def __init__(self, db):
            self.db = db

        def daily(self, start, end):
            calls.append(("daily", start, end))

        def valuation(self):
            calls.append(("valuation",))

    monkeypatch.setattr(bf_mod, "BackfillJob", FakeBackfill)
    monkeypatch.setattr(tasks, "get_session", lambda: FakeDB(is_open))
    tasks.job_nightly_backfill()
    return calls


def test_skip_non_trading_day(monkeypatch):
    assert run(False, monkeypatch) == []


def test_skip_missing_calendar_row(monkeypatch):
    # 日历无今日记录（scalar 返回 None）→ 保守跳过，避免无谓拉取
    assert run(None, monkeypatch) == []


def test_runs_on_trading_day(monkeypatch):
    calls = run(True, monkeypatch)
    assert len(calls) == 2, calls
    kind, start, end = calls[0]
    assert kind == "daily"
    assert start == date.fromisoformat(BACKFILL_START.replace("-", ""))
    assert end == date.today()
    assert calls[1] == ("valuation",)

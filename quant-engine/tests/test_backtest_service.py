"""回测任务队列服务测试：claim_job 原子领取 + 报告带净值序列"""
from datetime import date

import pytest
from sqlalchemy import select

from app.backtest_service import claim_job
from app.models.tables import BacktestJob
from tests.test_backtest import db, run_engine  # noqa: F401 复用 sqlite 夹具


def test_report_includes_nav_series(db):
    """_generate_report 带出净值序列（date/nav/benchmark 三字段，长度=交易日数）"""
    returns, report = run_engine(db)
    series = report["nav_series"]
    assert len(series) == len(returns)
    assert series == returns  # 与内存 daily_returns 原样一致
    assert set(series[0].keys()) == {"date", "nav", "benchmark"}


# ---- claim_job（SQLite 支持 UPDATE...RETURNING，无需 Postgres）----


def _job(jid: int, **kw):
    # SQLite 对 BIGINT 主键不自增，测试显式给 id
    return BacktestJob(
        id=jid,
        strategy_name=kw.get("strategy_name", "multi_factor"),
        start_date=kw.get("start_date", date(2026, 8, 10)),
        end_date=kw.get("end_date", date(2026, 8, 17)),
        top_n=kw.get("top_n", 2),
        status=kw.get("status", "pending"),
    )


def test_claim_job_atomic(db):
    """claim 返回 pending 任务并原子置为 running；再次 claim 不重复领取"""
    db.add(_job(1))
    db.add(_job(2, status="done"))
    db.commit()

    claimed = claim_job(db)
    assert claimed is not None
    assert claimed.status == "running"

    # 库中状态已变更（原子），done 不参与领取，无剩余 pending → None
    db.expire_all()
    assert db.execute(select(BacktestJob).where(BacktestJob.id == claimed.id)).scalar_one().status == "running"
    assert claim_job(db) is None


def test_claim_job_none_when_no_pending(db):
    db.add(_job(1, status="running"))
    db.add(_job(2, status="failed"))
    db.commit()
    assert claim_job(db) is None

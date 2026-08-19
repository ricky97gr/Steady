"""回测任务服务：任务队列（backtest_job → backtest_result）

消费方：
- tasks.py 每 5 分钟 claim pending 任务 → run_and_save（自动闭环）
- cli.py backtest --save 提交后同步执行（手动/测试）

幂等：同 (strategy_name, start_date, end_date, top_n) 唯一（uq_backtest_job），
重复提交返回已有任务；failed 任务重新提交会重置为 pending 重跑。
"""
import json
import logging
from datetime import date, datetime

from sqlalchemy import select, update
from sqlalchemy.dialects.postgresql import insert as pg_insert

from app.db import upsert
from app.models.tables import BacktestJob, BacktestResult

logger = logging.getLogger("backtest_service")

FACTOR_NAMES = ["ma_trend", "macd_signal", "pe_ratio", "pb_ratio",
                "roe_quality", "debt_risk"]


def build_replay_strategy(db, top_n: int | None = None):
    """构建内存重放策略（权重来自 factor_definition，参数来自 strategy 表）"""
    from sqlalchemy import select

    from app.backtest.replay import ReplayStrategy
    from app.models.tables import FactorDefinition, Strategy as StrategyModel

    defs = list(db.execute(
        select(FactorDefinition).where(FactorDefinition.name.in_(FACTOR_NAMES))
    ).scalars())
    weights = {d.name: float(d.weight) for d in defs}
    categories = {d.name: d.category for d in defs}

    params = {}
    row = db.execute(select(StrategyModel).where(
        StrategyModel.name == "multi_factor")).scalar()
    if row is not None and row.params:
        params = dict(row.params)
    if top_n:
        params["top_n"] = top_n
    return ReplayStrategy(db, params, weights, categories)


def create_job(db, start: date, end: date, top_n: int = 20) -> BacktestJob:
    """创建回测任务（幂等：同参数已存在直接返回；failed 重置为 pending 重跑）"""
    stmt = pg_insert(BacktestJob).values(
        strategy_name="multi_factor", start_date=start, end_date=end, top_n=top_n,
    ).on_conflict_do_update(
        index_elements=["strategy_name", "start_date", "end_date", "top_n"],
        set_={"status": "pending", "error": None, "finished_at": None},
        where=(BacktestJob.status == "failed"),
    )
    db.execute(stmt)
    db.commit()
    return db.execute(select(BacktestJob).where(
        BacktestJob.strategy_name == "multi_factor",
        BacktestJob.start_date == start,
        BacktestJob.end_date == end,
        BacktestJob.top_n == top_n,
    )).scalar_one()


def claim_job(db) -> BacktestJob | None:
    """原子领取一个 pending 任务（UPDATE ... WHERE status='pending' RETURNING）"""
    # 注：SQLAlchemy 2.0 的 update() 不支持 order_by；多任务并发的极端场景下
    # 领取顺序不保证，但消费循环会一轮全部处理，顺序无关紧要
    row = db.execute(
        update(BacktestJob)
        .where(BacktestJob.status == "pending")
        .values(status="running")
        .returning(BacktestJob)
    ).first()
    if row is None:
        return None
    db.commit()
    return row[0]


def run_and_save(db, job: BacktestJob):
    """执行回测并把结果写入 backtest_result；失败置 failed + error（不 panic）"""
    from app.backtest.engine import BacktestEngine

    try:
        strategy = build_replay_strategy(db, job.top_n)
        engine = BacktestEngine(strategy, str(job.start_date), str(job.end_date), db=db)
        report = engine.run()
        nav_series = report.pop("nav_series", [])
        p = report.get("portfolio", {})
        upsert(db, BacktestResult, [{
            "job_id": job.id,
            "total_return": p.get("total_return"),
            "annualized_return": p.get("annualized_return"),
            "max_drawdown": p.get("max_drawdown"),
            "sharpe": p.get("sharpe"),
            "trading_days": report.get("trading_days"),
            "final_value": report.get("final_value"),
            "trades": report.get("trades"),
            "positions": report.get("positions"),
            "benchmark_return": report.get("benchmark", {}).get("total_return"),
            "excess_return": report.get("excess_return"),
            "nav": nav_series,  # JSONB 列直接传 list，SQLAlchemy JSON 类型自动序列化
        }], conflict_cols=["job_id"], update_cols=[
            "total_return", "annualized_return", "max_drawdown", "sharpe",
            "trading_days", "final_value", "trades", "positions",
            "benchmark_return", "excess_return", "nav",
        ])
        db.execute(update(BacktestJob).where(BacktestJob.id == job.id).values(
            status="done", finished_at=datetime.now()))
        db.commit()
        logger.info("回测任务 %s 完成（%s ~ %s，%s 个交易日，总收益 %+.2%%）",
                    job.id, job.start_date, job.end_date, report.get("trading_days"),
                    (p.get("total_return") or 0) * 100)
    except Exception as exc:
        db.rollback()
        logger.exception("回测任务 %s 失败", job.id)
        db.execute(update(BacktestJob).where(BacktestJob.id == job.id).values(
            status="failed", error=str(exc)[:500], finished_at=datetime.now()))
        db.commit()


def consume_pending():
    """领取并执行所有 pending 任务（每 5 分钟调用一次）"""
    from app.db import get_session

    db = get_session()
    try:
        while True:
            job = claim_job(db)
            if job is None:
                return
            run_and_save(db, job)
    finally:
        db.close()

"""Quant Engine 定时任务（批处理，无 HTTP 服务）

时间表（docs §7.4）：19:00 计算因子 | 19:30 生成策略信号 | 21:00 日报（Sprint 6）
"""
import logging
from collections import Counter
from datetime import date

from apscheduler.schedulers.blocking import BlockingScheduler
from sqlalchemy import func, select

from app.db import get_session, upsert
from app.models.tables import DailyPrice, StockBasic, StrategySignal

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
)

logger = logging.getLogger("tasks")


def latest_trade_date(db) -> date | None:
    """最近一个已有行情数据的交易日（跳过指数伪股票）"""
    return db.execute(
        select(func.max(DailyPrice.trade_date))
        .where(DailyPrice.code.not_like("sh%"))
    ).scalar()


def market_ready(db, td: date) -> bool:
    """行情就绪检查：当日股票池有 bar 的比例 >= 90%，不足则跳过
    （16:30 行情同步刚完成，正常应全覆盖；比例低说明同步失败或回填未完成）"""
    pool = db.execute(
        select(StockBasic.code).where(StockBasic.universe.in_(("hs300", "zz500")))
    ).scalars().all()
    if not pool:
        return False
    with_bar = db.execute(
        select(DailyPrice.code).where(
            DailyPrice.trade_date == td, DailyPrice.code.in_(pool))
    ).scalars().all()
    return len(with_bar) / len(pool) >= 0.9


def job_calc_factors():
    """19:00 计算因子并写入 factor_value 表"""
    db = get_session()
    try:
        td = latest_trade_date(db)
        if td is None:
            logger.warning("无行情数据，跳过因子计算")
            return
        if not market_ready(db, td):
            logger.warning("%s 行情就绪比例不足，跳过因子计算", td)
            return
        from app.factor_service import compute_and_store

        compute_and_store(db, td)
    except Exception:
        logger.exception("因子计算任务失败")
    finally:
        db.close()


def generate_signals(db, td: date) -> int:
    """运行 multi_factor 策略并把信号写入 strategy_signal（幂等 upsert）；
    返回写入行数（CLI 与定时任务共用）"""
    from app.models.tables import Strategy as StrategyModel
    from app.strategies.multi_factor import MultiFactorStrategy

    row = db.execute(
        select(StrategyModel).where(
            StrategyModel.name == "multi_factor",
            StrategyModel.status == "active")
    ).scalar()
    if row is None:
        raise RuntimeError("策略 multi_factor 未配置（strategy 表）")
    config = dict(row.params or {})
    config["db"] = db
    signals = MultiFactorStrategy(config).run(str(td))
    if not signals:
        return 0
    rows = [
        {"strategy_name": "multi_factor", "code": s.code,
         "trade_date": td, "score": s.score,
         "action": s.action, "reason": s.reason}
        for s in signals
    ]
    upsert(db, StrategySignal, rows,
           conflict_cols=["strategy_name", "code", "trade_date"],
           update_cols=["score", "action", "reason"])
    counts = Counter(r["action"] for r in rows)
    logger.info("策略信号生成完成 %s：%s 条（%s）", td, len(rows), dict(counts))
    return len(rows)


def job_generate_signals():
    """19:30 运行多因子策略，信号写入 strategy_signal 表（幂等 upsert）"""
    db = get_session()
    try:
        td = latest_trade_date(db)
        if td is None:
            logger.warning("无行情数据，跳过策略信号")
            return
        n = generate_signals(db, td)
        if n == 0:
            logger.warning("%s 无信号输出（可能因子数据未就绪）", td)
    except Exception:
        logger.exception("策略信号任务失败")
    finally:
        db.close()


def job_daily_report():
    """汇总当日结果生成日报（Sprint 6 实现）"""
    logger.info("生成日报任务触发（待 Sprint 6 实现）")


def job_consume_backtests():
    """回测任务消费者：每 5 分钟领取 pending 任务并落库（APScheduler 线程池执行，
    与 19:00/19:30 任务并行不冲突——回测只读因子/行情，不写 factor_value）"""
    from app.backtest_service import consume_pending

    try:
        consume_pending()
    except Exception:
        logger.exception("回测任务消费失败")


if __name__ == "__main__":
    scheduler = BlockingScheduler()

    scheduler.add_job(job_calc_factors, "cron", hour=19, minute=0)
    scheduler.add_job(job_generate_signals, "cron", hour=19, minute=30)
    scheduler.add_job(job_daily_report, "cron", hour=21, minute=0)
    scheduler.add_job(job_consume_backtests, "interval", minutes=5)

    logger.info("quant-engine 调度器启动，等待定时任务...")
    scheduler.start()

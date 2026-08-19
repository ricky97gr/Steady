"""Quant Engine 定时任务（批处理，无 HTTP 服务）

时间表：19:00 计算因子 | 19:30 生成策略信号 | 21:00 生成日报
"""
import logging

from apscheduler.schedulers.blocking import BlockingScheduler

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
)

logger = logging.getLogger("tasks")


def job_calc_factors():
    """计算因子并写入 factor_value 表（Sprint 4 实现）"""
    logger.info("计算因子任务触发（待 Sprint 4 实现）")


def job_generate_signals():
    """运行多因子策略，信号写入 strategy_signal 表（Sprint 4 实现）"""
    logger.info("生成策略信号任务触发（待 Sprint 4 实现）")


def job_daily_report():
    """汇总当日结果生成日报（Sprint 6 实现）"""
    logger.info("生成日报任务触发（待 Sprint 6 实现）")


if __name__ == "__main__":
    scheduler = BlockingScheduler()

    scheduler.add_job(job_calc_factors, "cron", hour=19, minute=0)
    scheduler.add_job(job_generate_signals, "cron", hour=19, minute=30)
    scheduler.add_job(job_daily_report, "cron", hour=21, minute=0)

    logger.info("quant-engine 调度器启动，等待定时任务...")
    scheduler.start()

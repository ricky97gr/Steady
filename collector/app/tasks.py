"""定时任务调度（APScheduler）

时间表（见 docs/技术准备文档.md §7.4）：
09:00 股票列表 + 交易日历 | 16:30 当日行情 | 18:00 财务数据（财报季增量）
"""
import logging

from apscheduler.schedulers.blocking import BlockingScheduler

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
)

logger = logging.getLogger("tasks")


def job_sync_stock_list():
    from app.collectors.stock import StockCollector
    from app.db import get_session

    StockCollector(get_session()).run()


def job_sync_calendar():
    from app.collectors.calendar import CalendarCollector
    from app.db import get_session

    CalendarCollector(get_session()).run()


def job_sync_daily_price():
    from app.collectors.daily import DailyCollector
    from app.db import get_session

    # TODO(Sprint 1): 遍历股票池调用 DailyCollector(code, 起始日, 今日).run()
    DailyCollector(get_session()).run()


def job_sync_finance():
    from app.collectors.finance import FinanceCollector
    from app.db import get_session

    # TODO(Sprint 1): 财报季增量更新（按 announce_date）
    FinanceCollector(get_session()).run()


if __name__ == "__main__":
    scheduler = BlockingScheduler()

    scheduler.add_job(job_sync_stock_list, "cron", hour=9, minute=0)
    scheduler.add_job(job_sync_calendar, "cron", hour=9, minute=5)
    scheduler.add_job(job_sync_daily_price, "cron", hour=16, minute=30)
    scheduler.add_job(job_sync_finance, "cron", hour=18, minute=0)

    logger.info("collector 调度器启动，等待定时任务...")
    scheduler.start()

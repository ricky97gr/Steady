"""定时任务调度（APScheduler）

时间表（见 docs/技术准备文档.md §7.4）：
09:00 股票列表 + 交易日历 | 16:30 当日行情 | 18:00 财务数据（财报季增量）
"""
import logging
import time
from datetime import date, timedelta

from apscheduler.schedulers.blocking import BlockingScheduler
from sqlalchemy import func, select

from app.config import DAILY_FALLBACK_DAYS, DAILY_SYNC_INTERVAL
from app.models.tables import DailyPrice

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
)

logger = logging.getLogger("tasks")


def get_session():
    from app.db import get_session as _get

    return _get()


def job_sync_stock_list():
    from app.collectors.stock import StockCollector

    StockCollector(get_session()).run()


def job_sync_calendar():
    from app.collectors.calendar import CalendarCollector

    CalendarCollector(get_session()).run()


def job_sync_index():
    from app.collectors.index import IndexCollector
    from app.config import index_code_list

    for symbol in index_code_list():
        IndexCollector(get_session()).run(symbol=symbol)


def _daily_sync_codes(db) -> list[str]:
    """每日同步范围：已有日行情数据的股票（增量更新）

    无历史数据的股票不在每日同步内补 30 天，避免污染 backfill 的
    断点判断（covered 按 min(trade_date) 判定），完整历史由
    python -m app.collectors.backfill 负责。
    """
    with_data = db.execute(
        select(DailyPrice.code).where(DailyPrice.code.not_like("sh%")).distinct()
    ).scalars().all()
    return sorted(with_data)


def job_sync_daily_price():
    """16:30 同步当日行情：股票池 + 已有数据，逐只增量"""
    from app.collectors.daily import DailyCollector

    db = get_session()
    codes = _daily_sync_codes(db)
    end = date.today()
    logger.info("每日行情同步：%s 只股票", len(codes))
    ok = fail = 0
    for code in codes:
        max_d = db.execute(
            select(func.max(DailyPrice.trade_date)).where(DailyPrice.code == code)
        ).scalar()
        start = max_d + timedelta(days=1) if max_d else end - timedelta(days=DAILY_FALLBACK_DAYS)
        if start > end:
            continue
        if DailyCollector(db).run(code, start, end):
            ok += 1
        else:
            fail += 1
        time.sleep(DAILY_SYNC_INTERVAL)
    logger.info("每日行情同步完成：成功 %s，失败 %s", ok, fail)


def job_sync_finance():
    """18:00 财务数据增量：最近 4 个报告期（覆盖财报季尾部披露）"""
    from app.collectors.finance import FinanceCollector, quarter_ends
    from app.config import FINANCE_SYNC_QUARTERS

    FinanceCollector(get_session()).run(
        report_periods=quarter_ends(FINANCE_SYNC_QUARTERS)
    )


if __name__ == "__main__":
    scheduler = BlockingScheduler()

    scheduler.add_job(job_sync_stock_list, "cron", hour=9, minute=0)
    scheduler.add_job(job_sync_calendar, "cron", hour=9, minute=5)
    scheduler.add_job(job_sync_index, "cron", hour=16, minute=15)
    scheduler.add_job(job_sync_daily_price, "cron", hour=16, minute=30)
    scheduler.add_job(job_sync_finance, "cron", hour=18, minute=0)

    logger.info("collector 调度器启动，等待定时任务...")
    scheduler.start()

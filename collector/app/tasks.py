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
    """16:30 同步当日行情：Tushare 主源按日全市场快照（2 次调用/天）；
    失败/未配置降级 AkShare 逐只增量"""
    from app.collectors.daily import DailyCollector, upsert_daily_rows
    from app.sources import tushare

    db = get_session()
    codes = _daily_sync_codes(db)
    end = date.today()
    pro = tushare.make_pro(db)
    if pro is not None:
        try:
            rows = tushare.daily_snapshot(pro, end, codes=codes)
            if not rows:
                raise RuntimeError("Tushare 快照为空")
            n = upsert_daily_rows(db, rows)
            logger.info("Tushare 全市场快照：%s 只入库 %s 条",
                        len({r["code"] for r in rows}), n)
            return
        except Exception as e:
            logger.warning("Tushare 行情快照失败(%s)，降级 AkShare 逐只", e)
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


def job_sync_valuation():
    """16:45 同步日度估值：Tushare 主源按日全市场快照（1 次调用/天）；
    失败/未配置降级 AkShare 逐只"""
    from app.collectors.valuation import ValuationCollector
    from app.db import upsert
    from app.models.tables import DailyValuation, StockBasic
    from app.sources import tushare

    db = get_session()
    codes = sorted(
        db.execute(
            select(StockBasic.code).where(
                StockBasic.universe.in_(("hs300", "zz500"))
            )
        ).scalars().all()
    )
    pro = tushare.make_pro(db)
    if pro is not None:
        try:
            rows = tushare.daily_basic_snapshot(pro, date.today(), codes=codes)
            if not rows:
                raise RuntimeError("Tushare 估值快照为空")
            upsert(
                db,
                DailyValuation,
                rows,
                conflict_cols=["code", "trade_date"],
                update_cols=["close", "total_mv", "float_mv",
                             "pe_ttm", "pe_static", "pb"],
            )
            logger.info("Tushare 全市场估值快照：%s 只入库", len(rows))
            return
        except Exception as e:
            logger.warning("Tushare 估值快照失败(%s)，降级 AkShare 逐只", e)
    latest = {
        code: max_d
        for code, max_d in db.execute(
            select(DailyValuation.code, func.max(DailyValuation.trade_date))
            .group_by(DailyValuation.code)
        ).all()
    }
    todo = [c for c in codes if latest.get(c) is None or latest[c] < date.today()]
    logger.info("估值同步：%s 只中 %s 只需更新", len(codes), len(todo))
    ok = fail = 0
    for code in todo:
        if ValuationCollector(db).run(code):
            ok += 1
        else:
            fail += 1
        time.sleep(DAILY_SYNC_INTERVAL)
    logger.info("估值同步完成：成功 %s，失败 %s", ok, fail)


def job_sync_hotspot():
    """08:45 市场热点快照（隔夜外盘/板块涨幅与资金流/人气榜，供早盘简报）。
    周末跳过（日历 09:05 才同步，这里用周几启发式；工作日节假日多采无害）。"""
    if date.today().weekday() >= 5:
        logger.info("周末，跳过市场热点采集")
        return
    from app.collectors.hotspot import HotspotCollector

    HotspotCollector(get_session()).run(spot_date=date.today())


def job_nightly_backfill():
    """18:05 每日缺口回填（仅交易日）：日线补未覆盖到起始日的股票；估值补滞后股票。

    断点续传天然幂等（covered_codes 按 min(trade_date) 判定），正常日只补新股/缺口
    （16:30/16:45 已同步当日行情与估值，正常日 todo 为空，分钟级）；
    首次全量仍由 python -m app.collectors.backfill 手动触发。
    交易日门控：TradeCalendar.is_open != 1（含无记录）→ 跳过，避免周末/节假日无谓拉取。
    """
    from app.collectors.backfill import BackfillJob
    from app.config import BACKFILL_START
    from app.models.tables import TradeCalendar

    db = get_session()
    is_open = db.execute(
        select(TradeCalendar.is_open).where(TradeCalendar.cal_date == date.today())
    ).scalar()
    if not is_open:
        logger.info("非交易日或日历缺失，跳过夜间回填")
        return
    start_date = date.fromisoformat(BACKFILL_START.replace("-", ""))
    BackfillJob(db).daily(start_date, date.today())
    BackfillJob(db).valuation()


if __name__ == "__main__":
    scheduler = BlockingScheduler()

    scheduler.add_job(job_sync_hotspot, "cron", hour=8, minute=45)
    scheduler.add_job(job_sync_stock_list, "cron", hour=9, minute=0)
    scheduler.add_job(job_sync_calendar, "cron", hour=9, minute=5)
    scheduler.add_job(job_sync_index, "cron", hour=16, minute=15)
    scheduler.add_job(job_sync_daily_price, "cron", hour=16, minute=30)
    scheduler.add_job(job_sync_valuation, "cron", hour=16, minute=45)
    scheduler.add_job(job_sync_finance, "cron", hour=18, minute=0)
    scheduler.add_job(job_nightly_backfill, "cron", hour=18, minute=5)

    logger.info("collector 调度器启动，等待定时任务...")
    scheduler.start()

"""采集服务手动入口（日常触发 / 回填）

用法：
    python -m app.cli sync-stock          # 股票列表 + 股票池标记
    python -m app.cli sync-calendar       # 交易日历
    python -m app.cli sync-index          # 指数行情（沪深300/中证500）
    python -m app.cli sync-daily          # 全部待同步股票当日行情（股票池 + 已有数据）
    python -m app.cli sync-daily --code 600519 [--start 20260801] [--end 20260819]
    python -m app.cli sync-finance [--quarters 4]
    python -m app.cli sync-valuation [--codes 600519,000001]
    python -m app.cli backfill [--start 20160801] [--end 20260819] [--quarters 20]
                                [--codes 600519,000001] [--dry-run]
    python -m app.cli backfill-valuation [--codes 600519,000001] [--dry-run]
"""
import argparse
import logging
import sys
from datetime import date, timedelta

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
)

logger = logging.getLogger("cli")


def _codes_for_daily_sync(db) -> list[str]:
    """每日同步范围：股票池 ∪ 已有日行情数据的股票（不含指数）"""
    from sqlalchemy import select

    from app.models.tables import DailyPrice, StockBasic

    pool = db.execute(
        select(StockBasic.code).where(
            StockBasic.universe.in_(("hs300", "zz500"))
        )
    ).scalars().all()
    with_data = db.execute(
        select(DailyPrice.code).where(DailyPrice.code.not_like("sh%")).distinct()
    ).scalars().all()
    return sorted(set(pool) | set(with_data))


def cmd_sync_daily(args):
    """增量同步日行情：逐只拉取（上次入库日 → 今天，可用 --start/--end 覆盖）"""
    import time

    from sqlalchemy import func, select

    from app.collectors.daily import DailyCollector
    from app.config import DAILY_FALLBACK_DAYS, DAILY_SYNC_INTERVAL
    from app.db import get_session
    from app.models.tables import DailyPrice

    db = get_session()
    end = date.fromisoformat(args.end) if args.end else date.today()
    if args.code:
        codes = [args.code.zfill(6)]
    else:
        codes = _codes_for_daily_sync(db)
        logger.info("每日同步范围：%s 只", len(codes))
    ok_count = fail_count = 0
    for code in codes:
        if args.start:
            start = date.fromisoformat(args.start)
        else:
            max_d = db.execute(
                select(func.max(DailyPrice.trade_date))
                .where(DailyPrice.code == code)
            ).scalar()
            start = (
                max_d + timedelta(days=1)
                if max_d else end - timedelta(days=DAILY_FALLBACK_DAYS)
            )
        if start > end:
            logger.debug("%s 已是最新，跳过", code)
            continue
        ok = DailyCollector(db).run(code, start, end)
        ok_count += ok
        fail_count += not ok
        time.sleep(DAILY_SYNC_INTERVAL)
    logger.info("每日行情同步完成：成功 %s，失败 %s", ok_count, fail_count)
    return fail_count == 0


def cmd_sync_finance(args):
    from app.collectors.finance import sync_finance
    return sync_finance(quarters=args.quarters)


def cmd_backfill(args):
    from app.collectors.backfill import run_backfill
    run_backfill(args.start, args.end, args.quarters, args.dry_run, args.codes)
    return True


def cmd_sync_valuation(args):
    """同步日度估值（跳过当日已最新的股票）"""
    from app.collectors.valuation import sync_valuation

    codes = None
    if args.codes:
        codes = [c.strip().zfill(6) for c in args.codes.split(",")]
    return sync_valuation(codes)


def cmd_backfill_valuation(args):
    """估值回填（全量拉取，断点续传按 max(trade_date) 判定）"""
    from app.collectors.backfill import BackfillJob
    from app.config import BACKFILL_BATCH_SIZE, RATE_LIMIT_SECONDS
    from app.db import get_session

    job = BackfillJob(get_session(), rate_limit=RATE_LIMIT_SECONDS,
                      batch_size=BACKFILL_BATCH_SIZE, dry_run=args.dry_run)
    codes = [c.strip().zfill(6) for c in args.codes.split(",")] if args.codes else None
    job.valuation(codes)
    return True


def main():
    parser = argparse.ArgumentParser(prog="quant-collector")
    sub = parser.add_subparsers(dest="cmd", required=True)

    sub.add_parser("sync-stock", help="股票列表 + 股票池标记")
    sub.add_parser("sync-calendar", help="交易日历")
    sub.add_parser("sync-index", help="指数行情")

    p_daily = sub.add_parser("sync-daily", help="增量同步日行情")
    p_daily.add_argument("--code", help="只同步指定股票")
    p_daily.add_argument("--start", help="起始日期 YYYYMMDD")
    p_daily.add_argument("--end", help="结束日期 YYYYMMDD")

    p_fin = sub.add_parser("sync-finance", help="增量同步财务数据")
    p_fin.add_argument("--quarters", type=int, default=4, help="报告期数")

    p_back = sub.add_parser("backfill", help="历史回填（分批限速、断点续传）")
    p_back.add_argument("--start", help="起始日期 YYYYMMDD")
    p_back.add_argument("--end", help="结束日期 YYYYMMDD")
    p_back.add_argument("--quarters", type=int, help="财务报告期数")
    p_back.add_argument("--codes", help="只回填指定代码（逗号分隔）")
    p_back.add_argument("--dry-run", action="store_true")

    p_va = sub.add_parser("sync-valuation", help="增量同步日度估值")
    p_va.add_argument("--codes", help="只同步指定代码（逗号分隔）")

    p_bv = sub.add_parser("backfill-valuation", help="估值回填（全量拉取）")
    p_bv.add_argument("--codes", help="只回填指定代码（逗号分隔）")
    p_bv.add_argument("--dry-run", action="store_true")

    args = parser.parse_args()

    try:
        if args.cmd == "sync-stock":
            from app.collectors.stock import sync_stock_list
            ok = sync_stock_list()
        elif args.cmd == "sync-calendar":
            from app.collectors.calendar import sync_calendar
            ok = sync_calendar()
        elif args.cmd == "sync-index":
            from app.collectors.index import sync_index
            ok = sync_index()
        elif args.cmd == "sync-daily":
            ok = cmd_sync_daily(args)
        elif args.cmd == "sync-finance":
            ok = cmd_sync_finance(args)
        elif args.cmd == "backfill":
            ok = cmd_backfill(args)
        elif args.cmd == "sync-valuation":
            ok = cmd_sync_valuation(args)
        elif args.cmd == "backfill-valuation":
            ok = cmd_backfill_valuation(args)
        else:
            parser.error(f"未知命令: {args.cmd}")
        sys.exit(0 if ok else 1)
    except KeyboardInterrupt:
        logger.warning("中断，断点续传可安全重跑")
        sys.exit(130)


if __name__ == "__main__":
    main()

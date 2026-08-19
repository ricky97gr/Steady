"""量化引擎手动入口

用法：
    python -m app.cli factors [--date 2026-08-19]        # 手动计算因子
    python -m app.cli signals [--date 2026-08-19]        # 手动生成策略信号
    python -m app.cli backtest [--start 2025-01-01]      # 回测并打印报告
                                [--end 2026-08-20] [--top-n 20]
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


def _default_trade_date(db) -> date | None:
    """最近一个已有行情数据的交易日（与定时任务同口径）"""
    from sqlalchemy import func, select

    from app.models.tables import DailyPrice

    return db.execute(
        select(func.max(DailyPrice.trade_date))
        .where(DailyPrice.code.not_like("sh%"))
    ).scalar()


def cmd_factors(args) -> bool:
    from app.db import get_session
    from app.factor_service import compute_and_store

    db = get_session()
    td = date.fromisoformat(args.date) if args.date else _default_trade_date(db)
    if td is None:
        logger.error("无行情数据，请先同步日行情")
        return False
    stats = compute_and_store(db, td)
    logger.info("因子计算完成 %s：%s", td, stats)
    return True


def cmd_signals(args) -> bool:
    from app.db import get_session
    from app.tasks import generate_signals

    db = get_session()
    td = date.fromisoformat(args.date) if args.date else _default_trade_date(db)
    if td is None:
        logger.error("无行情数据，请先同步日行情")
        return False
    n = generate_signals(db, td)
    logger.info("策略信号完成 %s：%s 条", td, n)
    return True


def cmd_backtest(args) -> bool:
    from sqlalchemy import select

    from app.backtest.engine import BacktestEngine
    from app.backtest.replay import ReplayStrategy
    from app.db import get_session
    from app.models.tables import FactorDefinition, Strategy

    db = get_session()
    defs = list(db.execute(
        select(FactorDefinition).where(FactorDefinition.name.in_(
            ["ma_trend", "macd_signal", "pe_ratio", "pb_ratio",
             "roe_quality", "debt_risk"]))).scalars())
    weights = {d.name: float(d.weight) for d in defs}
    categories = {d.name: d.category for d in defs}

    params = {}
    row = db.execute(select(Strategy).where(Strategy.name == "multi_factor")
                     ).scalar()
    if row is not None and row.params:
        params = dict(row.params)
    if args.top_n:
        params["top_n"] = args.top_n

    strategy = ReplayStrategy(db, params, weights, categories)
    engine = BacktestEngine(strategy, args.start, args.end, db=db)
    report = engine.run()

    print("\n===== 回测报告（多因子轮动）=====")
    print(f"区间 {report['start']} ~ {report['end']}，"
          f"{report['trading_days']} 个交易日")
    p = report["portfolio"]
    print(f"期末净值 {report['final_value']:.2f}（初始 100000）")
    print(f"总收益    {p['total_return']:+.2%}")
    print(f"年化收益  {p['annualized_return']:+.2%}")
    print(f"最大回撤  {p['max_drawdown']:.2%}")
    print(f"夏普比率  {p['sharpe'] if p['sharpe'] is not None else 'N/A'}")
    if "benchmark" in report:
        b = report["benchmark"]
        print(f"基准(HS300) 总收益 {b['total_return']:+.2%}，"
              f"最大回撤 {b['max_drawdown']:.2%}")
        print(f"超额收益  {report['excess_return']:+.2%}")
    print(f"成交 {report['trades']} 笔，期末持仓 {report['positions']} 只")
    print("=" * 30)
    return True


def main():
    parser = argparse.ArgumentParser(prog="quant-engine")
    sub = parser.add_subparsers(dest="cmd", required=True)

    p_f = sub.add_parser("factors", help="手动计算因子")
    p_f.add_argument("--date", help="交易日 YYYY-MM-DD（默认最近交易日）")

    p_s = sub.add_parser("signals", help="手动生成策略信号")
    p_s.add_argument("--date", help="交易日 YYYY-MM-DD（默认最近交易日）")

    p_b = sub.add_parser("backtest", help="历史回测")
    p_b.add_argument("--start", default=(date.today() - timedelta(days=365 * 2))
                     .isoformat(), help="起始日 YYYY-MM-DD（默认近 2 年）")
    p_b.add_argument("--end", default=date.today().isoformat(),
                     help="结束日 YYYY-MM-DD")
    p_b.add_argument("--top-n", type=int, help="目标持仓数（默认取策略配置 20）")

    args = parser.parse_args()

    try:
        if args.cmd == "factors":
            ok = cmd_factors(args)
        elif args.cmd == "signals":
            ok = cmd_signals(args)
        elif args.cmd == "backtest":
            ok = cmd_backtest(args)
        else:
            parser.error(f"未知命令: {args.cmd}")
        sys.exit(0 if ok else 1)
    except KeyboardInterrupt:
        sys.exit(130)


if __name__ == "__main__":
    main()

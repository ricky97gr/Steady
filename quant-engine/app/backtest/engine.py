"""回测引擎主循环：按交易日历逐日执行，记录净值/回撤/基准对比"""
import logging
from typing import List, Optional

import pandas as pd
from sqlalchemy import select

from app.backtest.broker import Broker
from app.backtest.portfolio import Portfolio
from app.models.tables import DailyPrice, TradeCalendar
from app.strategies.base import Signal, Strategy

logger = logging.getLogger(__name__)


class BacktestEngine:
    """回测引擎：逐日跑策略 → 按信号成交（含 T+1/涨跌停/手续费）→ 净值曲线

    strategy 若实现 preload(start, end)（如 ReplayStrategy），引擎自动
    在 run() 前调用；价格取价优先走 strategy.price_at（复用预加载数据）。
    """

    def __init__(self, strategy: Strategy, start_date: str, end_date: str,
                 db=None):
        if db is None:
            from app.db import get_session
            db = get_session()
        self.db = db
        self.strategy = strategy
        self.start_date = start_date
        self.end_date = end_date
        self.portfolio = Portfolio(initial_cash=100000)
        self.broker = Broker()
        self.daily_returns = []
        self._benchmark_cache: Optional[dict] = None

    def _get_trading_dates(self) -> List[str]:
        """从 trade_calendar 取 [start, end] 区间的交易日"""
        rows = self.db.execute(
            select(TradeCalendar.cal_date)
            .where(TradeCalendar.cal_date >= self.start_date,
                   TradeCalendar.cal_date <= self.end_date,
                   TradeCalendar.is_open.is_(True))
            .order_by(TradeCalendar.cal_date)
        ).scalars().all()
        return [d.isoformat() for d in rows]

    def _get_price(self, code: str, date: str) -> Optional[float]:
        """当日真实价（不复权）；优先用策略预加载数据，回退查库"""
        if hasattr(self.strategy, "price_at"):
            return self.strategy.price_at(code, date)
        close = self.db.execute(
            select(DailyPrice.close).where(
                DailyPrice.code == code, DailyPrice.trade_date == date)
        ).scalar()
        return float(close) if close is not None else None

    def _get_prev_close(self, code: str, date: str) -> Optional[float]:
        """前一交易日收盘价，用于涨跌停判断"""
        if hasattr(self.strategy, "prev_close_at"):
            return self.strategy.prev_close_at(code, date)
        rows = self.db.execute(
            select(DailyPrice.close)
            .where(DailyPrice.code == code, DailyPrice.trade_date < date)
            .order_by(DailyPrice.trade_date.desc())
            .limit(1)
        ).scalar()
        return float(rows) if rows is not None else None

    def _get_benchmark_nav(self, date: str) -> Optional[float]:
        """沪深300 指数当日收盘（用于超额收益对比），缓存全量"""
        if self._benchmark_cache is None:
            rows = self.db.execute(
                select(DailyPrice.trade_date, DailyPrice.close)
                .where(DailyPrice.code == "sh000300")
            ).all()
            self._benchmark_cache = {
                d.isoformat(): float(c) for d, c in rows if c is not None}
        return self._benchmark_cache.get(date)

    def _calc_quantity(self, price: float) -> int:
        """等权 + 单股仓位上限，向下取整到 100 股整数倍"""
        budget = min(self.portfolio.total_value / self.strategy.top_n,
                     self.portfolio.total_value * self.strategy.max_position_pct)
        qty = int(budget / price // 100) * 100
        return qty

    def _unfreeze_t1(self, date: str):
        """T+1：上一交易日买入的份额当日可用（简化实现：全部解冻）"""
        for pos in self.portfolio.positions.values():
            pos.available_qty = pos.quantity

    def run(self) -> dict:
        """执行回测并返回报告；策略实现 preload 时先预加载"""
        preload = getattr(self.strategy, "preload", None)
        if callable(preload):
            try:
                preload(self.start_date, self.end_date)
            except TypeError:
                preload()
        dates = self._get_trading_dates()
        logger.info("回测区间 %s ~ %s，共 %s 个交易日", self.start_date,
                    self.end_date, len(dates))
        for date in dates:
            self._unfreeze_t1(date)  # T+1：解冻上一交易日买入
            signals = self.strategy.run(date)
            for signal in signals:
                self._process_signal(signal, date)
            # 记录每日净值（含基准对比）
            nav = self.portfolio.total_value
            benchmark = self._get_benchmark_nav(date)
            self.daily_returns.append({"date": date, "nav": nav,
                                       "benchmark": benchmark})
        return self._generate_report()

    def _process_signal(self, signal: Signal, date: str):
        if signal.action == "BUY":
            price = self._get_price(signal.code, date)
            if price is None:  # 停牌/数据缺失：跳过
                return
            qty = self._calc_quantity(price)
            if qty <= 0:
                return
            # 涨停检查 + 100股整手 + 资金校验（Broker 内完成）；
            # 滑点+佣金可能超出预算（top_n 全量买入时现金趋近预算），
            # 与 Go 模拟盘一致按 100 股递减重试，减到 0 放弃
            filled = 0
            while qty > 0:
                try:
                    ok = self.broker.execute_buy(
                        self.portfolio, signal.code, price, qty,
                        prev_close=self._get_prev_close(signal.code, date),
                        trade_date=date)
                except ValueError:
                    ok = False
                if ok:
                    filled = qty
                    break
                qty -= 100
            if filled > 0:
                self.portfolio.trades.append(
                    {"date": date, "code": signal.code, "action": "BUY",
                     "price": round(price, 2), "qty": filled})
        elif signal.action == "SELL":
            price = self._get_price(signal.code, date)
            pos = self.portfolio.positions.get(signal.code)
            if price is None or not pos or pos.available_qty <= 0:
                # 停牌/无持仓/T+1 当日买入不可卖：跳过
                return
            ok = self.broker.execute_sell(
                self.portfolio, signal.code, price, pos.available_qty,
                prev_close=self._get_prev_close(signal.code, date))
            if ok:
                self.portfolio.trades.append(
                    {"date": date, "code": signal.code, "action": "SELL",
                     "price": round(price, 2), "qty": pos.available_qty})

    def _generate_report(self) -> dict:
        """总收益、年化、最大回撤、Sharpe、基准对比"""
        if not self.daily_returns:
            return {"error": "无回测数据"}

        def metrics(values: list[float]) -> dict:
            s = pd.Series(values)
            rets = s.pct_change().dropna()
            total = s.iloc[-1] / s.iloc[0] - 1
            days = len(s)
            return {
                "total_return": round(float(total), 4),
                "annualized_return": round(float((1 + total) ** (252 / days) - 1), 4),
                "max_drawdown": round(float((s / s.cummax() - 1).min()), 4),
                "sharpe": round(float(rets.mean() / rets.std() * (252 ** 0.5)), 4)
                if len(rets) > 1 and rets.std() > 0 else None,
            }

        navs = [r["nav"] for r in self.daily_returns]
        report = {"start": self.daily_returns[0]["date"],
                  "end": self.daily_returns[-1]["date"],
                  "trading_days": len(navs),
                  "final_value": round(self.portfolio.total_value, 2),
                  "trades": len(self.portfolio.trades),
                  "positions": len(self.portfolio.positions),
                  "portfolio": metrics(navs)}
        bench = [r["benchmark"] for r in self.daily_returns
                 if r["benchmark"] is not None]
        if bench:
            bm = metrics(bench)
            report["benchmark"] = bm
            report["excess_return"] = round(
                report["portfolio"]["total_return"] - bm["total_return"], 4)
        # 净值序列（date/nav/benchmark，benchmark 缺失日 None）——落库/前端画图用
        report["nav_series"] = [
            {"date": r["date"], "nav": r["nav"], "benchmark": r["benchmark"]}
            for r in self.daily_returns
        ]
        return report

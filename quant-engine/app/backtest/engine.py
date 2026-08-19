"""回测引擎主循环：按交易日历逐日执行，记录净值/回撤/基准对比"""
from typing import List

from app.backtest.broker import Broker
from app.backtest.portfolio import Portfolio
from app.strategies.base import Signal, Strategy


class BacktestEngine:
    """回测引擎"""

    def __init__(self, strategy: Strategy, start_date: str, end_date: str):
        self.strategy = strategy
        self.start_date = start_date
        self.end_date = end_date
        self.portfolio = Portfolio(initial_cash=100000)
        self.broker = Broker()
        self.daily_returns = []

    def _get_trading_dates(self) -> List[str]:
        """TODO(Sprint 4): 从 trade_calendar 取 [start, end] 区间的交易日"""
        raise NotImplementedError("待 Sprint 4 实现")

    def _get_price(self, code: str, date: str) -> float:
        """TODO(Sprint 4): 从 daily_price 取当日真实价（不复权）"""
        raise NotImplementedError("待 Sprint 4 实现")

    def _get_prev_close(self, code: str, date: str) -> float:
        """TODO(Sprint 4): 取前一交易日收盘价，用于涨跌停判断"""
        raise NotImplementedError("待 Sprint 4 实现")

    def _get_benchmark_nav(self, date: str) -> float:
        """TODO(Sprint 4): 沪深300 指数同期净值，用于超额收益对比"""
        raise NotImplementedError("待 Sprint 4 实现")

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

    def run(self):
        dates = self._get_trading_dates()
        for date in dates:
            self._unfreeze_t1(date)  # T+1：解冻上一交易日买入
            signals = self.strategy.run(date)
            for signal in signals:
                self._process_signal(signal, date)
            # 记录每日净值（含基准对比）
            nav = self.portfolio.total_value
            benchmark = self._get_benchmark_nav(date)
            self.daily_returns.append({"date": date, "nav": nav, "benchmark": benchmark})
        return self._generate_report()

    def _process_signal(self, signal: Signal, date: str):
        if signal.action == "BUY":
            price = self._get_price(signal.code, date)
            qty = self._calc_quantity(price)
            # 涨停检查 + 100股整手 + 资金校验（Broker 内完成）
            self.broker.execute_buy(self.portfolio, signal.code, price, qty,
                                    prev_close=self._get_prev_close(signal.code, date),
                                    trade_date=date)
        elif signal.action == "SELL":
            price = self._get_price(signal.code, date)
            pos = self.portfolio.positions.get(signal.code)
            if not pos or pos.available_qty <= 0:  # T+1：当日买入不可卖
                return
            self.broker.execute_sell(self.portfolio, signal.code, price, pos.available_qty,
                                     prev_close=self._get_prev_close(signal.code, date))

    def _generate_report(self):
        """TODO(Sprint 4): 总收益、年化、最大回撤、Sharpe、基准对比"""
        raise NotImplementedError("待 Sprint 4 实现")

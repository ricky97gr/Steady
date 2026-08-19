"""组合管理：现金、持仓、盈亏（T+1、整手）"""
from dataclasses import dataclass, field
from typing import Dict


@dataclass
class Position:
    code: str
    quantity: int
    available_qty: int  # T+1：当日买入冻结，次一交易日解冻
    cost_price: float
    current_price: float

    @property
    def market_value(self) -> float:
        return self.quantity * self.current_price

    @property
    def profit(self) -> float:
        return self.quantity * (self.current_price - self.cost_price)


@dataclass
class Portfolio:
    initial_cash: float = 100000.0
    cash: float = field(default_factory=lambda: 100000.0)
    positions: Dict[str, Position] = field(default_factory=dict)
    trades: list = field(default_factory=list)
    last_date: str = ""  # 用于 T+1 解冻判断

    @property
    def total_value(self) -> float:
        mv = sum(p.market_value for p in self.positions.values())
        return self.cash + mv

    @property
    def total_return(self) -> float:
        return (self.total_value - self.initial_cash) / self.initial_cash

    def buy(self, code: str, price: float, qty: int, commission: float, trade_date: str = ""):
        if qty % 100 != 0:
            raise ValueError("买入数量必须为100股整数倍")
        cost = price * qty + commission
        if cost > self.cash:
            raise ValueError("资金不足")
        self.cash -= cost
        if code in self.positions:
            pos = self.positions[code]
            total_cost = pos.cost_price * pos.quantity + price * qty
            pos.quantity += qty
            pos.cost_price = total_cost / pos.quantity
            # T+1：当日买入部分冻结，由引擎在次一交易日解冻
            pos.available_qty -= 0  # 冻结体现在 available_qty 不动，解冻逻辑见 engine._unfreeze_t1
        else:
            # T+1：新建仓当日 available_qty = 0
            self.positions[code] = Position(code, qty, 0, price, price)
        self.last_date = trade_date

    def sell(self, code: str, price: float, qty: int, commission: float, tax: float):
        if code not in self.positions:
            raise ValueError("无持仓")
        pos = self.positions[code]
        if qty > pos.available_qty:
            raise ValueError("可用持仓不足（T+1 限制）")
        proceeds = price * qty - commission - tax
        self.cash += proceeds
        pos.quantity -= qty
        pos.available_qty -= qty
        if pos.quantity == 0:
            del self.positions[code]

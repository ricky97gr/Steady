"""策略基类：所有策略继承统一接口，便于新增/并行/回测对比"""
import logging
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import List, Optional

import pandas as pd


@dataclass
class Signal:
    """策略信号"""

    code: str
    score: float
    action: str  # BUY / SELL / HOLD
    reason: str = ""


class Strategy(ABC):
    """策略基类

    标准执行流程（run）：
        prepare(trade_date) → calculate() → generate_signal()
    """

    name: str = "base"
    description: str = ""

    def __init__(self, config: dict):
        self.config = config
        self.logger = logging.getLogger(f"strategy.{self.name}")

    @abstractmethod
    def prepare(self, trade_date: str) -> None:
        """准备阶段：加载行情数据、因子值、候选股票池"""
        raise NotImplementedError

    @abstractmethod
    def calculate(self) -> pd.DataFrame:
        """计算阶段：因子得分 → 综合评分 → 排序"""
        raise NotImplementedError

    @abstractmethod
    def generate_signal(self) -> List[Signal]:
        """生成阶段：按轮动规则输出 BUY/SELL/HOLD"""
        raise NotImplementedError

    def run(self, trade_date: str) -> List[Signal]:
        self.prepare(trade_date)
        self.calculate()
        return self.generate_signal()

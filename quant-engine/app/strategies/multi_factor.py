"""多因子评分策略：趋势40% + 价值30% + 质量20% + 风险10%，每日排名轮动

轮动规则（见 docs/技术准备文档.md §8.3）：
- 股票池：沪深300 + 中证500（约 800 只），横截面排名
- 未持仓且 rank <= buy_buffer  → BUY（等权，单股仓位上限 max_position_pct）
- 已持仓且 rank > sell_buffer  → SELL
- 其余 → HOLD（缓冲带防止频繁换手）
"""
import logging
from typing import List

import pandas as pd

from app.strategies.base import Signal, Strategy

logger = logging.getLogger(__name__)


class MultiFactorStrategy(Strategy):
    name = "multi_factor"
    description = "趋势40% + 价值30% + 质量20% + 风险10%"

    def __init__(self, config: dict):
        super().__init__(config)
        self.weights = {
            "trend": config.get("trend_weight", 0.40),
            "value": config.get("value_weight", 0.30),
            "quality": config.get("quality_weight", 0.20),
            "risk": config.get("risk_weight", 0.10),
        }
        self.universe = config.get("universe", "hs300+zz500")  # 股票池
        self.top_n = config.get("top_n", 20)                   # 目标持仓数
        self.buy_buffer = config.get("buy_buffer", 15)         # 未持仓且排名<=15 → BUY
        self.sell_buffer = config.get("sell_buffer", 30)       # 已持仓且排名>30 → SELL
        self.max_position_pct = config.get("max_position_pct", 0.20)  # 单股仓位上限
        self.holdings: set = set()  # 当前持仓代码（由 Trade Service 回填）
        self.data: pd.DataFrame = None

    def prepare(self, trade_date: str):
        # TODO(Sprint 4):
        # 1. 从 stock_basic 加载股票池（universe=hs300/zz500）
        # 2. 加载因子值（factor_value 表，横截面）
        # 3. 财务因子只用 announce_date <= 当日 的数据（防未来函数）
        # 4. 同步当前持仓（从 trade-service / position 表）
        raise NotImplementedError("待 Sprint 4 实现")

    def calculate(self) -> pd.DataFrame:
        # TODO(Sprint 4):
        # 1. 各因子去极值（winsorize）+ 横截面百分位归一化（rank_normalize）
        # 2. 按权重加权求和 → 综合评分
        # 3. 结果写入 factor_value / strategy_signal 表
        raise NotImplementedError("待 Sprint 4 实现")

    def generate_signal(self) -> List[Signal]:
        # TODO(Sprint 4):
        # 未持仓且 rank <= buy_buffer  → BUY
        # 已持仓且 rank > sell_buffer  → SELL
        # 其余 → HOLD
        raise NotImplementedError("待 Sprint 4 实现")

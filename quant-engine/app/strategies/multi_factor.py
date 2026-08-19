"""多因子评分策略：趋势40% + 价值30% + 质量20% + 风险10%，每日排名轮动

轮动规则（docs §8.3）：
- 股票池：沪深300 + 中证500（约 800 只），横截面排名
- 评分 = 100 × Σ(factor_definition.weight × normalized)，normalized 已按
  方向统一为"越高越好"（因子计算见 app/factor_service.py）
- 未持仓且 rank <= buy_buffer    → BUY
- 已持仓且 rank >  sell_buffer   → SELL
- 其余                          → HOLD（缓冲带防止频繁换手）

持仓来源：Sprint 5 才有 position 表，此处从最近一次历史信号重建
（昨日 action ∈ {BUY, HOLD} → 持有；SELL → 平仓），首日为空集。
"""
import logging
from datetime import date
from typing import List

import pandas as pd
from sqlalchemy import select

from app.factor_service import score_cross_section
from app.models.tables import FactorDefinition, FactorValue, StrategySignal
from app.strategies.base import Signal, Strategy

logger = logging.getLogger(__name__)

ALL_FACTORS = ["ma_trend", "macd_signal", "pe_ratio", "pb_ratio",
               "roe_quality", "debt_risk"]


def rotation_action(rank: int, held: bool, buy_buffer: int,
                    sell_buffer: int) -> str:
    """轮动规则（实盘与回测同口径，docs §8.3）：
    - 未持仓且 rank <= buy_buffer → BUY
    - 已持仓且 rank > sell_buffer → SELL
    - 其余 → HOLD（缓冲带防止频繁换手）"""
    if not held and rank <= buy_buffer:
        return "BUY"
    if held and rank > sell_buffer:
        return "SELL"
    return "HOLD"


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
        self.db = config.get("db")
        self.trade_date: date | None = None
        self.holdings: set = set()
        self.data: pd.DataFrame | None = None

    # ---------- 三个阶段 ----------

    def prepare(self, trade_date: str) -> None:
        """加载该交易日因子横截面 + 从最近一次历史信号重建持仓"""
        if self.db is None:
            from app.db import get_session
            self.db = get_session()
        self.trade_date = date.fromisoformat(trade_date)

        rows = self.db.execute(
            select(FactorValue.code, FactorValue.factor_name,
                   FactorValue.normalized)
            .where(FactorValue.trade_date == self.trade_date)
        ).all()
        if not rows:
            raise RuntimeError(
                f"{self.trade_date} 无因子数据，请先运行因子计算（job_calc_factors）")
        df = pd.DataFrame(rows, columns=["code", "factor_name", "normalized"])
        self.factor_df = df.pivot_table(
            index="code", columns="factor_name", values="normalized")
        missing = [f for f in ALL_FACTORS if f not in self.factor_df.columns]
        if missing:
            logger.warning("因子数据缺少 %s（当日可能无有效值）", missing)
        self.holdings = self._reconstruct_holdings(self.trade_date)

    def calculate(self) -> pd.DataFrame:
        """综合评分（复用 factor_service.score_cross_section，回测同口径）"""
        defs = list(self.db.execute(
            select(FactorDefinition).where(
                FactorDefinition.name.in_(ALL_FACTORS))).scalars())
        self.factor_weights = {d.name: float(d.weight) for d in defs}
        self.factor_categories = {d.name: d.category for d in defs}
        if len(self.factor_weights) != len(ALL_FACTORS):
            logger.warning("factor_definition 权重不完整：%s", self.factor_weights)
        self.data = score_cross_section(self.factor_df, self.factor_weights)
        return self.data

    def generate_signal(self) -> List[Signal]:
        if self.data is None or self.data.empty:
            logger.warning("无可评分股票（6 因子全有才参与），无信号")
            return []
        n = len(self.data)
        signals: List[Signal] = []
        for code, row in self.data.iterrows():
            rank = int(row["rank"])
            action = rotation_action(rank, code in self.holdings,
                                     self.buy_buffer, self.sell_buffer)
            signals.append(Signal(
                code=code, score=round(float(row["score"]), 2),
                action=action, reason=self._reason(row, rank, n, action)))
        return signals

    # ---------- 内部 ----------

    def _reconstruct_holdings(self, td: date) -> set:
        """持仓重建：最近一次历史信号日 action ∈ {BUY, HOLD} 的代码集合"""
        prev = self.db.execute(
            select(StrategySignal.trade_date)
            .where(StrategySignal.strategy_name == self.name,
                   StrategySignal.trade_date < td)
            .order_by(StrategySignal.trade_date.desc())
            .limit(1)
        ).scalar()
        if prev is None:
            logger.info("无历史信号，首次运行持仓为空")
            return set()
        actions = self.db.execute(
            select(StrategySignal.code, StrategySignal.action)
            .where(StrategySignal.strategy_name == self.name,
                   StrategySignal.trade_date == prev)
        ).all()
        return {code for code, action in actions if action in ("BUY", "HOLD")}

    def _reason(self, row: pd.Series, rank: int, n: int,
                action: str) -> str:
        """信号原因：评分 + 排名 + 分项得分（类别口径，权重来自 factor_definition）"""
        detail = []
        for cat, label in (("trend", "趋势"), ("value", "价值"),
                           ("quality", "质量"), ("risk", "风险")):
            cat_factors = [
                f for f in self.factor_weights
                if self.factor_categories.get(f) == cat
            ]
            if not cat_factors:
                continue
            part = sum(
                self.factor_weights[f]
                * float(self.factor_df.loc[row.name, f]) * 100.0
                for f in cat_factors
                if f in self.factor_df.columns
                and not pd.isna(self.factor_df.loc[row.name, f])
            )
            detail.append(f"{label} {part:.0f}")
        action_desc = {
            "BUY": "进入买入区",
            "SELL": "跌出卖出缓冲带",
            "HOLD": "缓冲带内",
        }[action]
        return f"评分 {row['score']:.1f}，排名 {rank}/{n}；" \
               f"{' '.join(detail)}；{action_desc}"

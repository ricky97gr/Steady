"""回测策略：内存重放历史因子，与实盘同口径评分/轮动

因子重放原则（docs §8.3）：
- MA/MACD 的 rolling/ewm 均为因果算子，序列在 T 日的值只依赖 <= T 的数据
  → 逐日取截面值天然无未来函数
- 估值（日度 PE/PB）按 as-of 对齐（<= T 的最近一日，超过 30 天视为缺失）
- 财务按 announce_date <= T 对齐（防未来函数核心）
- 前复权价：close × adj_factor / 区间最新 adj_factor（对 MA/MACD 的
  0/1 因子数学上等价于原始价，仍按文档口径计算）

持仓演化：从回测内信号历史累计（BUY 加入 / SELL 移除），首日为空集，
与实盘从 strategy_signal 重建的规则一致。
"""
import logging
from datetime import date, timedelta

import numpy as np
import pandas as pd
from sqlalchemy import select

from app.factor_service import FACTOR_DIRECTION, normalize_cross_section, \
    score_cross_section
from app.factors.financial import latest_by_announce
from app.factors.trend import macd_signal, ma_trend
from app.models.tables import (DailyPrice, DailyValuation, FinancialIndicator,
                               StockBasic, TradeCalendar)
from app.strategies.base import Signal
from app.strategies.multi_factor import ALL_FACTORS, rotation_action

logger = logging.getLogger(__name__)

VALUATION_ASOF_DAYS = 30  # 与 factor_service 同口径
PRICE_WARMUP_DAYS = 120   # 区间起点前的因子 warmup 窗口（MA20/EMA26）


class ReplayStrategy:
    """回测用策略：一次 preload 全部因子序列，逐日生成信号"""

    name = "multi_factor"
    description = "多因子评分策略（回测重放）"

    def __init__(self, db, config: dict, weights: dict[str, float],
                 categories: dict[str, str]):
        self.db = db
        self.weights = weights
        self.categories = categories
        self.top_n = config.get("top_n", 20)
        self.buy_buffer = config.get("buy_buffer", 15)
        self.sell_buffer = config.get("sell_buffer", 30)
        self.max_position_pct = config.get("max_position_pct", 0.20)
        self.holdings: set = set()
        self.pool: list[str] = []
        # 预计算数据：{code: {dates: np.ndarray(日期), factors: np.ndarray(n×6),
        #                     closes: {date: 原始收盘}}}
        self.series: dict[str, dict] = {}
        self._date_pos: dict[date, int] = {}

    # ---------- 预计算 ----------

    def preload(self, start: date | str, end: date | str):
        """加载股票池因子序列与真实价（一次全量，内存重放）"""
        start = date.fromisoformat(start) if isinstance(start, str) else start
        end = date.fromisoformat(end) if isinstance(end, str) else end
        self.pool = sorted(self.db.execute(
            select(StockBasic.code).where(
                StockBasic.universe.in_(("hs300", "zz500")))).scalars().all())
        grid_rows = self.db.execute(
            select(TradeCalendar.cal_date).where(
                TradeCalendar.cal_date >= start,
                TradeCalendar.cal_date <= end,
                TradeCalendar.is_open.is_(True))
            .order_by(TradeCalendar.cal_date)).scalars().all()
        self.grid = [d for d in grid_rows]
        self._date_pos = {d: i for i, d in enumerate(self.grid)}
        if not self.pool or not self.grid:
            raise RuntimeError("股票池或交易日为空，无法回测")

        price_start = start - timedelta(days=PRICE_WARMUP_DAYS)
        price_rows = self.db.execute(
            select(DailyPrice.code, DailyPrice.trade_date, DailyPrice.close,
                   DailyPrice.adj_factor)
            .where(DailyPrice.code.in_(self.pool),
                   DailyPrice.trade_date >= price_start,
                   DailyPrice.trade_date <= end)
            .order_by(DailyPrice.code, DailyPrice.trade_date)).all()
        closes: dict[str, list] = {}
        anchors: dict[str, float] = {}
        for code, td, close, adj in price_rows:
            if close is None or adj is None:
                continue
            closes.setdefault(code, []).append((td, float(close), float(adj)))
            anchors[code] = float(adj)
        val_rows = self.db.execute(
            select(DailyValuation.code, DailyValuation.trade_date,
                   DailyValuation.pe_ttm, DailyValuation.pb)
            .where(DailyValuation.code.in_(self.pool),
                   DailyValuation.trade_date <= end)
            .order_by(DailyValuation.code, DailyValuation.trade_date)).all()
        valuations: dict[str, list] = {}
        for code, td, pe, pb in val_rows:
            valuations.setdefault(code, []).append(
                {"trade_date": td,
                 "pe_ttm": float(pe) if pe is not None else None,
                 "pb": float(pb) if pb is not None else None})
        fin_rows = self.db.execute(
            select(FinancialIndicator.code, FinancialIndicator.report_date,
                   FinancialIndicator.announce_date,
                   FinancialIndicator.roe, FinancialIndicator.debt_ratio)
            .where(FinancialIndicator.code.in_(self.pool))
            .order_by(FinancialIndicator.code,
                      FinancialIndicator.announce_date)).all()
        financials: dict[str, list] = {}
        for code, rd, ad, roe, debt in fin_rows:
            financials.setdefault(code, []).append(
                {"report_date": rd, "announce_date": ad,
                 "roe": float(roe) if roe is not None else None,
                 "debt_ratio": float(debt) if debt is not None else None})

        for code in self.pool:
            self.series[code] = self._build_series(
                code, closes.get(code, []), anchors.get(code),
                valuations.get(code, []), financials.get(code, []))
        logger.info("回测预计算完成：%s 只股票 × %s 个交易日",
                    len(self.pool), len(self.grid))

    def _build_series(self, code: str, price_rows: list, anchor: float | None,
                      val_rows: list, fin_rows: list) -> dict:
        """单只股票：6 因子在回测日期网格上的序列 + 真实收盘价"""
        n = len(self.grid)
        factors = np.full((n, len(ALL_FACTORS)), np.nan)
        closes: dict[date, float] = {}
        if price_rows and anchor:
            series = pd.Series(
                [c * adj / anchor for _, c, adj in price_rows],
                index=[td for td, _, _ in price_rows])
            closes = {td: c for td, c, _ in price_rows}
            idx = {"ma_trend": 0, "macd_signal": 1}
            for name, pos in idx.items():
                fn = ma_trend if name == "ma_trend" else macd_signal
                s = fn(series).reindex(self.grid, method="ffill")
                for i, v in enumerate(s.values):
                    if v is not None and not pd.isna(v):
                        factors[i, pos] = float(v)
        # 估值 as-of（<= T 最近一日，> 30 天视为缺失）
        if val_rows:
            val_df = pd.DataFrame(val_rows)
            factors[:, 2] = self._asof_grid(val_df, "pe_ttm")
            factors[:, 3] = self._asof_grid(val_df, "pb")
        # 财务 as-of（announce_date <= T 的最新一期）
        if fin_rows:
            fin_df = pd.DataFrame(fin_rows)
            factors[:, 4] = self._asof_grid(fin_df, "roe", by="announce_date")
            factors[:, 5] = self._asof_grid(fin_df, "debt_ratio", by="announce_date")
        return {"factors": factors, "closes": closes}

    def _asof_grid(self, df: pd.DataFrame, col: str,
                   by: str = "trade_date") -> np.ndarray:
        """df(by, col) → 网格上的 as-of 序列（searchsorted 前向对齐）

        - 估值：<= T 的最近一日，超过 VALUATION_ASOF_DAYS 视为缺失；
          PE/PB <= 0（亏损股）不参与价值横截面
        - 财务：announce_date <= T 的最新一期（无窗口限制）
        """
        if by == "trade_date":
            date_col = "trade_date"
            limit = np.timedelta64(timedelta(days=VALUATION_ASOF_DAYS))
        else:
            date_col = "announce_date"
            limit = np.timedelta64(timedelta(days=3650))  # 财报 as-of 无窗口限制
        t = df[[date_col, col]].dropna(subset=[col])
        if t.empty:
            return np.full(len(self.grid), np.nan)
        t = t.drop_duplicates(date_col, keep="last").sort_values(date_col)
        src = t[date_col].values.astype("datetime64[ns]")
        vals = t[col].values.astype(float)
        grid = np.array(self.grid, dtype="datetime64[ns]")
        pos = np.searchsorted(src, grid, side="right") - 1  # 每个网格点最后一个 <= 的源
        out = np.full(len(self.grid), np.nan)
        valid = pos >= 0
        if not valid.any():
            return out
        out[valid] = vals[np.clip(pos[valid], 0, len(vals) - 1)]
        last_dates = src[np.clip(pos[valid], 0, len(src) - 1)]
        gap = grid[valid] - last_dates
        out[valid] = np.where(gap > limit, np.nan, out[valid])
        if by == "trade_date":  # 亏损股 PE/PB <= 0 不参与价值横截面
            out[out <= 0] = np.nan
        return out

    # ---------- 三阶段（与实盘 Strategy.run 接口一致） ----------

    def prepare(self, trade_date: str):
        td = date.fromisoformat(trade_date)
        if td not in self._date_pos:
            raise RuntimeError(f"{td} 不在回测区间内")
        pos = self._date_pos[td]
        # 当日截面原始值（缺失 = NaN）
        self._raw = {f: {} for f in ALL_FACTORS}
        for code, s in self.series.items():
            vals = s["factors"][pos]
            for i, f in enumerate(ALL_FACTORS):
                v = vals[i]
                if v == v and not np.isnan(v):  # NaN 不参与横截面
                    self._raw[f][code] = float(v)
        self._date = td

    def calculate(self) -> pd.DataFrame:
        norm = {
            f: normalize_cross_section(self._raw[f], FACTOR_DIRECTION[f])
            for f in ALL_FACTORS
        }
        pivot = pd.DataFrame({
            f: {code: rec[2] for code, rec in n.items()} for f, n in norm.items()
        })
        self.data = score_cross_section(pivot, self.weights)
        return self.data

    def generate_signal(self):
        if self.data is None or self.data.empty:
            return []
        n = len(self.data)
        signals: list[Signal] = []
        for code, row in self.data.iterrows():
            rank = int(row["rank"])
            held = code in self.holdings
            action = rotation_action(rank, held, self.buy_buffer, self.sell_buffer)
            if action == "BUY":
                self.holdings.add(code)
            elif action == "SELL":
                self.holdings.discard(code)
            signals.append(Signal(
                code=code, score=round(float(row["score"]), 2),
                action=action,
                reason=f"回测排名 {rank}/{n}，评分 {row['score']:.1f}"))
        return signals

    def run(self, trade_date: str):
        self.prepare(trade_date)
        self.calculate()
        return self.generate_signal()

    # ---------- 引擎取价（复用预加载数据，避免重复查库） ----------

    def price_at(self, code: str, trade_date: str) -> float | None:
        """当日真实收盘价（不复权，模拟成交口径）"""
        s = self.series.get(code)
        if s is None:
            return None
        return s["closes"].get(date.fromisoformat(trade_date))

    def prev_close_at(self, code: str, trade_date: str) -> float | None:
        """前一交易日收盘价（涨跌停判断；区间首日无前日 → None）"""
        td = date.fromisoformat(trade_date)
        pos = self._date_pos.get(td)
        if pos is None or pos == 0:
            return None
        prev = self.grid[pos - 1]
        s = self.series.get(code)
        return s["closes"].get(prev) if s else None

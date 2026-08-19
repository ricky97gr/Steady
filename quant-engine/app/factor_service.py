"""因子计算服务：对指定交易日计算全股票池 6 因子并写入 factor_value

流程（docs §8.3）：
1. 加载股票池与因子输入（行情 90 日 / 估值 as-of 30 日 / 财务全量）
2. 逐因子横截面：winsorize 去极值 → rank_normalize 百分位 → 方向调整
   （asc 保持、desc 取反，normalized 恒为"越高越好"），缺失值不参与横截面
3. upsert 写入 factor_value（UNIQUE code+factor_name+trade_date，当日重跑幂等）

trend 因子用前复权价（qfq = close × adj_factor / 窗口最新 adj_factor）。
"""
import logging
from datetime import date, timedelta

import pandas as pd
from sqlalchemy import select

from app.db import upsert
from app.factors.financial import debt_by_announce, roe_by_announce
from app.factors.trend import macd_signal, ma_trend, winsorize
from app.factors.value import pb_value, pe_value
from app.models.tables import (DailyPrice, DailyValuation,
                               FinancialIndicator, FactorValue, StockBasic)

logger = logging.getLogger(__name__)

# 因子方向：asc = 值越大分越高；desc = 值越小分越高
FACTOR_DIRECTION = {
    "ma_trend": "asc",
    "macd_signal": "asc",
    "pe_ratio": "desc",
    "pb_ratio": "desc",
    "roe_quality": "asc",
    "debt_risk": "desc",
}

PRICE_WINDOW_DAYS = 90    # 行情窗口（MA20 与 MACD EMA26 warmup 足够）
VALUATION_ASOF_DAYS = 30  # 估值 as-of 回看窗口（停牌日兜底）


def pool_codes(db) -> list[str]:
    """股票池（沪深300 + 中证500）"""
    return sorted(db.execute(
        select(StockBasic.code).where(StockBasic.universe.in_(("hs300", "zz500")))
    ).scalars().all())


def load_factor_inputs(db, codes: list[str], trade_date: date) -> dict[str, dict]:
    """一次性加载因子输入：{code: {close: Series, valuation: df, financial: df}}"""
    price_start = trade_date - timedelta(days=PRICE_WINDOW_DAYS)
    val_start = trade_date - timedelta(days=VALUATION_ASOF_DAYS)

    # 1. 行情（前复权：close × adj_factor / 窗口最新 adj_factor）
    price_rows = db.execute(
        select(DailyPrice.code, DailyPrice.trade_date, DailyPrice.close,
               DailyPrice.adj_factor)
        .where(DailyPrice.code.in_(codes),
               DailyPrice.trade_date >= price_start,
               DailyPrice.trade_date <= trade_date)
        .order_by(DailyPrice.code, DailyPrice.trade_date)
    ).all()
    closes: dict[str, list] = {}
    anchors: dict[str, float] = {}
    for code, td, close, adj in price_rows:
        if close is None or adj is None:
            continue
        closes.setdefault(code, []).append((td, float(close), float(adj)))
        anchors[code] = float(adj)

    # 2. 估值（as-of 窗口内，停牌日取最近一日）
    val_rows = db.execute(
        select(DailyValuation.code, DailyValuation.trade_date,
               DailyValuation.pe_ttm, DailyValuation.pb)
        .where(DailyValuation.code.in_(codes),
               DailyValuation.trade_date >= val_start,
               DailyValuation.trade_date <= trade_date)
        .order_by(DailyValuation.code, DailyValuation.trade_date)
    ).all()
    valuations: dict[str, list] = {}
    for code, td, pe, pb in val_rows:
        valuations.setdefault(code, []).append(
            {"trade_date": td,
             "pe_ttm": float(pe) if pe is not None else None,
             "pb": float(pb) if pb is not None else None})

    # 3. 财务（公告日 <= 当日；回填后约 800×20 期，内存可承受）
    fin_rows = db.execute(
        select(FinancialIndicator.code, FinancialIndicator.report_date,
               FinancialIndicator.announce_date, FinancialIndicator.roe,
               FinancialIndicator.debt_ratio)
        .where(FinancialIndicator.code.in_(codes),
               FinancialIndicator.announce_date <= trade_date)
    ).all()
    financials: dict[str, list] = {}
    for code, rd, ad, roe, debt in fin_rows:
        financials.setdefault(code, []).append(
            {"report_date": rd, "announce_date": ad,
             "roe": float(roe) if roe is not None else None,
             "debt_ratio": float(debt) if debt is not None else None})

    inputs: dict[str, dict] = {}
    for code in codes:
        close_rows = closes.get(code, [])
        inputs[code] = {
            "close": pd.Series(
                [c * adj / anchors[code] for _, c, adj in close_rows],
                index=[td for td, _, _ in close_rows],
            ),
            "valuation": pd.DataFrame(valuations.get(code, [])),
            "financial": pd.DataFrame(financials.get(code, [])),
        }
    return inputs


def factor_raw_value(factor: str, inp: dict, trade_date: date) -> float | None:
    """单个因子在 trade_date 的原始值（None = 缺失，不参与横截面）"""
    if factor == "ma_trend":
        s = ma_trend(inp["close"])
        return None if s.empty or pd.isna(s.iloc[-1]) else float(s.iloc[-1])
    if factor == "macd_signal":
        s = macd_signal(inp["close"])
        return None if s.empty else float(s.iloc[-1])
    if factor == "pe_ratio":
        return pe_value(inp["valuation"], trade_date)
    if factor == "pb_ratio":
        return pb_value(inp["valuation"], trade_date)
    if factor == "roe_quality":
        return roe_by_announce(inp["financial"], trade_date)
    if factor == "debt_risk":
        return debt_by_announce(inp["financial"], trade_date)
    raise ValueError(f"未知因子: {factor}")


def normalize_cross_section(raw: dict[str, float | None], direction: str
                            ) -> dict[str, tuple[float, int, float]]:
    """横截面归一化 → {code: (value, rank, normalized)}

    winsorize 去极值 → 百分位 rank；desc 方向翻转（rank=1 / normalized=1
    恒为该因子最优）。rank 用 method="min" 保证并列名次一致。
    """
    s = pd.Series(raw).dropna()
    if s.empty:
        return {}
    win = winsorize(s)
    ranks = win.rank(method="min")
    pct = win.rank(pct=True)
    if direction == "desc":
        ranks = len(ranks) + 1 - ranks
        pct = 1 - pct
    return {
        code: (float(v), int(ranks[code]), float(pct[code]))
        for code, v in s.items()
    }


def score_cross_section(factor_df: pd.DataFrame,
                        weights: dict[str, float]) -> pd.DataFrame:
    """综合评分：score = 100 × Σ(weight × normalized)，仅 6 因子全有的股票

    返回 DataFrame(index=code, columns=[score, rank])，按 rank 升序。
    因子权重总和为 1.0（factor_definition 预置，见 init.sql），
    normalized 已按方向统一为"越高越好"。
    """
    # 全池均无有效值的因子（如该日无任何股票披露）不参与评分，
    # 避免全 NaN 列把整个横截面 drop 为空
    factors = [f for f in weights
               if f in factor_df.columns and factor_df[f].notna().any()]
    complete = factor_df[factors].dropna()
    if complete.empty:
        return pd.DataFrame(columns=["score", "rank"])
    score = sum(float(weights[f]) * complete[f] for f in factors) * 100.0
    df = pd.DataFrame({"score": score})
    df["rank"] = df["score"].rank(method="min", ascending=False).astype(int)
    return df.sort_values("rank")


def compute_and_store(db, trade_date: date) -> dict:
    """计算全部因子并写入 factor_value（当日重跑幂等），返回每因子有效数"""
    codes = pool_codes(db)
    inputs = load_factor_inputs(db, codes, trade_date)
    stats: dict[str, int] = {}
    total = 0
    for factor, direction in FACTOR_DIRECTION.items():
        raw = {
            code: factor_raw_value(factor, inp, trade_date)
            for code, inp in inputs.items()
        }
        norm = normalize_cross_section(raw, direction)
        rows = [
            {"code": code, "factor_name": factor, "trade_date": trade_date,
             "value": v, "rank": rk, "normalized": nm}
            for code, (v, rk, nm) in norm.items()
        ]
        upsert(db, FactorValue, rows,
               conflict_cols=["code", "factor_name", "trade_date"],
               update_cols=["value", "rank", "normalized"])
        stats[factor] = len(rows)
        total += len(rows)
    logger.info("因子计算完成 %s：%s，共 %s 行", trade_date, stats, total)
    return stats

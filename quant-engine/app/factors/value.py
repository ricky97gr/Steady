"""价值因子（日度估值，as-of 取值）

估值接口（东财 stock_value_em）按日给出 PE(TTM)/市净率，天然 as-of，
无未来函数问题；pe/pb <= 0（亏损股）返回 None，不参与价值横截面。
"""
from datetime import date

import pandas as pd


def latest_asof(df: pd.DataFrame, trade_date: date, col: str) -> float | None:
    """取 <= trade_date 的最后一行 col 值（停牌兜底前移；30 天前仍无 → None）"""
    if df is None or df.empty:
        return None
    valid = df[df["trade_date"] <= trade_date]
    if valid.empty:
        return None
    v = valid.iloc[-1][col]
    if v is None or pd.isna(v):
        return None
    return float(v)


def pe_value(valuation_df: pd.DataFrame | None, trade_date: date) -> float | None:
    """PE 因子值：as-of 日 PE(TTM)；<=0（亏损）→ None"""
    pe = latest_asof(valuation_df, trade_date, "pe_ttm")
    return pe if pe is not None and pe > 0 else None


def pb_value(valuation_df: pd.DataFrame | None, trade_date: date) -> float | None:
    """PB 因子值：as-of 日市净率；<=0 → None"""
    pb = latest_asof(valuation_df, trade_date, "pb")
    return pb if pb is not None and pb > 0 else None

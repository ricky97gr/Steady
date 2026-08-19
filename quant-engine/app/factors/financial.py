"""财务因子（质量/风险，announce_date 防未来函数）

核心约束：因子计算只能使用 announce_date <= 交易日期 的财报
（docs §5.2.3 警示：按 report_date 对齐会"看了未来数据"）。
"""
from datetime import date

import pandas as pd


def latest_by_announce(fin_df: pd.DataFrame | None, trade_date: date,
                       col: str) -> float | None:
    """最新一期财务值：announce_date <= trade_date 中公告日最晚的一期"""
    if fin_df is None or fin_df.empty:
        return None
    valid = fin_df[fin_df["announce_date"] <= trade_date]
    if valid.empty:
        return None
    row = valid.sort_values(["announce_date", "report_date"]).iloc[-1]
    v = row[col]
    if v is None or pd.isna(v):
        return None
    return float(v)


def roe_by_announce(fin_df: pd.DataFrame | None, trade_date: date) -> float | None:
    """ROE 质量因子：公告日 <= 当日的最新一期 ROE"""
    return latest_by_announce(fin_df, trade_date, "roe")


def debt_by_announce(fin_df: pd.DataFrame | None, trade_date: date) -> float | None:
    """负债风险因子：公告日 <= 当日的最新一期资产负债率（越低越好，desc）"""
    return latest_by_announce(fin_df, trade_date, "debt_ratio")

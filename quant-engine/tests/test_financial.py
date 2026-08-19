"""财务因子单元测试：announce_date 防未来函数（docs §5.2.3）"""
from datetime import date

import pandas as pd
import pytest

from app.factors.financial import (debt_by_announce, latest_by_announce,
                                   roe_by_announce)


def _fin_df(rows):
    """rows: [(report_date, announce_date, {col: value}), ...] → DataFrame"""
    return pd.DataFrame([
        {"report_date": date(*r), "announce_date": date(*a), **kv}
        for r, a, kv in rows])


def test_roe_future_announce_not_used():
    """防未来函数核心：公告日晚于 T 的财报，T 日因子不得使用"""
    df = _fin_df([
        ((2026, 3, 31), (2026, 4, 20), {"roe": 10.0}),  # 一季报（已公告）
        ((2026, 6, 30), (2026, 8, 18), {"roe": 18.0}),  # 中报 8-18 才公告
    ])
    assert roe_by_announce(df, date(2026, 8, 10)) == 10.0  # 中报未公告 → 用一季报
    assert roe_by_announce(df, date(2026, 8, 18)) == 18.0  # 公告当日可用
    assert roe_by_announce(df, date(2026, 4, 19)) is None  # 首份公告前无数据


def test_latest_by_announce_tiebreak_report_date():
    """同日公告多期 → 报告期最新者胜（补报场景）"""
    df = _fin_df([
        ((2026, 3, 31), (2026, 5, 1), {"debt_ratio": 40.0}),
        ((2026, 6, 30), (2026, 5, 1), {"debt_ratio": 45.0}),
    ])
    assert debt_by_announce(df, date(2026, 5, 2)) == 45.0


def test_latest_by_announce_same_date_take_last():
    # 中报公告晚于 Q1 快报 → 取中报
    df = _fin_df([
        ((2026, 3, 31), (2026, 7, 10), {"roe": 8.0}),
        ((2026, 6, 30), (2026, 7, 20), {"roe": 12.0}),
    ])
    assert roe_by_announce(df, date(2026, 7, 21)) == 12.0


def test_nan_col_value_returns_none():
    df = _fin_df([((2026, 6, 30), (2026, 7, 1), {"roe": None})])
    assert roe_by_announce(df, date(2026, 7, 2)) is None


def test_empty_inputs():
    assert roe_by_announce(None, date(2026, 7, 2)) is None
    assert debt_by_announce(pd.DataFrame(), date(2026, 7, 2)) is None

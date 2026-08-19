"""价值因子（日度估值）单元测试：as-of 取值 + 亏损股排除"""
from datetime import date

import pandas as pd
import pytest

from app.factors.value import latest_asof, pb_value, pe_value


def _val_df(rows):
    """rows: [((y, m, d), {col: value}), ...] → DataFrame"""
    return pd.DataFrame(
        [{"trade_date": date(*d), **kv} for d, kv in rows])


def test_latest_asof_takes_last_row_before_or_equal():
    df = _val_df([
        ((2026, 8, 1), {"pe_ttm": 10.0}),
        ((2026, 8, 15), {"pe_ttm": 12.0}),
        ((2026, 8, 20), {"pe_ttm": 15.0}),
    ])
    assert latest_asof(df, date(2026, 8, 16), "pe_ttm") == 12.0
    assert latest_asof(df, date(2026, 8, 20), "pe_ttm") == 15.0  # 等号包含当日


def test_latest_asof_before_first_date_is_none():
    df = _val_df([((2026, 8, 20), {"pe_ttm": 15.0})])
    assert latest_asof(df, date(2026, 8, 10), "pe_ttm") is None
    assert latest_asof(None, date(2026, 8, 10), "pe_ttm") is None
    assert latest_asof(pd.DataFrame(), date(2026, 8, 10), "pe_ttm") is None


def test_latest_asof_nan_row_returns_none():
    # 最近一日列为 NaN → 视为缺失（上游估值接口该日无有效值）
    df = _val_df([((2026, 8, 15), {"pe_ttm": None})])
    assert latest_asof(df, date(2026, 8, 16), "pe_ttm") is None


def test_pe_value_nonpositive_is_none():
    df = _val_df([
        ((2026, 8, 15), {"pe_ttm": -3.0}),
        ((2026, 8, 16), {"pe_ttm": 0.0}),
        ((2026, 8, 17), {"pe_ttm": 12.5}),
    ])
    assert pe_value(df, date(2026, 8, 15)) is None  # 亏损
    assert pe_value(df, date(2026, 8, 16)) is None  # 零
    assert pe_value(df, date(2026, 8, 17)) == 12.5
    assert pe_value(df, date(2026, 8, 14)) is None  # 无数据


def test_pb_value_asof_forward():
    df = _val_df([((2026, 8, 15), {"pb": 2.5})])
    assert pb_value(df, date(2026, 8, 15)) == 2.5
    assert pb_value(df, date(2026, 8, 20)) == 2.5  # as-of 前移（停牌兜底）


def test_pb_value_nonpositive_is_none():
    df = _val_df([((2026, 8, 15), {"pb": -1.0})])
    assert pb_value(df, date(2026, 8, 15)) is None

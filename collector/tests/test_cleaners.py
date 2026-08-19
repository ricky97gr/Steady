"""数据质量校验规则单元测试（文档 §7.7）"""
from app.cleaners import (
    board_of,
    clean_daily_rows,
    validate_ohlc,
    validate_price_change,
)


def test_ohlc_valid():
    row = {"open": 10, "high": 11, "low": 9, "close": 10.5}
    assert validate_ohlc(row) == []


def test_ohlc_inconsistent_high():
    # high < open，异常
    row = {"open": 10, "high": 9, "low": 8, "close": 10.5}
    assert validate_ohlc(row) != []


def test_ohlc_missing_field():
    row = {"open": None, "high": 11, "low": 9, "close": 10.5}
    assert validate_ohlc(row) != []


def test_price_change_within_limit():
    # 主板 600519：+10% 正好在涨停边界内
    assert validate_price_change("600519", 11.0, 10.0)


def test_price_change_exceed_limit():
    # 主板 +20% 超涨停，异常
    assert not validate_price_change("600519", 12.0, 10.0)


def test_price_change_chinext():
    # 创业板 300001：+20% 在边界内
    assert validate_price_change("300001", 12.0, 10.0)


def test_board_of():
    assert board_of("688001") == "star"
    assert board_of("300001") == "chinext"
    assert board_of("830001") == "bse"
    assert board_of("600519") == "main"


def test_clean_daily_rows_drops_bad():
    rows = [
        {"trade_date": "2026-01-05", "open": 10, "high": 11, "low": 9,
         "close": 10.5, "prev_close": 10},
        {"trade_date": "2026-01-06", "open": 10, "high": 9, "low": 8,
         "close": 10.5, "prev_close": 10},  # OHLC 异常，应被丢弃
    ]
    clean = clean_daily_rows("600519", rows)
    assert len(clean) == 1
    assert clean[0]["trade_date"] == "2026-01-05"

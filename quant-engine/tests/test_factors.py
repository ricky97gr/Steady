"""因子计算单元测试"""
import pandas as pd

from app.factors.trend import ma_trend, macd_signal, rank_normalize, winsorize


def test_ma_trend_upward():
    # 持续上升：MA5 > MA20，末值为 1
    close = pd.Series(range(1, 51), dtype=float)
    assert ma_trend(close).iloc[-1] == 1


def test_ma_trend_downtrend():
    # 持续下跌：MA5 < MA20，末值为 0
    close = pd.Series(range(50, 0, -1), dtype=float)
    assert ma_trend(close).iloc[-1] == 0


def test_ma_trend_early_nan():
    # 前 19 个交易日无足够窗口，应为 NaN（不参与横截面计算）
    close = pd.Series(range(1, 30), dtype=float)
    s = ma_trend(close)
    assert s.iloc[:19].isna().all()


def test_macd_signal_upward():
    close = pd.Series(range(1, 60), dtype=float)
    assert macd_signal(close).iloc[-1] == 1


def test_rank_normalize_range():
    s = pd.Series([1, 5, 3, 9, 2])
    r = rank_normalize(s)
    assert r.min() >= 0
    assert r.max() <= 1
    assert r[s.idxmax()] == 1.0  # 最大值排名最高
    assert r[s.idxmin()] < r[s.idxmax()]  # 最小值排名最低


def test_winsorize_bounds():
    s = pd.Series([1, 2, 3, 1000])
    w = winsorize(s)
    assert w.max() < 1000  # 极端值被截断
    assert w.min() >= 1    # 低分位截断不越过最小值

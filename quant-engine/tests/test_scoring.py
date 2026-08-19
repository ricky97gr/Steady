"""横截面归一化与综合评分单元测试（docs §8.3 口径）"""
import pandas as pd
import pytest

from app.factor_service import normalize_cross_section, score_cross_section


def test_normalize_asc_direction():
    """asc：值越大归一化越高；rank 升序（pandas≥2.0 rank(pct=True)=rank/n）"""
    norm = normalize_cross_section({"a": 1.0, "b": 3.0, "c": 9.0}, "asc")
    assert norm["c"][2] == pytest.approx(1.0)       # 最高值 → 1.0
    assert norm["a"][2] == pytest.approx(1 / 3)     # 最低值 → 1/3
    assert norm["b"][2] == pytest.approx(2 / 3)
    assert norm["a"][1] == 1 and norm["c"][1] == 3  # rank
    assert norm["c"][0] > norm["b"][0] > norm["a"][0]  # value 保留原始值


def test_normalize_desc_flips():
    """desc（pe/pb/debt）：值越小归一化越高，normalized 恒为越高越好"""
    norm = normalize_cross_section({"a": 1.0, "b": 3.0, "c": 9.0}, "desc")
    assert norm["a"][2] == pytest.approx(2 / 3)     # 最小 raw → 最高归一化
    assert norm["c"][2] == pytest.approx(0.0)
    assert norm["a"][1] == 3 and norm["c"][1] == 1  # rank 翻转


def test_normalize_drops_missing():
    norm = normalize_cross_section({"a": 1.0, "b": None, "c": 9.0}, "asc")
    assert set(norm) == {"a", "c"}


def test_normalize_all_missing_returns_empty():
    assert normalize_cross_section({"a": None, "b": None}, "asc") == {}


def test_score_weights_product():
    """score = 100 × Σ(weight × normalized)"""
    pivot = pd.DataFrame({
        "ma_trend": {"A": 1.0, "B": 0.5},
        "pe_ratio": {"A": 0.8, "B": 0.9},
    })
    weights = {"ma_trend": 0.4, "pe_ratio": 0.6}
    df = score_cross_section(pivot, weights)
    # A: 100×(0.4×1.0+0.6×0.8)=88；B: 100×(0.4×0.5+0.6×0.9)=74
    assert df.loc["A", "score"] == pytest.approx(88.0)
    assert df.loc["B", "score"] == pytest.approx(74.0)
    assert df.loc["A", "rank"] == 1
    assert df.loc["B", "rank"] == 2


def test_score_requires_all_factors():
    """6 因子全有才参与评分：任因子缺失（NaN）→ 该股票排除"""
    pivot = pd.DataFrame({
        "ma_trend": {"A": 1.0, "B": 0.5},
        "pe_ratio": {"A": None, "B": 0.9},
    })
    df = score_cross_section(pivot, {"ma_trend": 0.5, "pe_ratio": 0.5})
    assert "A" not in df.index
    assert "B" in df.index
    assert df.loc["B", "score"] == pytest.approx(70.0)  # 100×(0.5×0.5+0.5×0.9)


def test_score_tie_rank_same():
    """并列分数 → 相同 rank（method=min），次名跳号"""
    pivot = pd.DataFrame({
        "ma_trend": {"A": 1.0, "B": 1.0, "C": 0.5},
        "pe_ratio": {"A": 0.0, "B": 0.0, "C": 0.5},
    })
    df = score_cross_section(pivot, {"ma_trend": 0.6, "pe_ratio": 0.4})
    # A: 100×0.6=60；B: 60；C: 100×(0.6×0.5+0.4×0.5)=50
    assert df.loc["A", "score"] == df.loc["B", "score"] == pytest.approx(60.0)
    assert df.loc["A", "rank"] == df.loc["B", "rank"] == 1
    assert df.loc["C", "rank"] == 3

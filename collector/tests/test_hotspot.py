"""热点采集解析单测（Issue #4）：列名防御 _pick / 安全浮点 _f / 单位换算 / 板块映射"""
import pandas as pd
import pytest

from app.collectors.hotspot import _f, _fmt_flow, _fmt_yi, _pick


def test_pick_exact_then_substring():
    df = pd.DataFrame(columns=["板块", "涨跌幅", "净流入", "领涨股"])
    assert _pick(df, "板块") == "板块"
    # 候选 2 才精确命中
    assert _pick(df, "涨幅", "涨跌幅") == "涨跌幅"
    # 大小写不敏感
    df2 = pd.DataFrame(columns=["Name", "Close"])
    assert _pick(df2, "name") == "Name"
    # 子串双向包含（候选是列名的子串）
    assert _pick(df2, "nam") == "Name"


def test_pick_missing_returns_none():
    df = pd.DataFrame(columns=["板块"])
    assert _pick(df, "涨跌幅", "主力净流入") is None
    assert _pick(df, *()) is None


def test_f_safe():
    assert _f("12.5") == 12.5
    assert _f(None) is None
    assert _f("--") is None
    assert _f(float("nan")) is None


def test_fmt_units():
    assert _fmt_flow(1.5e8) == "1.50亿"   # 元 → 亿
    assert _fmt_yi(7.82) == "7.82亿"      # 已是亿（同花顺板块概览净流入单位）
    assert _fmt_flow(None) is None
    assert _fmt_yi(None) is None


def test_sectors_from_ths_mapping():
    """同花顺板块概览数据 → 涨幅榜/资金流榜映射（不触发网络）"""
    from app.collectors.hotspot import _sectors_gain_from, _sectors_flow_from

    ths = [
        {"name": "生物制品", "change_pct": 9.12, "leader": "三元基因",
         "net_inflow": "7.82亿", "net_inflow_raw": 7.82},
        {"name": "养殖业", "change_pct": 5.5, "leader": "某股",
         "net_inflow": None, "net_inflow_raw": None},
        {"name": "半导体", "change_pct": 2.1, "leader": "中芯",
         "net_inflow": "3.00亿", "net_inflow_raw": 3.0},
    ]
    gains = _sectors_gain_from(ths)
    assert gains[0]["name"] == "生物制品" and gains[0]["change_pct"] == 9.12
    assert len(gains) == 3

    flows = _sectors_flow_from(ths)
    assert flows[0]["name"] == "生物制品"      # 净流入最高
    assert len(flows) == 2                    # 净流入为 None 的养殖业被过滤
    assert flows[0]["net_inflow"] == "7.82亿"

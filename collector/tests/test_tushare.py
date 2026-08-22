"""Tushare 适配层测试（mock pro 客户端，无网络）

覆盖：token 读取 / ts_code 解析 / 全市场快照行结构（单位换算、复权因子）、
按股票 daily_pairs 的东财格式对齐。
"""
from datetime import date

import pandas as pd

from app.sources import tushare


# ---------- token 读取 ----------

def test_load_token_none_db():
    assert tushare.load_token(None) == ""


class NoTokenSession:
    """execute 返回 None（桩）：按未配置处理，且不炸（无 rollback 也安全）"""

    def __init__(self):
        self.executed = []

    def execute(self, stmt, params=None):
        self.executed.append(stmt)
        return None


def test_load_token_missing_row():
    assert tushare.load_token(NoTokenSession()) == ""


# ---------- ts_code 解析 ----------

def test_ts_code():
    assert tushare.ts_code("600519") == "600519.SH"
    assert tushare.ts_code("000001") == "000001.SZ"
    assert tushare.ts_code("300750") == "300750.SZ"
    assert tushare.ts_code("830001") == "830001.BJ"


# ---------- 全市场行情快照 ----------

class FakePro:
    """模拟 tushare.pro_api 客户端：快照按 trade_date、逐只按 ts_code+区间"""

    def daily(self, trade_date=None, **kw):
        td = trade_date or kw.get("end_date", "20260821")
        rows = [
            {"ts_code": "600519.SH", "trade_date": int(td),
             "open": 1500.0, "high": 1520.0, "low": 1490.0, "close": 1510.0,
             "vol": 10000.0, "amount": 1.5e6},  # amount 千元
            {"ts_code": "000001.SZ", "trade_date": int(td),
             "open": 10.0, "high": 10.2, "low": 9.8, "close": 10.1,
             "vol": 2e6, "amount": 2e4},
        ]
        if kw.get("ts_code"):
            rows = [r for r in rows if r["ts_code"] == kw["ts_code"]]
        return pd.DataFrame(rows)

    def adj_factor(self, trade_date=None, **kw):
        td = trade_date or kw.get("end_date", "20260821")
        rows = [
            {"ts_code": "600519.SH", "trade_date": int(td), "adj_factor": 12.3456},
            {"ts_code": "000001.SZ", "trade_date": int(td), "adj_factor": 1.0},
        ]
        if kw.get("ts_code"):
            rows = [r for r in rows if r["ts_code"] == kw["ts_code"]]
        return pd.DataFrame(rows)

    def daily_basic(self, trade_date=None, **kw):
        td = trade_date or kw.get("end_date", "20260821")
        return pd.DataFrame([
            {"ts_code": "600519.SH", "trade_date": int(td), "close": 1510.0,
             "total_mv": 1.9e6, "circ_mv": 1.8e6, "pe_ttm": 20.5, "pe": 21.0, "pb": 8.0},
            {"ts_code": "000001.SZ", "trade_date": int(td), "close": 10.1,
             "total_mv": 1.9e5, "circ_mv": 1.8e5, "pe_ttm": None, "pe": None, "pb": None},
        ])


def test_daily_snapshot_rows():
    pro = FakePro()
    rows = tushare.daily_snapshot(pro, date(2026, 8, 21))
    assert len(rows) == 2
    r = rows[0]
    assert r["code"] == "600519"                 # 去掉 .SH 后缀
    assert r["trade_date"] == date(2026, 8, 21)
    assert r["close"] == 1510.0
    assert r["amount"] == 1.5e9                  # 千元 → 元（×1000）
    assert r["adj_factor"] == 12.3456            # 绝对复权因子原样（东财口径）
    assert rows[1]["code"] == "000001"


def test_daily_snapshot_filters_codes():
    rows = tushare.daily_snapshot(FakePro(), date(2026, 8, 21), codes=["600519"])
    assert [r["code"] for r in rows] == ["600519"]


def test_daily_basic_snapshot_units():
    rows = tushare.daily_basic_snapshot(FakePro(), date(2026, 8, 21))
    r = rows[0]
    assert r["total_mv"] == 1.9e10               # 万元 → 元（×10000）
    assert r["float_mv"] == 1.8e10
    assert r["pe_ttm"] == 20.5
    # 缺失值 → None（入库 NULL，正负判断留给因子侧）
    assert rows[1]["pe_ttm"] is None
    assert rows[1]["pb"] is None


def test_daily_pairs_hfq():
    """按股票：raw 东财中文列 + hfq 收盘 = 收盘 × 复权因子"""
    raw, hfq = tushare.daily_pairs(FakePro(), "600519",
                                   date(2026, 8, 21), date(2026, 8, 21))
    assert list(raw.columns) == ["日期", "开盘", "最高", "最低", "收盘", "成交量", "成交额"]
    assert raw.iloc[0]["成交额"] == 1.5e9
    assert hfq.iloc[0]["收盘"] == 1510.0 * 12.3456

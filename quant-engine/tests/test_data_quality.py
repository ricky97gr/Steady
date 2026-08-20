"""数据质量检查单测（Issue #6）：sqlite 种子 + 逐项注入异常，断言检查电池正确上报"""
from datetime import date, timedelta

import pytest
from sqlalchemy import delete, select
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool

from app.data_quality import (check_data_quality, _board_limit)
from app.models.tables import (Base, DailyPrice, DailyValuation,
                               FinancialIndicator, StockBasic, TradeCalendar)

TD = date(2026, 8, 21)
PREV = date(2026, 8, 20)
# 池内股票：3 只（hs300）；非池股票 300001 仅用于价格检查全覆盖
POOL = {"600519", "600001", "000002"}


@pytest.fixture
def db():
    engine = create_engine("sqlite://", poolclass=StaticPool)
    Base.metadata.create_all(engine)
    session = sessionmaker(bind=engine)()
    seed(session)
    return session


def seed(session):
    for i, code in enumerate(["600519", "600001", "000002", "300001"]):
        universe = "hs300" if code in POOL else ""
        session.add(StockBasic(code=code, name=f"股{i}", universe=universe,
                               list_date=date(2020, 1, 1)))
    for d in (PREV, TD):
        session.add(TradeCalendar(cal_date=d, is_open=True))
    # BigInteger 主键在 sqlite 不自动递增，需显式传 id（与 test_backtest.py 同模式）
    pid = 0
    for code, base in [("600519", 1800.0), ("600001", 10.0), ("000002", 20.0),
                       ("300001", 30.0), ("sh000300", 4000.0)]:
        for d, px in ((PREV, base), (TD, base * 1.01)):
            pid += 1
            session.add(DailyPrice(id=pid, code=code, trade_date=d, open=px,
                                   high=px * 1.02, low=px * 0.98,
                                   close=px, volume=10000, amount=1000000.0))
    vid = fid = 0
    for code in POOL:
        vid += 1
        session.add(DailyValuation(id=vid, code=code, trade_date=TD,
                                   close=10, pe_ttm=15, pb=2))
        fid += 1
        session.add(FinancialIndicator(id=fid, code=code,
                                       report_date=date(2026, 6, 30),
                                       announce_date=date(2026, 8, 10),
                                       roe=10, pe=10, pb=2))
    session.commit()


def _bar(session, code, d=TD):
    return session.execute(select(DailyPrice).where(
        DailyPrice.code == code, DailyPrice.trade_date == d)).scalar_one()


def _set_close_valid(session, code, new_close):
    """改 close 并同步 high/low，保证 bar 合法（只测涨跌幅越界）"""
    bar = _bar(session, code)
    bar.close = new_close
    open_px = float(bar.open)
    bar.high = max(open_px, new_close) * 1.01
    bar.low = min(open_px, new_close) * 0.99
    session.commit()


# ---------- 用例 ----------

def test_clean_db_all_ok(db):
    r = check_data_quality(db, TD)
    assert r["overall"] == "ok"
    assert r["checks_total"] == 7
    assert r["ok"] == 7 and r["warn"] == 0 and r["fail"] == 0


def test_coverage_fail(db):
    # 新增池内股票只有 PREV 行情，无 TD bar → 覆盖 3/4=75%
    db.add(StockBasic(code="000004", name="缺行情", universe="hs300",
                      list_date=date(2020, 1, 1)))
    db.add(DailyPrice(id=99, code="000004", trade_date=PREV, open=10, high=10,
                      low=10, close=10, volume=100, amount=1000.0))
    db.add(FinancialIndicator(id=99, code="000004", report_date=date(2026, 6, 30),
                              announce_date=date(2026, 8, 10), roe=10, pe=10, pb=2))
    db.commit()
    r = check_data_quality(db, TD)
    d = r["check_details"]["coverage"]
    assert d["with_bar"] == 3 and d["pool"] == 4 and d["pct"] == 75.0
    assert r["overall"] == "fail"


def test_missing_trading_day(db):
    gap = date(2026, 8, 19)
    db.add(TradeCalendar(cal_date=gap, is_open=True))
    db.commit()
    r = check_data_quality(db, TD)
    assert r["check_details"]["missing_days"]["missing"] == [str(gap)]
    assert r["overall"] == "fail"


def test_duplicates_detected(db):
    bar = _bar(db, "600519")
    db.add(DailyPrice(id=98, code=bar.code, trade_date=bar.trade_date,
                      close=bar.close, volume=1, amount=1.0))
    db.commit()
    r = check_data_quality(db, TD)
    assert r["check_details"]["duplicates"]["daily_price"] == 1
    assert r["overall"] == "fail"


def test_price_anomaly_close_zero(db):
    bar = _bar(db, "600519")
    bar.close = 0
    db.commit()
    r = check_data_quality(db, TD)
    d = r["check_details"]["price_anomalies"]
    assert d["fail_count"] >= 1
    assert any("收盘价 0" in s for s in d["samples"])
    assert r["overall"] == "fail"


def test_price_board_limit_warn(db):
    _set_close_valid(db, "600519", 1800.0 * 1.15)  # 主板 +15% 越 10% 限但 ≤30.5
    r = check_data_quality(db, TD)
    d = r["check_details"]["price_anomalies"]
    assert d["fail_count"] == 0 and d["warn_count"] >= 1
    assert r["overall"] == "warn"


def test_price_hard_limit_fail(db):
    _set_close_valid(db, "600519", 1800.0 * 1.40)  # +40% 超硬上限
    r = check_data_quality(db, TD)
    assert r["check_details"]["price_anomalies"]["fail_count"] >= 1
    assert r["overall"] == "fail"


def test_valuation_stale(db):
    # 估值只到 PREV（落后 1 天 → warn）
    for i, code in enumerate(POOL):
        db.add(DailyValuation(id=50 + i, code=code, trade_date=PREV,
                              close=10, pe_ttm=15, pb=2))
    db.execute(delete(DailyValuation).where(DailyValuation.trade_date == TD))
    db.commit()
    r = check_data_quality(db, TD)
    d = r["check_details"]["valuation"]
    assert d["lag_days"] == 1
    assert r["overall"] == "warn"


def test_financial_stale(db):
    fin = db.execute(select(FinancialIndicator).where(
        FinancialIndicator.code == "000002")).scalar_one()
    fin.announce_date = TD - timedelta(days=400)
    db.commit()
    r = check_data_quality(db, TD)
    d = r["check_details"]["financial"]
    assert d["coverage_pct"] < 90
    assert r["overall"] == "fail"


def test_benchmark_stale(db):
    db.execute(delete(DailyPrice).where(
        DailyPrice.code == "sh000300", DailyPrice.trade_date == TD))
    db.commit()
    r = check_data_quality(db, TD)
    d = r["check_details"]["benchmark"]
    assert d["lag_days"] == 1
    assert r["overall"] == "fail"


def test_board_limit():
    assert _board_limit("600519") == 10.0   # 主板
    assert _board_limit("300001") == 20.0   # 创业板
    assert _board_limit("688001") == 20.0   # 科创板
    assert _board_limit("830001") == 30.0   # 北交所


def test_no_market_data(db):
    db.execute(delete(DailyPrice))
    db.commit()
    r = check_data_quality(db, None)
    assert r["checks_total"] == 0 and r["overall"] == "fail"

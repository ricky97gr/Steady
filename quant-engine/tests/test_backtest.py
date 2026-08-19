"""回测可复现性测试：同 DB 跑两遍，NAV 序列与报告完全一致"""
from datetime import date, timedelta

import pytest
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool

from app.backtest.engine import BacktestEngine
from app.backtest.replay import ReplayStrategy
from app.models.tables import (Base, DailyPrice, DailyValuation,
                               FinancialIndicator, StockBasic, TradeCalendar)

WEIGHTS = {"ma_trend": 0.20, "macd_signal": 0.20, "pe_ratio": 0.15,
           "pb_ratio": 0.15, "roe_quality": 0.20, "debt_risk": 0.10}
CATEGORIES = {"ma_trend": "trend", "macd_signal": "trend", "pe_ratio": "value",
              "pb_ratio": "value", "roe_quality": "quality", "debt_risk": "risk"}
CODES = ["600001", "600002", "600003"]
DAYS = [date(2026, 8, 10 + i) for i in range(8)]
# 回测区间前的价格 warmup（MA20/MACD 需要足够历史，否则趋势因子全 NaN）
WARMUP = [date(2026, 7, 1) + timedelta(days=i) for i in range(30)]


@pytest.fixture
def db():
    engine = create_engine("sqlite://", poolclass=StaticPool)
    Base.metadata.create_all(engine)
    session = sessionmaker(bind=engine)()
    seed(session)
    return session


def seed(session):
    for d in DAYS:
        session.add(TradeCalendar(cal_date=d, is_open=True))
    # 基准指数（沪深300，用于超额收益对比）
    for i, d in enumerate(DAYS):
        session.add(DailyPrice(id=1000 + i, code="sh000300",
                               trade_date=d, close=3000 + i))
    # 股票池：价格单调上行（趋势同质），估值/财务拉开横截面差距
    for ci, code in enumerate(CODES):
        # warmup 段（区间前，趋势因子窗口）
        for i, d in enumerate(WARMUP):
            session.add(DailyPrice(id=5000 + ci * 100 + i, code=code,
                                   trade_date=d,
                                   close=10 + ci + i * 0.01, adj_factor=1.0))
        for i, d in enumerate(DAYS):
            session.add(DailyPrice(id=2000 + ci * 100 + i, code=code,
                                   trade_date=d,
                                   close=10 + ci + i * 0.1, adj_factor=1.0))
            session.add(DailyValuation(id=3000 + ci * 100 + i, code=code,
                                       trade_date=d,
                                       pe_ttm=10 + ci * 5, pb=1 + ci))
        session.add(FinancialIndicator(id=4000 + ci, code=code,
                                       report_date=date(2026, 6, 30),
                                       announce_date=date(2026, 8, 12),
                                       roe=15 + ci, debt_ratio=40 + ci))
        session.add(StockBasic(code=code, name=f"测试{ci}", market="SH",
                               universe="hs300"))
    session.commit()


def run_engine(db):
    """全新引擎实例跑一遍完整回测（独立持仓/组合状态）"""
    strat = ReplayStrategy(db, {"top_n": 2, "buy_buffer": 1, "sell_buffer": 2},
                           WEIGHTS, CATEGORIES)
    engine = BacktestEngine(strat, "2026-08-10", "2026-08-17", db=db)
    report = engine.run()
    return engine.daily_returns, report


def test_backtest_reproducible_nav_and_report(db):
    """同 DB 两次独立回测 → 净值序列与报告逐字段一致"""
    returns1, report1 = run_engine(db)
    returns2, report2 = run_engine(db)
    assert [r["nav"] for r in returns1] == [r["nav"] for r in returns2]
    assert returns1 == returns2
    assert report1 == report2


def test_backtest_executes_trades(db):
    """信号确实成交（有交易发生），报告字段完整"""
    returns, report = run_engine(db)
    assert report["trades"] >= 1
    assert report["trading_days"] == len(DAYS)
    assert report["final_value"] > 0
    assert report["portfolio"]["total_return"] is not None
    # 基准对比：sh000300 有数据 → 超额收益存在
    assert "benchmark" in report and "excess_return" in report
    # 净值首日 = 初始资金 10 万（首日信号 BUY 生效于当日收盘价成交）
    assert returns[0]["nav"] > 0


def test_replay_signal_actions_valid(db):
    """重放生成的信号 action 均在 BUY/SELL/HOLD 内，评分 > 0"""
    strat = ReplayStrategy(db, {"top_n": 2, "buy_buffer": 1, "sell_buffer": 2},
                           WEIGHTS, CATEGORIES)
    strat.preload("2026-08-10", "2026-08-17")
    for d in DAYS:
        signals = strat.run(d.isoformat())
        for sig in signals:
            assert sig.action in ("BUY", "SELL", "HOLD")
            assert sig.score > 0

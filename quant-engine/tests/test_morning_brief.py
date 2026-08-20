"""早盘简报单测（Issue #4）：sqlite 种子 → 全量组装 / 空库兜底 / 非交易日 / 卡片渲染"""
from datetime import date

import pytest
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool

from app.morning_brief import CHECKLIST, _is_open, assemble_brief
from app.models.tables import (
    AccountNav,
    Base,
    MarketHotspot,
    Position,
    StockBasic,
    StrategySignal,
    TaskRun,
    TradeCalendar,
)
from app.notify_scheduler import _morning_brief_content

TD = date(2026, 8, 21)   # 简报日（今日）
PREV = date(2026, 8, 20)  # 回顾日


@pytest.fixture
def db():
    engine = create_engine("sqlite://", poolclass=StaticPool)
    Base.metadata.create_all(engine)
    session = sessionmaker(bind=engine)()
    seed(session)
    return session


def seed(session):
    session.add(StockBasic(code="600519", name="贵州茅台", universe="hs300"))
    session.add(TradeCalendar(cal_date=TD, is_open=True))
    session.add(MarketHotspot(spot_date=TD, sections={
        "indices": [{"name": "道琼斯", "code": ".DJI", "close": 53463.05, "change_pct": 0.22}],
        "sectors_gain": [{"name": "生物制品", "change_pct": 9.12, "leader": "三元基因"}],
        "sectors_flow": [{"name": "养殖业", "net_inflow": "7.82亿"}],
        "hot_stocks": [{"rank": 1, "code": "000931", "name": "中关村", "change_pct": 9.98,
                        "board_days": 2, "industry": "化学制药"}],
    }))
    # BigInteger 主键 sqlite 不自动递增，显式传 id（与 test_backtest.py 同模式）
    session.add(StrategySignal(id=1, strategy_name="multi_factor", code="600519",
                               trade_date=PREV, score=80.0, action="BUY"))
    session.add(TaskRun(id=1, task_name="auto_trade", run_date=PREV, status="success",
                        detail={"buy_count": 1, "sell_count": 0,
                                "orders": [{"code": "600519", "direction": "BUY",
                                            "price": 1610.0, "quantity": 100}]}))
    session.add(TaskRun(id=2, task_name="data_quality", run_date=PREV, status="success",
                        detail={"overall": "ok", "fail": 0, "warn": 0,
                                "message": "7 项检查全部通过"}))
    session.add(TaskRun(id=3, task_name="generate_signals", run_date=PREV, status="success",
                        message="生成 1 条信号"))
    session.add(AccountNav(id=1, account_id=1, trade_date=PREV, nav=1.05, daily_return=0.012,
                           drawdown=-0.02, total_asset=110000))
    session.add(Position(id=1, account_id=1, code="600519", quantity=100,
                         available_qty=100, market_value=161000, profit_rate=0.05))
    session.commit()


def test_assemble_full_hit(db):
    hotspot = db.query(MarketHotspot).one().sections
    sections = assemble_brief(db, TD, PREV, hotspot)

    assert sections["brief_date"] == "2026-08-21"
    assert sections["trade_date"] == "2026-08-20"
    assert sections["is_open_today"] is True
    # market 透传
    assert sections["market"]["indices"][0]["name"] == "道琼斯"
    assert sections["market"]["hot_stocks"][0]["board_days"] == 2
    # yesterday
    y = sections["yesterday"]
    assert y["signal"]["total"] == 1 and y["signal"]["counts"]["BUY"] == 1
    assert y["signal"]["top_buys"] == ["600519"]
    assert y["trade"]["buy_count"] == 1 and y["trade"]["orders"][0]["code"] == "600519"
    assert y["nav"]["nav"] == 1.05 and y["nav"]["daily_return"] == 0.012
    assert y["data_health"]["overall"] == "ok"
    assert [t["task_name"] for t in y["tasks"]] == [
        "auto_trade", "data_quality", "generate_signals"]
    # today
    assert sections["today"]["checklist"] == [{"time": t, "task": n} for t, n in CHECKLIST]
    assert sections["today"]["positions"][0]["name"] == "贵州茅台"


def test_assemble_empty_db(db):
    for m in (StrategySignal, AccountNav, Position, MarketHotspot):
        db.query(m).delete()
    for tr in db.query(TaskRun).all():  # auto_trade 置「无交易动作」，其余删
        if tr.task_name == "auto_trade":
            tr.detail = {"skipped": True, "message": "无交易动作"}
        else:
            db.delete(tr)
    db.commit()

    sections = assemble_brief(db, TD, PREV, {})
    assert sections["market"] == {}
    y = sections["yesterday"]
    assert y["signal"]["total"] == 0 and y["signal"]["counts"] == {}
    assert y["trade"]["buy_count"] == 0 and y["trade"]["orders"] == []
    assert y["nav"] == {}
    assert y["data_health"]["overall"] == "none"
    assert sections["today"]["positions"] == []


def test_non_trading_day(db):
    assert _is_open(db, TD) is True
    closed = date(2026, 8, 22)
    db.add(TradeCalendar(cal_date=closed, is_open=False))
    db.commit()
    assert _is_open(db, closed) is False
    # 非交易日组装的简报 is_open_today=False（job 依此 skip 并记 skipped）
    sections = assemble_brief(db, closed, PREV, {})
    assert sections["is_open_today"] is False


def test_card_render_smoke(db):
    hotspot = db.query(MarketHotspot).one().sections
    sections = assemble_brief(db, TD, PREV, hotspot)
    content = _morning_brief_content(db, TD, sections)

    assert "交易日 2026-08-21（开市）" in content
    assert "回顾 2026-08-20" in content
    assert "道琼斯 +0.22%" in content          # 市场节已是百分比数值
    assert "生物制品 +9.12%" in content
    assert "中关村" in content and "最高连板 2 板" in content
    assert "信号1条" in content and "买1卖0" in content
    assert "数据通过" in content
    assert "今日计划" in content
    assert "持仓 **1** 只" in content

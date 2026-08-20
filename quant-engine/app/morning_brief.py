"""早盘简报生成（Issue #4）：quant-engine 09:10 组装落 morning_brief 表
供 notify_scheduler 09:15 推送 + backend 只读接口 + 前端 /brief 页。

数据全部来自现有表 + market_hotspot（collector 08:45 采集落库）：
- 回顾日 td = 最近一个已有行情数据的交易日（通常 T-1）
- market 节：market_hotspot.sections（隔夜外盘/板块涨幅与资金流/活跃个股）
- yesterday 节：strategy_signal（信号）/ order、auto_trade 台账（成交）/
  account_nav、nav_snapshot 台账（净值）/ data_quality 台账（数据健康）/ task_run 全量
- today 节：静态时间清单 + position 持仓（股票名联 stock_basic）

sections JSONB 结构（backend/frontend 按此解析）：
{
  brief_date, trade_date, is_open_today,
  market:  { indices, sectors_gain, sectors_flow, hot_stocks },   ← 来自 market_hotspot
  yesterday: { signal:{total,counts,top_buys[]},
               trade:{buy_count,sell_count,orders[],message?},
               nav:{nav,daily_return,drawdown,total_asset},
               data_health:{overall,fail,warn,message},
               tasks:[{task_name,status,message}] },
  today:   { checklist:[{time,task}], positions:[{code,name,quantity,market_value,profit_rate}] }
}
"""
import logging
from datetime import date

from sqlalchemy import func, select, text

from app.db import get_session, upsert
from app.models.tables import (
    AccountNav,
    DailyPrice,
    MarketHotspot,
    MorningBrief,
    Position,
    StockBasic,
    StrategySignal,
    TaskRun,
    TradeCalendar,
)
from app.task_run import record_task

logger = logging.getLogger(__name__)

# 今日计划静态清单（收盘后任务；与 collector/engine 调度时间表一致）
CHECKLIST = [
    ("16:15", "指数同步"), ("16:30", "行情同步"), ("16:45", "估值同步"),
    ("18:00", "财务同步"), ("18:30", "数据健康检查"), ("19:00", "因子计算"),
    ("19:30", "策略信号"), ("19:35", "自动交易"), ("21:05", "净值快照"),
]


def _latest_trade_date(db) -> date | None:
    """最近一个已有行情数据的交易日（跳过指数伪股票），语义与 app.tasks 一致"""
    return db.execute(
        select(func.max(DailyPrice.trade_date)).where(DailyPrice.code.not_like("sh%"))
    ).scalar()


def _is_open(db, d: date) -> bool:
    """今日是否开市：交易日历（collector 09:05 已同步）"""
    return bool(db.execute(
        select(TradeCalendar.is_open).where(TradeCalendar.cal_date == d)
    ).scalar())


def _pct_str(v) -> str:
    if v is None:
        return ""
    s = "+" if v > 0 else ""
    return f"{s}{v:.2f}%"


# ---------- 分节组装（各自查询，单节失败不影响整体） ----------

def _signal_section(db, td: date) -> dict:
    rows = db.execute(
        select(StrategySignal.action, func.count())
        .where(StrategySignal.trade_date == td)
        .group_by(StrategySignal.action)
    ).all()
    counts = {a: c for a, c in rows}
    top_buys = [r[0] for r in db.execute(
        select(StrategySignal.code).where(
            StrategySignal.trade_date == td, StrategySignal.action == "BUY")
        .order_by(StrategySignal.score.desc()).limit(5)
    ).all()]
    return {"total": sum(counts.values()), "counts": counts, "top_buys": top_buys}


def _trade_section(db, td: date) -> dict:
    """昨日成交：优先 auto_trade 台账 detail（含成交明细），回退 order 表当日"""
    row = db.execute(
        select(TaskRun).where(TaskRun.task_name == "auto_trade", TaskRun.run_date == td)
    ).scalar()
    if row and row.status == "success" and row.detail:
        d = row.detail
        if d.get("skipped"):
            return {"buy_count": 0, "sell_count": 0, "orders": [],
                    "message": d.get("message", "无交易动作")}
        return {"buy_count": d.get("buy_count", 0), "sell_count": d.get("sell_count", 0),
                "orders": d.get("orders") or []}
    rows = db.execute(text(
        'SELECT code, direction, price, quantity FROM "order" '
        'WHERE created_at::date = :d ORDER BY id DESC'), {"d": td}).all()
    orders = [{"code": r.code, "direction": r.direction,
               "price": float(r.price) if r.price is not None else None,
               "quantity": r.quantity} for r in rows]
    return {"buy_count": sum(1 for o in orders if o["direction"] == "BUY"),
            "sell_count": sum(1 for o in orders if o["direction"] == "SELL"),
            "orders": orders}


def _nav_section(db, td: date) -> dict:
    """昨日净值：优先 nav_snapshot 台账 detail，回退 account_nav 表"""
    row = db.execute(
        select(TaskRun).where(TaskRun.task_name == "nav_snapshot", TaskRun.run_date == td)
    ).scalar()
    if row and row.status == "success" and row.detail:
        d = row.detail
        return {"nav": d.get("nav"), "daily_return": d.get("daily_return"),
                "drawdown": d.get("drawdown"), "total_asset": d.get("total_asset")}
    nav = db.execute(
        select(AccountNav).where(AccountNav.trade_date == td)
        .order_by(AccountNav.id.desc()).limit(1)
    ).scalar()
    if nav is None:
        return {}
    return {
        "nav": float(nav.nav) if nav.nav is not None else None,
        "daily_return": float(nav.daily_return) if nav.daily_return is not None else None,
        "drawdown": float(nav.drawdown) if nav.drawdown is not None else None,
        "total_asset": float(nav.total_asset) if nav.total_asset is not None else None,
    }


def _data_health_section(db, td: date) -> dict:
    row = db.execute(
        select(TaskRun).where(TaskRun.task_name == "data_quality", TaskRun.run_date == td)
    ).scalar()
    if row is None or row.status != "success" or not row.detail:
        return {"overall": "none", "fail": 0, "warn": 0, "message": "未执行"}
    d = row.detail
    return {"overall": d.get("overall", "ok"), "fail": d.get("fail", 0),
            "warn": d.get("warn", 0), "message": d.get("message", "")}


def _tasks_section(db, td: date) -> list[dict]:
    return [{"task_name": r.task_name, "status": r.status, "message": r.message}
            for r in db.execute(
                select(TaskRun.task_name, TaskRun.status, TaskRun.message)
                .where(TaskRun.run_date == td).order_by(TaskRun.id)).all()]


def _positions_section(db) -> list[dict]:
    return [{"code": r.code, "name": r.name or "", "quantity": r.quantity,
             "market_value": float(r.market_value) if r.market_value is not None else None,
             "profit_rate": float(r.profit_rate) if r.profit_rate is not None else None}
            for r in db.execute(
                select(Position.code, Position.quantity, Position.market_value,
                       Position.profit_rate, StockBasic.name)
                .outerjoin(StockBasic, StockBasic.code == Position.code)).all()]


def assemble_brief(db, today: date, td: date, hotspot: dict) -> dict:
    """组装早报 sections；hotspot 为 market_hotspot.sections（缺则为 {}）"""
    return {
        "brief_date": str(today),
        "trade_date": str(td),
        "is_open_today": _is_open(db, today),
        "market": hotspot or {},
        "yesterday": {
            "signal": _signal_section(db, td),
            "trade": _trade_section(db, td),
            "nav": _nav_section(db, td),
            "data_health": _data_health_section(db, td),
            "tasks": _tasks_section(db, td),
        },
        "today": {
            "checklist": [{"time": t, "task": name} for t, name in CHECKLIST],
            "positions": _positions_section(db),
        },
    }


def _summary(sections: dict) -> str:
    """简短摘要（task_run.message，页面/日志展示）"""
    market = sections.get("market") or {}
    ind = " · ".join(f"{x.get('name', '')}{_pct_str(x.get('change_pct'))}"
                     for x in (market.get("indices") or [])[:3])
    hot = " · ".join(x.get("name", "") for x in (market.get("hot_stocks") or [])[:3])
    sig = (sections.get("yesterday") or {}).get("signal") or {}
    return f"外盘 {ind or 'N/A'}；热点 {hot or 'N/A'}；昨日信号 {sig.get('total', 0)} 条"


def job_morning_brief() -> None:
    """09:10 组装早盘简报：非交易日/无行情 skip；market 节取当日热点（缺则最近一天）"""
    db = get_session()
    today = date.today()
    try:
        if not _is_open(db, today):
            record_task(db, "morning_brief", today, "skipped", "非交易日")
            logger.info("今日非交易日，跳过早盘简报")
            return
        td = _latest_trade_date(db)
        if td is None:
            record_task(db, "morning_brief", today, "skipped", "无行情数据")
            logger.warning("无行情数据，跳过早盘简报")
            return
        spot = db.execute(
            select(MarketHotspot).where(MarketHotspot.spot_date == today)
        ).scalar()
        if spot is None:  # 补数据/采集失败场景：退最近一天热点
            spot = db.execute(
                select(MarketHotspot).order_by(MarketHotspot.spot_date.desc()).limit(1)
            ).scalar()
        hotspot = spot.sections if spot else {}
        sections = assemble_brief(db, today, td, hotspot)
        upsert(db, MorningBrief, [{"brief_date": today, "sections": sections}],
               conflict_cols=["brief_date"], update_cols=["sections"])
        summary = _summary(sections)
        record_task(db, "morning_brief", today, "success", summary, detail=sections)
        logger.info("早盘简报生成完成 %s：%s", today, summary)
    except Exception:
        logger.exception("早盘简报任务失败")
        db.rollback()
        record_task(db, "morning_brief", today, "failed", "早盘简报生成异常")
    finally:
        db.close()


def sync_morning_brief() -> bool:
    """手动触发入口（调试/补数据）：执行今日早报"""
    job_morning_brief()
    return True


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
    job_morning_brief()

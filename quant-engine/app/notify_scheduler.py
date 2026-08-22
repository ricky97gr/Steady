"""通知调度器：每 1 分钟 tick，按 notify_config 配置推送飞书卡片

职责：
- 定时事件（weekday / trading_day）：到点且今日未发送 → 推当日卡片；
  源任务缺失 → 宽限 30 分钟后推红色「未执行」告警
- 事件型：backtest 由 backtest_service.run_and_save 直接推送；
  task_alert 在此检测当日失败任务并推送（每任务每日一次）
- 去重：所有推送落 task_run 表（notify:{event} / alert:{task_name}），保证每日一次；
  内容优先取源任务 task_run.detail（结构化、LLM-ready），缺省回退直查业务表
"""
import logging
from datetime import date, datetime, timedelta

from sqlalchemy import func, select, text

from app.db import get_session
from app.models.tables import DailyPrice, NotifyConfig, StrategySignal, StockBasic, TaskRun, TradeCalendar
from app.notify import FeishuNotifier, load_config
from app.task_run import already_run, record_task

logger = logging.getLogger("notify_scheduler")

# 事件 → 数据源任务（backend 或本引擎写 task_run）
SOURCE_TASK = {
    "signal": "generate_signals",
    "auto_trade": "auto_trade",
    "nav": "nav_snapshot",
    "daily_report": "daily_report",
    "data_quality": "data_quality",
    "morning_brief": "morning_brief",
}

# 检查级别 → 卡片行前缀图标
_LVL_ICON = {"ok": "✅", "warn": "⚠️", "fail": "❌"}

# 源任务缺失的宽限期（分钟），超过仍未执行才发「未执行」告警
GRACE_MINUTES = 30

_ACTION_CN = {"BUY": "买入", "SELL": "卖出", "HOLD": "持有"}


def _fmt_pct(x) -> str:
    if x is None:
        return "N/A"
    s = "+" if x > 0 else ""
    return f"{s}{x * 100:.2f}%"


def _code_names(db, codes) -> dict:
    if not codes:
        return {}
    return {r.code: r.name for r in db.execute(
        select(StockBasic.code, StockBasic.name).where(StockBasic.code.in_(codes))
    ).all() if r.name}


def _source_detail(db, task_name: str, td: date):
    """读源任务当日执行记录：返回 (status, detail)；从未执行返回 (None, None)"""
    row = db.execute(
        select(TaskRun).where(TaskRun.task_name == task_name, TaskRun.run_date == td)
    ).scalar()
    if row is None:
        return None, None
    return row.status, (row.detail or {})


# ---------- 内容构建（detail 优先，回退直查业务表） ----------

def _signal_content(db, td: date, detail: dict) -> str:
    if detail:
        counts = detail.get("counts") or {}
        total = detail.get("total") or sum(counts.values())
        buys = detail.get("top_buys") or []
        tdate = detail.get("trade_date") or str(td)
    else:
        rows = db.execute(
            select(StrategySignal.action, func.count())
            .where(StrategySignal.trade_date == td)
            .group_by(StrategySignal.action)
        ).all()
        counts = {a: c for a, c in rows}
        total = sum(counts.values())
        buys = [r[0] for r in db.execute(
            select(StrategySignal.code).where(
                StrategySignal.trade_date == td,
                StrategySignal.action == "BUY")
            .order_by(StrategySignal.score.desc()).limit(5)
        ).all()]
        tdate = str(td)
    lines = [f"**交易日期** {tdate}",
             f"**信号总数** {total} 条",
             "　".join(f"{_ACTION_CN.get(a, a)} **{c}**" for a, c in counts.items())]
    if buys:
        names = _code_names(db, buys)
        lines += ["", "**买入前五**"]
        lines += [f"{i}. {code} {names.get(code, '')}".rstrip()
                  for i, code in enumerate(buys, 1)]
    return "**策略信号**\n\n" + "\n".join(lines)


def _auto_trade_content(db, td: date, detail: dict) -> str:
    if detail:
        tdate = detail.get("trade_date") or str(td)
        if detail.get("skipped"):
            return (f"**自动交易**\n\n{tdate} 无交易动作"
                    f"（{detail.get('message', '信号无成交/已跳过')}）")
        buy = detail.get("buy_count", 0)
        sell = detail.get("sell_count", 0)
        orders = detail.get("orders") or []
        lines = [f"**交易日期** {tdate}",
                 f"买入 **{buy}** 笔 · 卖出 **{sell}** 笔"]
        if orders:
            lines += ["", "**成交明细**"]
            for o in orders[:5]:
                arrow = "买入" if o.get("direction") == "BUY" else "卖出"
                lines.append(f"• {arrow} {o.get('code', '')} "
                             f"{o.get('quantity', '')}股 @ {o.get('price', '')}")
        return "**自动交易结果**\n\n" + "\n".join(lines)
    # 回退：order 表当日汇总（order 为 SQL 关键字）
    rows = db.execute(text(
        'SELECT direction, COUNT(*) FROM "order" '
        "WHERE created_at::date = :d GROUP BY direction"
    ), {"d": td}).all()
    if not rows:
        return f"**自动交易结果**\n\n{td} 无委托单。"
    parts = "　".join(f"{_ACTION_CN.get(a, a)} **{c}**" for a, c in rows)
    return f"**自动交易结果**\n\n**交易日期** {td}\n{parts}"


def _nav_content(db, td: date, detail: dict) -> str:
    if detail:
        tdate = detail.get("trade_date") or str(td)
        nav = detail.get("nav")
        daily = detail.get("daily_return")
        dd = detail.get("drawdown")
        asset = detail.get("total_asset")
    else:
        row = db.execute(text(
            "SELECT nav, daily_return, drawdown, total_asset FROM account_nav "
            "WHERE trade_date = :d ORDER BY id DESC LIMIT 1"
        ), {"d": td}).first()
        if row is None:
            return f"**账户净值**\n\n{td} 尚无净值快照。"
        nav, daily, dd, asset = row
        tdate = str(td)
    lines = [f"**交易日期** {tdate}",
             f"**净值** {nav if nav is not None else 'N/A'}",
             f"**当日收益** {_fmt_pct(daily)}",
             f"**累计回撤** {_fmt_pct(dd)}"]
    if asset is not None:
        lines.append(f"**总资产** {asset:,.2f} 元")
    return "**账户净值**\n\n" + "\n".join(lines)


def _daily_report_content(db, td: date, detail: dict) -> str:
    """日报：汇总信号 / 交易 / 净值 / 行情就绪状态（由调度器生成）"""
    sig_status, sig_detail = _source_detail(db, "generate_signals", td)
    trade_status, trade_detail = _source_detail(db, "auto_trade", td)
    nav_status, nav_detail = _source_detail(db, "nav_snapshot", td)

    signal_part = "未执行"
    if sig_status == "success":
        counts = (sig_detail or {}).get("counts") or {}
        total = (sig_detail or {}).get("total") or sum(counts.values())
        signal_part = f"{total} 条" + ("　".join(
            f"{_ACTION_CN.get(a, a)}{c}" for a, c in counts.items()) or "")
    trade_part = "未执行"
    if trade_status == "success":
        d = trade_detail or {}
        if d.get("skipped"):
            trade_part = "无交易动作"
        else:
            trade_part = f"买 {d.get('buy_count', 0)} / 卖 {d.get('sell_count', 0)}"
    nav_part = "未执行"
    if nav_status == "success":
        d = nav_detail or {}
        nav_part = f"{d.get('nav', 'N/A')}（{_fmt_pct(d.get('daily_return'))}）"

    lines = [
        f"**交易日** {td}",
        "**信号** " + signal_part,
        "**交易** " + trade_part,
        "**净值** " + nav_part,
    ]
    return "**今日日报**\n\n" + "\n".join(lines)


def _data_quality_content(db, td: date, detail: dict) -> tuple[str, str]:
    """数据健康卡片：返回 (content, template)。
    模板由检查结果驱动（fail 红 / warn 蓝 / ok 绿）；detail 来自 data_quality 台账。"""
    results = detail.get("results") or []
    overall = detail.get("overall") or "ok"
    date_str = detail.get("trade_date") or str(td)
    lines = [f"**交易日** {date_str} {_LVL_ICON.get(overall, '')}", ""]
    lines += [f"{_LVL_ICON.get(r.get('level'), '•')} {r.get('message', '')}"
              for r in results]
    n_total = detail.get("checks_total") or len(results)
    if overall == "ok":
        lines += ["", f"{n_total} 项检查全部通过。"]
    elif overall == "fail":
        lines += ["", f"发现 {detail.get('fail', 0)} 项异常，请检查 collector 日志与数据源。"]
    else:
        lines += ["", f"{detail.get('warn', 0)} 项警告，请留意。"]
    template = {"ok": "green", "warn": "blue", "fail": "red"}[overall]
    return "**数据健康检查**\n\n" + "\n".join(lines), template


def _morning_brief_content(db, td: date, detail: dict) -> str:
    """早盘简报卡片：隔夜外盘 + 热点板块 + 活跃个股 + 昨日回顾 + 今日计划。
    detail 为 morning_brief 台账 detail（即 assemble_brief 的 sections 结构）。
    注意：市场节 change_pct 已是百分比数值（0.22=+0.22%），与净值 daily_return 的小数单位不同，
    用本地 _pct 格式化（_fmt_pct 会再乘 100，只适用于小数单位）。"""

    def _pct(v):
        if v is None:
            return "N/A"
        s = "+" if v > 0 else ""
        return f"{s}{v:.2f}%"

    market = (detail or {}).get("market") or {}
    yesterday = (detail or {}).get("yesterday") or {}
    today = (detail or {}).get("today") or {}

    title = f"交易日 {(detail or {}).get('brief_date') or td}"
    title += "（开市）" if (detail or {}).get("is_open_today") else "（休市）"
    trade_date = (detail or {}).get("trade_date")
    if trade_date:
        title += f" · 回顾 {trade_date}"
    lines = [f"**{title}**"]

    indices = market.get("indices") or []
    if indices:
        lines.append("**隔夜外盘** " + " · ".join(
            f"{x.get('name', '')} {_pct(x.get('change_pct'))}" for x in indices[:4]))
    sectors = market.get("sectors_gain") or []
    if sectors:
        lines.append("**热点板块** " + " · ".join(
            f"{x.get('name', '')} {_pct(x.get('change_pct'))}" for x in sectors[:3]))
    hot = market.get("hot_stocks") or []
    if hot:
        names = "　".join(f"{i}. {x.get('name', '')}" for i, x in enumerate(hot[:3], 1))
        if hot[0].get("board_days"):
            names += f" · 最高连板 {max(h.get('board_days', 1) for h in hot[:3])} 板"
        lines.append("**活跃个股** " + names)

    sig = yesterday.get("signal") or {}
    trade = yesterday.get("trade") or {}
    nav = yesterday.get("nav") or {}
    health = yesterday.get("data_health") or {}
    sig_str = f"{sig.get('total', 0)}条" if sig.get("total") else "无"
    trade_str = (f"买{trade.get('buy_count', 0)}卖{trade.get('sell_count', 0)}"
                 if trade.get("buy_count") or trade.get("sell_count") else "无")
    nav_str = ("无净值" if not nav.get("nav")
               else f"{nav.get('nav')}({_fmt_pct(nav.get('daily_return'))})")
    health_icon = _LVL_ICON.get(health.get("overall", "none"), "•")
    health_str = {"ok": "通过", "warn": f"{health.get('warn', 0)}项警告",
                  "fail": f"{health.get('fail', 0)}项异常",
                  "none": "未执行"}.get(health.get("overall"), "未执行")
    lines += ["",
              f"**昨日回顾** 📈信号{sig_str} · 💹{trade_str} · 💰净值{nav_str} · "
              f"{health_icon}数据{health_str}"]

    checklist = today.get("checklist") or []
    if checklist:
        lines.append("**今日计划** " + " · ".join(
            f"{c.get('time', '')}{c.get('task', '')}" for c in checklist[:6]))
    positions = today.get("positions") or []
    if positions:
        mv = sum(p.get("market_value") or 0 for p in positions)
        lines.append(f"💼 持仓 **{len(positions)}** 只 · 市值 ¥{mv:,.0f}")
    return "**早盘简报**\n\n" + "\n".join(lines)


# ---------- 发送 ----------

def _send(db, notifier: FeishuNotifier, key: str, td: date,
          title: str, content: str, template: str = "blue",
          footer: str | None = None) -> None:
    if not notifier.ready:
        record_task(db, key, td, "skipped", "通知未启用或未配置 webhook")
        return
    ok = notifier.send_card(title, content, template=template, footer=footer)
    record_task(db, key, td, "success" if ok else "failed",
                "通知已投递" if ok else "通知发送失败")


def _maybe_send_scheduled(db, notifier: FeishuNotifier, ev: NotifyConfig, td: date) -> None:
    now = datetime.now()
    send_at = datetime.combine(td, ev.send_at)
    if now < send_at:
        return
    key = f"notify:{ev.event_key}"
    if already_run(db, key, td):
        return
    source = SOURCE_TASK.get(ev.event_key)
    status, detail = _source_detail(db, source, td) if source else (None, None)

    # 日报：由本调度器生成内容并记录数据源任务
    if ev.event_key == "daily_report":
        content = _daily_report_content(db, td, detail)
        _send(db, notifier, key, td, "Steady · 今日日报", content,
              template=ev.template or "blue", footer="日报由调度器自动生成")
        record_task(db, "daily_report", td, "success", "日报已生成")
        return

    # 源任务状态驱动：缺失→宽限后「未执行」；失败→失败卡片；成功→正常卡片
    if status is None:
        if now < send_at + timedelta(minutes=GRACE_MINUTES):
            return  # 宽限期内：等待源任务
        content = (f"**{ev.name} 未执行**\n\n"
                   f"任务 `{source}` 今日尚无执行记录（计划发送 {ev.send_at}）。\n"
                   f"可能原因：调度未启动 / 任务失败 / 行情缺失。")
        _send(db, notifier, key, td, "⚠️ Steady · 任务未执行", content,
              template="red", footer="该做没做 · 执行监控")
        return
    if status == "failed":
        content = (f"**{ev.name} 执行失败**\n\n任务 `{source}` 今日执行失败，"
                   f"详情见任务告警卡片。")
        _send(db, notifier, key, td, "❌ Steady · 执行失败", content,
              template="red", footer="执行监控")
        return

    # 数据健康：源任务执行即 success（发现问题是产出），卡片颜色由检查结果驱动
    if ev.event_key == "data_quality":
        content, template = _data_quality_content(db, td, detail)
        _send(db, notifier, key, td, "📋 Steady · 数据健康检查", content,
              template=template, footer="数据质量监控 · 每日 18:35")
        return

    # 早盘简报：源任务 success 才推（skipped=非交易日/无数据，不推）；
    # 未执行/失败已由上方通用分支处理（宽限告警/失败卡片）
    if ev.event_key == "morning_brief":
        if status != "success":
            return
        content = _morning_brief_content(db, td, detail)
        _send(db, notifier, key, td, "🌅 Steady · 早盘简报", content,
              template=ev.template or "blue", footer="早盘简报 · 每日 09:15")
        return

    builders = {"signal": _signal_content, "auto_trade": _auto_trade_content,
                "nav": _nav_content}
    titles = {"signal": "📈 Steady · 策略信号", "auto_trade": "💹 Steady · 自动交易",
              "nav": "💰 Steady · 账户净值"}
    builder = builders.get(ev.event_key)
    if builder is None:
        return
    content = builder(db, td, detail)
    _send(db, notifier, key, td, titles[ev.event_key], content,
          template=ev.template or "blue", footer="Steady 量化")


def _check_task_alerts(db, notifier: FeishuNotifier, td: date) -> None:
    """当日失败任务 → 红色告警卡片（每任务每日一次，按 alert:{task_name} 去重）"""
    failed = db.execute(
        select(TaskRun.task_name, TaskRun.message)
        .where(TaskRun.run_date == td, TaskRun.status == "failed")
    ).all()
    # consistency_check 由 backend 服务推送专属对账卡片，跳过通用告警避免重复红卡
    pending = [(name, msg) for name, msg in failed
               if name != "consistency_check"
               and not already_run(db, f"alert:{name}", td)]
    if not pending:
        return
    lines = ["**今日执行失败的任务**", ""]
    lines += [f"• `{name}`：{msg or '无错误信息'}" for name, msg in pending]
    lines += ["", "请检查日志，或在 Dashboard 查看任务状态。"]
    notifier.send_card("❌ Steady · 任务失败告警", "\n".join(lines),
                       template="red", footer="任务执行监控")
    for name, _ in pending:
        record_task(db, f"alert:{name}", td, "success", "已告警")


def _schedule_matches(db, ev: NotifyConfig, td: date) -> bool:
    if ev.schedule_type == "weekday":
        if not ev.weekdays:
            return False
        days = {w.strip() for w in ev.weekdays.split(",")}
        return str(td.isoweekday()) in days
    if ev.schedule_type == "trading_day":
        if ev.event_key == "morning_brief":
            # 早报：早晨判定「今日是否开市」（日历 09:05 已同步）。
            # 下方「最近行情 == 今日」在早晨恒 False（行情 16:30 才落地），必须特判。
            return bool(db.execute(
                select(TradeCalendar.is_open).where(TradeCalendar.cal_date == td)
            ).scalar())
        # 仅交易日触发：最近有行情数据的交易日 == 今天（周末/节假日不触发）
        latest = db.execute(
            select(func.max(DailyPrice.trade_date))
            .where(DailyPrice.code.not_like("sh%"))
        ).scalar()
        return latest == td
    return False  # event 型由各自触发点直接推送


def tick() -> None:
    """每 1 分钟调用一次：检查所有定时通知 + 失败告警"""
    db = get_session()
    try:
        cfg = load_config(db)
        if not cfg["enabled"]:
            return
        notifier = FeishuNotifier(cfg)
        td = date.today()
        for ev in db.execute(select(NotifyConfig)).scalars():
            if ev.enabled and ev.schedule_type != "event" \
                    and _schedule_matches(db, ev, td):
                try:
                    _maybe_send_scheduled(db, notifier, ev, td)
                except Exception:
                    db.rollback()
                    logger.exception("通知事件 %s 处理失败", ev.event_key)
        _check_task_alerts(db, notifier, td)
    finally:
        db.close()

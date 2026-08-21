"""Tushare Pro 数据源适配层（数据主源；token 从 app_config 表读取，页面可改）

设计原则（与 quant-engine/app/notify.py 的「业务配置全走页面，env 层只保留
数据库凭据」一致）：
- token 存 app_config 表键 `tushare.token`，前端设置页配置，值以库为准，
  本层**不读环境变量**；
- `make_pro()` 无 token 时返回 None → 采集器保持纯 AkShare；接口调用失败
  抛异常 → 采集器降级 AkShare（保住现有可靠性）；
- 各 `xxx_rows()` 返回与对应 AkShare 调用**同形状**的数据（上层 build_rows /
  save / 清洗 / 重试逻辑完全不动）；
- 提供两组接口：
  * 按股票（`daily_pairs` / `daily_basic_rows` / `fina_indicator_rows`）：
    复用现有逐只循环，用于回填 / 补缺 / 财务等低频场景；
  * 按日期全市场快照（`daily_snapshot` / `daily_basic_snapshot`）：
    Tushare `daily` / `daily_basic` 支持按 trade_date 一次返回全市场，
    每日增量 1~2 次调用即可覆盖全市场（120 积分日频限额内）。

注意：Tushare 积分决定接口可用范围与日频调用次数。低积分下按股票拉取会
超限（报错降级 AkShare），按日期全市场快照优先用于每日增量，可把调用压到
每天个位数。积分提升后再逐步扩大按股票场景。
"""
import logging
from datetime import date, timedelta

import pandas as pd

try:
    import tushare as ts
except ImportError:  # tushare 未安装：本层退化，采集器全部走 AkShare
    ts = None

from sqlalchemy import select

from app.models.tables import AppConfig

logger = logging.getLogger(__name__)

# app_config 键
_TOKEN_KEY = "tushare.token"

# 新浪指数代码 → Tushare ts_code（沪深300 / 中证500）
INDEX_TS_CODE = {"sh000300": "000300.SH", "sh000905": "000905.SH"}


def load_token(db) -> str:
    """从 app_config 表读取 Tushare token（空 / 未配置 → ""）

    对 session-like 对象做防御：execute 返回 None（测试桩）按未配置处理，
    rollback 缺失（测试桩）不补调，避免读配置异常炸到采集主流程。
    """
    if db is None:
        return ""
    try:
        result = db.execute(
            select(AppConfig).where(AppConfig.key == _TOKEN_KEY)
        )
        row = result.scalar_one_or_none() if result is not None else None
        return (row.value or "").strip() if row else ""
    except Exception:
        rollback = getattr(db, "rollback", None)
        if callable(rollback):
            rollback()
        logger.warning("读取 app_config.tushare.token 失败，按未配置处理", exc_info=True)
        return ""


def make_pro(db):
    """初始化 Tushare Pro 客户端；无 token / 依赖缺失 → None（采集器走 AkShare）"""
    token = load_token(db)
    if not token or ts is None:
        return None
    try:
        return ts.pro_api(token)
    except Exception:
        logger.warning("Tushare 初始化失败，降级 AkShare", exc_info=True)
        return None


def ts_code(code: str) -> str:
    """股票代码 → Tushare ts_code：600519→600519.SH / 000001→000001.SZ / 8|4|9→.BJ"""
    code = str(code)
    if code.startswith(("8", "4", "9")):
        return f"{code}.BJ"
    return f"{code}.SH" if code.startswith("6") else f"{code}.SZ"


def _ymd(d) -> str:
    """date / str → Tushare 需要的 YYYYMMDD"""
    if isinstance(d, date):
        return d.strftime("%Y%m%d")
    return str(d).replace("-", "")


def _iso(v) -> str:
    """Tushare YYYYMMDD（int/str）→ YYYY-MM-DD"""
    s = str(int(v))
    return f"{s[:4]}-{s[4:6]}-{s[6:8]}"


def _range(start_date, end_date, back_days: int = 365, forward_days: int = 365):
    """补全日期区间：调用方不传时默认 back 往前 / forward 往后"""
    today = date.today()
    if start_date is None:
        start_date = today - timedelta(days=back_days)
    if end_date is None:
        end_date = today + timedelta(days=forward_days)
    return start_date, end_date


def _to_date(v) -> date:
    """Tushare YYYYMMDD → date"""
    s = str(int(v))
    return date(int(s[:4]), int(s[4:6]), int(s[6:8]))


def _col_date(df: pd.DataFrame, col: str = "trade_date") -> pd.Series:
    """Tushare trade_date(int YYYYMMDD) → YYYY-MM-DD 字符串列（对齐东财格式）"""
    return df[col].map(_iso)


# ---------- 按股票：日行情（raw + 后复权，对齐东财中文列） ----------

def daily_pairs(pro, code: str, start_date, end_date):
    """不复权行情 + 复权因子 → (raw_df, hfq_df)，列为东财格式（中文）

    build_rows 期望：raw 含 日期/开盘/最高/最低/收盘/成交量/成交额；
    hfq 含 日期/收盘（后复权价 = 收盘 × 复权因子）。Tushare 的 adj_factor
    即后复权因子，直接相乘即可，无需二次拉取。
    """
    tc = ts_code(code)
    raw = pro.daily(ts_code=tc, start_date=_ymd(start_date), end_date=_ymd(end_date))
    if raw is None or raw.empty:
        return pd.DataFrame(), pd.DataFrame()
    fac = pro.adj_factor(ts_code=tc, start_date=_ymd(start_date), end_date=_ymd(end_date))
    fmap = dict(zip(fac["trade_date"], fac["adj_factor"])) if fac is not None and not fac.empty else {}

    raw = raw.copy()
    raw["日期"] = _col_date(raw)
    # vol 单位手（与东财一致）、amount 单位千元 → 元（与东财一致）
    raw = raw.rename(columns={
        "open": "开盘", "high": "最高", "low": "最低", "close": "收盘",
        "vol": "成交量", "amount": "成交额",
    })
    raw["成交量"] = raw["成交量"].astype(float)
    raw["成交额"] = raw["成交额"] * 1000

    raw["_factor"] = raw["trade_date"].map(fmap)
    hfq = raw[["日期", "收盘"]].copy()
    hfq["收盘"] = raw["收盘"] * raw["_factor"]
    return raw[["日期", "开盘", "最高", "最低", "收盘", "成交量", "成交额"]], hfq


# ---------- 按日期：全市场行情快照（每日增量主路径） ----------

def daily_snapshot(pro, trade_date, codes=None):
    """当日全市场行情 + 复权因子 → 与 build_rows 相同的入库行列表

    单次 pro.daily(trade_date=...) + pro.adj_factor(trade_date=...) 覆盖全市场，
    每日增量仅 2 次调用。codes 非空时仅保留这些股票（股票池优先，降请求体量）。
    """
    td = _ymd(trade_date)
    daily = pro.daily(trade_date=td)
    fac = pro.adj_factor(trade_date=td)
    if daily is None or daily.empty:
        return []
    # 注意：复权因子按 ts_code 键控——同一交易日不同股票因子不同，
    # 按 trade_date 键会被后一只覆盖（全市场快照每只都拿到错误因子）。
    fmap = dict(zip(fac["ts_code"], fac["adj_factor"])) if fac is not None and not fac.empty else {}
    keep = set(codes) if codes else None
    rows = []
    for _, r in daily.iterrows():
        code = r["ts_code"].split(".")[0]
        if keep is not None and code not in keep:
            continue
        close = float(r["close"])
        factor = fmap.get(r["ts_code"])
        rows.append({
            "code": code,
            "trade_date": _to_date(r["trade_date"]),
            "open": float(r["open"]), "high": float(r["high"]),
            "low": float(r["low"]), "close": close,
            "volume": float(r["vol"]),
            "amount": float(r["amount"]) * 1000 if pd.notna(r["amount"]) else None,
            # Tushare adj_factor 为绝对复权因子（相对上市首日），与东财 hfq 口径一致
            "adj_factor": round(float(factor), 4) if factor else None,
        })
    return rows


# ---------- 按股票：日度估值 ----------

def daily_basic_rows(pro, code: str, start_date, end_date) -> list[dict]:
    """pro.daily_basic 按股票区间 → to_rows 同形状入库行

    单位换算：Tushare total_mv/circ_mv 单位万元 → ×10000 转元（对齐东财口径）。
    pe_ttm / pe / pb 缺失（停牌/亏损）原样 None，判断留给因子侧。
    """
    tc = ts_code(code)
    df = pro.daily_basic(ts_code=tc, start_date=_ymd(start_date), end_date=_ymd(end_date))
    if df is None or df.empty:
        return []
    rows = []
    for _, r in df.iterrows():
        def _num(v):
            return float(v) if v is not None and not (isinstance(v, float) and pd.isna(v)) else None
        rows.append({
            "code": code,
            "trade_date": _to_date(r["trade_date"]),
            "close": _num(r.get("close")),
            "total_mv": _num(r.get("total_mv")) * 10000 if _num(r.get("total_mv")) is not None else None,
            "float_mv": _num(r.get("circ_mv")) * 10000 if _num(r.get("circ_mv")) is not None else None,
            "pe_ttm": _num(r.get("pe_ttm")),
            "pe_static": _num(r.get("pe")),
            "pb": _num(r.get("pb")),
        })
    return rows


# ---------- 按日期：全市场估值快照（每日增量主路径） ----------

def daily_basic_snapshot(pro, trade_date, codes=None) -> list[dict]:
    """当日全市场估值快照 → 入库行列表（单次调用；codes 过滤股票池优先）"""
    df = pro.daily_basic(trade_date=_ymd(trade_date))
    if df is None or df.empty:
        return []
    keep = set(codes) if codes else None
    rows = []
    for _, r in df.iterrows():
        code = r["ts_code"].split(".")[0]
        if keep is not None and code not in keep:
            continue

        def _num(v):
            return float(v) if v is not None and not (isinstance(v, float) and pd.isna(v)) else None

        mv = _num(r.get("total_mv"))
        cmv = _num(r.get("circ_mv"))
        rows.append({
            "code": code,
            "trade_date": _to_date(r["trade_date"]),
            "close": _num(r.get("close")),
            "total_mv": mv * 10000 if mv is not None else None,
            "float_mv": cmv * 10000 if cmv is not None else None,
            "pe_ttm": _num(r.get("pe_ttm")),
            "pe_static": _num(r.get("pe")),
            "pb": _num(r.get("pb")),
        })
    return rows


# ---------- 指数 ----------

def index_daily_rows(pro, symbol: str, start_date=None, end_date=None) -> list[dict]:
    """新浪指数码 sh000300 → pro.index_daily → build_rows(index) 同形状行"""
    start_date, end_date = _range(start_date, end_date, back_days=365, forward_days=0)
    tc = INDEX_TS_CODE.get(symbol, symbol.replace("sh", "") + ".SH")
    df = pro.index_daily(ts_code=tc, start_date=_ymd(start_date), end_date=_ymd(end_date))
    if df is None or df.empty:
        return []
    rows = []
    for _, r in df.iterrows():
        rows.append({
            "code": symbol,
            "trade_date": _to_date(r["trade_date"]),
            "open": float(r["open"]), "high": float(r["high"]),
            "low": float(r["low"]), "close": float(r["close"]),
            "volume": int(r["vol"]) if pd.notna(r.get("vol")) else None,
            "amount": float(r["amount"]) * 1000 if pd.notna(r.get("amount")) else None,
            "adj_factor": None,
        })
    return rows


# ---------- 交易日历 ----------

def trade_cal_rows(pro, start_date=None, end_date=None) -> list[dict]:
    """交易日历（is_open=1 才入库，对齐现有「只有交易日」语义）"""
    start_date, end_date = _range(start_date, end_date, back_days=730, forward_days=365)
    df = pro.trade_cal(exchange="SSE", start_date=_ymd(start_date),
                       end_date=_ymd(end_date), is_open="1")
    if df is None or df.empty:
        return []
    return [{
        "cal_date": _to_date(r["cal_date"]),
        "is_open": True,
        "exchange": "SSE",
    } for _, r in df.iterrows()]


# ---------- 股票列表 ----------

def stock_basic_rows(pro) -> list[dict]:
    """pro.stock_basic 全市场列表（list_date 有则带回，无则由 AkShare 交易所接口补全）"""
    fields = "ts_code,symbol,name,list_date"
    df = pro.stock_basic(list_status="L", fields=fields)
    if df is None or df.empty:
        return []
    from app.collectors.stock import infer_market
    rows = []
    for _, r in df.iterrows():
        code = str(r["symbol"]).zfill(6)
        name = str(r["name"]).replace(" ", "").replace("　", "")
        row = {
            "code": code,
            "name": name,
            "market": infer_market(code),
            "status": "D" if "退" in name else "L",
        }
        ld = r.get("list_date")
        if pd.notna(ld) and ld:
            row["list_date"] = str(ld)[:10].replace("/", "-")
        rows.append(row)
    return rows


# ---------- 财务指标 ----------

def fina_indicator_rows(pro, code: str, period: str) -> list[dict]:
    """pro.fina_indicator 单报告期 → 入库行（industry 不在本接口，回填仍走 AkShare）

    period 形如 '20260331'。字段映射：roe/profit_yoy/or_yoy/
    grossprofit_margin/debt_to_assets → 现有 financial_indicator 列。
    """
    df = pro.fina_indicator(ts_code=ts_code(code), period=period)
    if df is None or df.empty:
        return []

    def _num(v):
        return float(v) if v is not None and not (isinstance(v, float) and pd.isna(v)) else None

    rows = []
    for _, r in df.iterrows():
        ann = r.get("ann_date")
        row = {
            "code": code,
            "report_date": date.fromisoformat(_iso(period)),
            "roe": _num(r.get("roe")),
            "profit_growth": _num(r.get("profit_yoy")),
            "revenue_growth": _num(r.get("or_yoy")),
            "gross_margin": _num(r.get("grossprofit_margin")),
            "debt_ratio": _num(r.get("debt_to_assets")),
        }
        if pd.notna(ann) and ann:
            row["announce_date"] = date.fromisoformat(_iso(ann))
        rows.append(row)
    return rows
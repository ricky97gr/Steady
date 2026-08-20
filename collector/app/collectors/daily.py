"""日行情采集器：不复权 + 后复权两次拉取，计算复权因子，质量校验后入库

数据源：优先东财（stock_zh_a_hist），连接失败自动降级新浪（stock_zh_a_daily）。
新浪成交量单位为股，统一转手（/100）。
"""
import logging
from concurrent.futures import ThreadPoolExecutor
from datetime import date

import akshare as ak
import pandas as pd
import requests

from app.collectors.base import BaseCollector, to_ak_date
from app.cleaners import clean_daily_rows
from app.config import REQUEST_TIMEOUT
from app.db import upsert
from app.models.tables import DailyPrice

logger = logging.getLogger(__name__)

# AkShare 列名 → 内部字段
COLUMN_MAP = {
    "日期": "trade_date",
    "开盘": "open",
    "最高": "high",
    "最低": "low",
    "收盘": "close",
    "成交量": "volume",
    "成交额": "amount",
}


def sina_symbol(code: str) -> str:
    """股票代码 → 新浪带市场前缀格式（sh600519 / sz000001 / bj830001）"""
    prefix = "sh" if code.startswith("6") else "bj" if code.startswith(("8", "4", "9")) else "sz"
    return prefix + code


def normalize_sina(df: pd.DataFrame) -> pd.DataFrame:
    """新浪列名 → 东财列名；成交量 股 → 手"""
    df = df.rename(columns={
        "date": "日期", "open": "开盘", "high": "最高",
        "low": "最低", "close": "收盘", "volume": "成交量", "amount": "成交额",
    })
    if "成交量" in df.columns:
        df["成交量"] = (df["成交量"] / 100).round(0)
    return df


def _with_timeout(fn, *args, timeout=None, **kwargs):
    """在线程内执行 AkShare 请求并施加超时。

    AkShare 底层 requests 未设置 timeout，对端半开连接时会永久挂起
    （曾因此卡死整个同步）。此包装器兜底：超时抛 TimeoutError，
    由调用方按"降级/重试"处理，而不是无限等待。
    """
    if timeout is None:
        timeout = REQUEST_TIMEOUT
    with ThreadPoolExecutor(max_workers=1) as ex:
        try:
            return ex.submit(fn, *args, **kwargs).result(timeout=timeout)
        except TimeoutError:
            raise TimeoutError(f"请求超时（>{timeout}s）")


def fetch_pair(code: str, start: str, end: str) -> tuple[pd.DataFrame, pd.DataFrame]:
    """拉取不复权 + 后复权两套行情，返回 (raw, hfq)，列为东财格式

    东财失败/超时 → 降级新浪；新浪同样为空/失败 → 抛异常触发 base.run() 重试，
    避免"返回空"被静默记为成功造成行情缺口无人知晓。
    """
    try:
        raw = _with_timeout(
            ak.stock_zh_a_hist, symbol=code, period="daily",
            start_date=start, end_date=end, adjust="",
        )
        hfq = _with_timeout(
            ak.stock_zh_a_hist, symbol=code, period="daily",
            start_date=start, end_date=end, adjust="hfq",
        )
        return raw, hfq
    except Exception as e:
        reason = "超时" if isinstance(e, TimeoutError) else str(e)
        logger.warning("%s 东财接口失败(%s)，降级新浪源", code, reason)
        raw = _with_timeout(
            ak.stock_zh_a_daily, symbol=sina_symbol(code),
            start_date=start, end_date=end, adjust="",
        )
        hfq = _with_timeout(
            ak.stock_zh_a_daily, symbol=sina_symbol(code),
            start_date=start, end_date=end, adjust="hfq",
        )
        raw, hfq = normalize_sina(raw), normalize_sina(hfq)
        if raw is None or raw.empty:
            # 双源都拿不到数据：判定失败，交给上层重试，不静默记 0 条成功
            raise RuntimeError(
                f"{code} 东财与新浪均未返回数据（东财{reason}），判定失败触发重试")
    return raw, hfq


def build_rows(code: str, raw: pd.DataFrame, hfq: pd.DataFrame) -> list[dict]:
    """不复权 + 后复权合并 → 入库行（含 adj_factor 与 prev_close）"""
    if raw.empty:
        return []
    raw = raw.rename(columns=COLUMN_MAP)
    hfq_close = hfq.set_index("日期")["收盘"] if not hfq.empty else pd.Series(dtype=float)
    rows = []
    prev_close = None
    for _, r in raw.iterrows():
        raw_close = float(r["close"])
        hfq_c = hfq_close.get(r["trade_date"])
        rows.append(
            {
                "code": code,
                "trade_date": date.fromisoformat(str(r["trade_date"])),
                "open": float(r["open"]),
                "high": float(r["high"]),
                "low": float(r["low"]),
                "close": raw_close,
                "volume": int(r["volume"]) if pd.notna(r["volume"]) else None,
                "amount": float(r["amount"]) if pd.notna(r["amount"]) else None,
                # 转 Python float：numpy 类型无法被 psycopg2 直接绑定
                "adj_factor": (
                    round(float(hfq_c) / raw_close, 4)
                    if hfq_c and raw_close > 0 else None
                ),
                "prev_close": prev_close,
            }
        )
        prev_close = raw_close
    return rows


class DailyCollector(BaseCollector):
    """按股票代码拉取日K行情，经质量校验后入库"""

    def fetch(self, code: str, start_date, end_date, *args, **kwargs) -> list[dict]:
        start = to_ak_date(start_date)
        end = to_ak_date(end_date)
        # 后复权用于计算复权因子（因子计算用前复权价，此处只需因子比例）
        raw, hfq = fetch_pair(code, start, end)
        rows = build_rows(code, raw, hfq)
        logger.info("%s 拉取 %s 条日行情", code, len(rows))
        return rows

    def save(self, data):
        code = data[0]["code"] if data else "-"
        clean = clean_daily_rows(code, data)
        # prev_close 仅供涨跌幅校验使用，入库前过滤掉
        table_cols = set(DailyPrice.__table__.columns.keys())
        clean = [{k: r[k] for k in table_cols if k in r} for r in clean]
        upsert(
            self.db,
            DailyPrice,
            clean,
            conflict_cols=["code", "trade_date"],
            update_cols=[
                "open", "high", "low", "close", "volume", "amount", "adj_factor",
            ],
        )
        logger.info("%s 入库 %s 条（丢弃 %s 条）", code, len(clean),
                    len(data) - len(clean))
        return True

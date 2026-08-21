"""指数行情采集器：沪深300 / 中证500 日行情，作策略收益基准对比

数据源：新浪 stock_zh_index_daily（已验证可用，返回完整历史）。
存储：stock_basic 中 market='INDEX' 的伪股票 + daily_price 通用表。
"""
import logging
from datetime import date

import akshare as ak
import pandas as pd

from app.collectors.base import BaseCollector
from app.db import upsert
from app.models.tables import DailyPrice, StockBasic
from app.sources import tushare

logger = logging.getLogger(__name__)

# 指数代码（新浪格式）→ 名称
INDEX_NAMES = {"sh000300": "沪深300", "sh000905": "中证500"}


def build_rows(symbol: str, df: pd.DataFrame,
               start_date: date | None = None,
               end_date: date | None = None) -> list[dict]:
    rows = []
    for _, r in df.iterrows():
        d = date.fromisoformat(str(r["date"]))
        if start_date and d < start_date:
            continue
        if end_date and d > end_date:
            continue
        rows.append({
            "code": symbol,
            "trade_date": d,
            "open": float(r["open"]),
            "high": float(r["high"]),
            "low": float(r["low"]),
            "close": float(r["close"]),
            "volume": int(r["volume"]) if pd.notna(r["volume"]) else None,
            "amount": None,
            "adj_factor": None,
        })
    return rows


class IndexCollector(BaseCollector):
    """拉取指数日行情，供收益曲线与沪深300 基准对比"""

    def fetch(self, symbol: str = "sh000300", start_date=None, end_date=None,
              *args, **kwargs) -> list[dict]:
        # Tushare 主源：index_daily（指数无缺口，1 次拉全量区间）
        pro = tushare.make_pro(self.db)
        if pro is not None:
            try:
                rows = tushare.index_daily_rows(pro, symbol, start_date, end_date)
                if not rows:
                    raise RuntimeError(f"{symbol} Tushare 指数未返回数据")
                logger.info("%s Tushare 拉取指数行情 %s 条", symbol, len(rows))
                return rows
            except Exception as e:
                logger.warning("%s Tushare 指数失败(%s)，降级 AkShare", symbol, e)
        df = ak.stock_zh_index_daily(symbol=symbol)
        rows = build_rows(symbol, df, start_date, end_date)
        logger.info("%s AkShare 拉取指数行情 %s 条", symbol, len(rows))
        return rows

    def save(self, data):
        if not data:
            return True
        symbol = data[0]["code"]
        # 1. 确保指数在 stock_basic 中（market='INDEX'，满足 daily_price 外键）
        upsert(
            self.db,
            StockBasic,
            [{
                "code": symbol,
                "name": INDEX_NAMES.get(symbol, symbol),
                "market": "INDEX",
                "status": "L",
            }],
            conflict_cols=["code"],
            update_cols=["name", "market", "status"],
        )
        # 2. 指数日行情入库（与股票共用 daily_price 表）
        upsert(
            self.db,
            DailyPrice,
            data,
            conflict_cols=["code", "trade_date"],
            update_cols=["open", "high", "low", "close", "volume", "amount",
                         "adj_factor"],
        )
        logger.info("%s 指数行情入库 %s 条", symbol, len(data))
        return True


def sync_index() -> bool:
    """手动触发入口：同步全部配置指数"""
    from app.config import index_code_list
    from app.db import get_session

    ok = True
    for symbol in index_code_list():
        ok = IndexCollector(get_session()).run(symbol=symbol) and ok
    return ok
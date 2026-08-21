"""日度估值采集器：东财 stock_value_em（PE(TTM)/PE(静)/PB/市值）

接口单次返回全历史（约 2018 年至今，逐日一行），无日期参数，
故全量拉取 + upsert 幂等；PE/PB 缺失（停牌/亏损无值）原样存 NULL，
正负值判断留给因子计算侧（pe<=0 不参与价值横截面）。
"""
import logging
from datetime import date, timedelta

import akshare as ak
import pandas as pd

from app.collectors.base import BaseCollector
from app.db import upsert
from app.models.tables import DailyValuation
from app.sources import tushare

logger = logging.getLogger(__name__)


def _num(v) -> float | None:
    """NaN/空 → None，其余转 float（与 finance.py 一致）"""
    if v is None or (isinstance(v, float) and pd.isna(v)) or v == "":
        return None
    return float(v)


def to_rows(code: str, df: pd.DataFrame) -> list[dict]:
    """估值表 → 入库行（列名与 DailyValuation 对齐）"""
    rows = []
    for _, r in df.iterrows():
        if pd.isna(r.get("数据日期")):
            continue
        rows.append(
            {
                "code": code,
                "trade_date": pd.Timestamp(r["数据日期"]).date(),
                "close": _num(r.get("当日收盘价")),
                "total_mv": _num(r.get("总市值")),
                "float_mv": _num(r.get("流通市值")),
                "pe_ttm": _num(r.get("PE(TTM)")),
                "pe_static": _num(r.get("PE(静)")),
                "pb": _num(r.get("市净率")),
            }
        )
    return rows


class ValuationCollector(BaseCollector):
    """日度估值采集（东财，按股票全量拉取）"""

    def fetch(self, code: str, *args, **kwargs) -> list[dict]:
        # Tushare 主源：daily_basic 按股票近一年（主增量走 tasks 的按日全市场快照）
        pro = tushare.make_pro(self.db)
        if pro is not None:
            try:
                today = date.today()
                rows = tushare.daily_basic_rows(
                    pro, code, today - timedelta(days=365), today)
                if not rows:
                    raise RuntimeError(f"{code} Tushare 估值未返回数据")
                logger.info("%s Tushare 拉取 %s 条估值", code, len(rows))
                return rows
            except Exception as e:
                logger.warning("%s Tushare 估值失败(%s)，降级 AkShare", code, e)
        df = ak.stock_value_em(symbol=code)
        logger.info("AkShare 返回 %s 估值 %s 条", code, len(df))
        return to_rows(code, df)

    def save(self, data):
        upsert(
            self.db,
            DailyValuation,
            data,
            conflict_cols=["code", "trade_date"],
            update_cols=["close", "total_mv", "float_mv",
                         "pe_ttm", "pe_static", "pb"],
        )
        return True


def sync_valuation(codes: list[str] | None = None, rate_limit: float = 1.0) -> bool:
    """手动触发入口：同步日度估值（跳过当日已最新的股票）"""
    import time

    from datetime import date

    from sqlalchemy import func, select

    from app.db import get_session
    from app.models.tables import StockBasic

    db = get_session()
    if codes is None:
        codes = sorted(
            db.execute(
                select(StockBasic.code).where(
                    StockBasic.universe.in_(("hs300", "zz500"))
                )
            ).scalars().all()
        )
    latest = {
        code: max_d
        for code, max_d in db.execute(
            select(DailyValuation.code, func.max(DailyValuation.trade_date))
            .group_by(DailyValuation.code)
        ).all()
    }
    todo = [c for c in codes if latest.get(c) is None or latest[c] < date.today()]
    logger.info("估值同步：%s 只中 %s 只需更新", len(codes), len(todo))
    ok_count = fail_count = 0
    for code in todo:
        ok = ValuationCollector(db).run(code)
        ok_count += ok
        fail_count += not ok
        time.sleep(rate_limit)
    logger.info("估值同步完成：成功 %s，失败 %s", ok_count, fail_count)
    return fail_count == 0

"""股票列表采集器：全市场列表 + 股票池（universe）标记 + 上市日期/行业补全"""
import logging

import akshare as ak
import pandas as pd

from app.collectors.base import BaseCollector
from app.db import upsert
from app.models.tables import StockBasic
from app.sources import tushare

logger = logging.getLogger(__name__)

# 股票池成分来源：沪深300 / 中证500（官方中证指数公司）
UNIVERSE_INDEX = {"hs300": "000300", "zz500": "000905"}


def infer_market(code: str) -> str:
    """按代码段推断市场：6xx→沪，0/3xx→深，8/4/9xx→北交所"""
    if code.startswith("6"):
        return "SH"
    if code.startswith(("0", "3")):
        return "SZ"
    return "BJ"


def normalize_stock_rows(df) -> list[dict]:
    """股票列表 → 入库行（market 推断、退市标记）"""
    rows = []
    for _, r in df.iterrows():
        code = str(r["code"]).zfill(6)
        # 交易所原始名称带空格（如"南 京 港"），去掉以支持前端模糊搜索
        name = str(r["name"]).replace(" ", "").replace("　", "")
        rows.append(
            {
                "code": code,
                "name": name,
                "market": infer_market(code),
                "status": "D" if "退" in name else "L",
                # industry 由财务采集器顺带回填；list_date AkShare 免费接口不提供
            }
        )
    return rows


def fetch_list_dates() -> dict[str, str]:
    """交易所官方列表接口补全上市日期（沪主板/科创板/深A/北交所，各 1 次请求，免费）"""
    result: dict[str, str] = {}
    for symbol in ("主板A股", "科创板"):
        df = ak.stock_info_sh_name_code(symbol=symbol)
        for _, r in df.iterrows():
            code = str(r["证券代码"]).zfill(6)
            if pd.notna(r.get("上市日期")):
                result[code] = str(r["上市日期"])[:10]
    df = ak.stock_info_sz_name_code(symbol="A股列表")
    for _, r in df.iterrows():
        code = str(r["A股代码"]).zfill(6)
        if pd.notna(r.get("A股上市日期")):
            result[code] = str(r["A股上市日期"])[:10]
    df = ak.stock_info_bj_name_code()
    for _, r in df.iterrows():
        code = str(r["证券代码"]).zfill(6)
        if pd.notna(r.get("上市日期")):
            result[code] = str(r["上市日期"])[:10]
    logger.info("上市日期补全拉取：%s 只", len(result))
    return result


def fetch_industries() -> dict[str, str]:
    """深市/北交所官方列表自带行业（证监会分类）→ {code: 行业}。

    沪市主板/科创板官方列表无行业字段，由财务采集器（业绩报表，东财分类）
    在财报披露季顺带回填，故此处不覆盖沪市。
    """
    result: dict[str, str] = {}
    df = ak.stock_info_sz_name_code(symbol="A股列表")
    for _, r in df.iterrows():
        code = str(r["A股代码"]).zfill(6)
        if pd.notna(r.get("所属行业")) and str(r["所属行业"]).strip():
            result[code] = str(r["所属行业"]).strip()
    df = ak.stock_info_bj_name_code()
    for _, r in df.iterrows():
        code = str(r["证券代码"]).zfill(6)
        if pd.notna(r.get("所属行业")) and str(r["所属行业"]).strip():
            result[code] = str(r["所属行业"]).strip()
    logger.info("行业补全拉取：%s 只", len(result))
    return result


def fetch_universe() -> dict[str, set[str]]:
    """拉取沪深300/中证500 成分股代码集合"""
    result = {}
    for name, index_code in UNIVERSE_INDEX.items():
        df = ak.index_stock_cons_csindex(symbol=index_code)
        result[name] = {str(c).zfill(6) for c in df["成分券代码"]}
        logger.info("股票池 %s 成分股 %s 只", name, len(result[name]))
    return result


class StockCollector(BaseCollector):
    """从 AkShare 拉取全市场股票列表并入库（含 universe 标记）"""

    def fetch(self, *args, **kwargs) -> list[dict]:
        # Tushare 主源：stock_basic（自带 list_date，省 3 个交易所请求；无则回退）
        pro = tushare.make_pro(self.db)
        if pro is not None:
            try:
                rows = tushare.stock_basic_rows(pro)
                if not rows:
                    raise RuntimeError("Tushare 股票列表未返回数据")
                logger.info("Tushare 返回股票 %s 只", len(rows))
                return rows
            except Exception as e:
                logger.warning("Tushare 股票列表失败(%s)，降级 AkShare", e)
        df = ak.stock_info_a_code_name()
        logger.info("AkShare 返回股票 %s 只", len(df))
        return normalize_stock_rows(df)

    def save(self, data):
        # 1. UPSERT 全市场股票列表
        upsert(
            self.db,
            StockBasic,
            data,
            conflict_cols=["code"],
            update_cols=["name", "market", "status"],
        )
        # 2. 标记股票池（成分股 → universe；非成分股 → 置空）
        universe = fetch_universe()
        for row in data:
            pool = next(
                (name for name, codes in universe.items() if row["code"] in codes),
                None,
            )
            row["universe"] = pool
        upsert(
            self.db,
            StockBasic,
            data,
            conflict_cols=["code"],
            update_cols=["universe"],
        )
        updated = sum(1 for r in data if r["universe"])
        logger.info("入库 %s 只，其中股票池 %s 只", len(data), updated)

        # 3. 补全上市日期与行业（纯 UPDATE：UPSERT 的 ON CONFLICT 在冲突探测前
        #    先强制 NOT NULL 检查，缺 name 的行即使命中冲突也会报错，
        #    与 finance.py 行业回填同理）
        from sqlalchemy import text

        # Tushare 列表自带 list_date 时直接回填；否则走交易所接口补全
        ts_dates = {r["code"]: r["list_date"] for r in data if r.get("list_date")}
        dates = ts_dates or fetch_list_dates()
        if dates:
            self.db.execute(
                text("UPDATE stock_basic SET list_date = :d WHERE code = :code"),
                [{"code": c, "d": d} for c, d in dates.items()],
            )
            self.db.commit()
        industries = fetch_industries()
        if industries:
            self.db.execute(
                text("UPDATE stock_basic SET industry = :industry WHERE code = :code"),
                [{"code": c, "industry": i} for c, i in industries.items()],
            )
            self.db.commit()
        return True


def sync_stock_list() -> bool:
    """手动触发入口：同步股票列表与股票池标记"""
    from app.db import get_session

    return StockCollector(get_session()).run()

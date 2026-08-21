"""财务数据采集器：业绩报表 + 资产负债表（均含公告日，防止未来函数）

AkShare 免费接口（已验证）：
- stock_yjbb_em(date)：业绩报表，含 净资产收益率/净利润同比增长/营收同比增长/毛利率/所处行业/最新公告日期
- stock_zcfz_em(date)：资产负债表，含 资产负债率/公告日期

PE/PB 为日度估值数据，V1 暂不填入本表（Sprint 4 因子计算时用日度估值接口补充）。
"""
import logging
from datetime import date

import akshare as ak
import pandas as pd
from sqlalchemy import and_

from app.collectors.base import BaseCollector
from app.db import upsert
from app.models.tables import FinancialIndicator, StockBasic
from app.sources import tushare

logger = logging.getLogger(__name__)

# 报告期 = 季度末：0331 / 0630 / 0930 / 1231
QUARTER_MONTH_DAY = ((3, 31), (6, 30), (9, 30), (12, 31))


def quarter_ends(n: int) -> list[str]:
    """最近 n 个已结束的报告期（YYYYMMDD），从最新往前"""
    today = date.today()
    y, q = today.year, (today.month - 1) // 3  # 当前季度序号 0..3
    end_month = QUARTER_MONTH_DAY[q][0]
    if today.month <= end_month:  # 当前季度尚未结束，从上一季度起算
        q -= 1
        if q < 0:
            q = 3
            y -= 1
    periods = []
    for _ in range(n):
        m, d = QUARTER_MONTH_DAY[q]
        periods.append(f"{y}{m:02d}{d}")
        q -= 1
        if q < 0:
            q = 3
            y -= 1
    return periods


def _num(v) -> float | None:
    """数值字段：NaN / 空 → None"""
    if v is None or pd.isna(v):
        return None
    return float(v)


def _dt(v) -> date | None:
    """日期字段：datetime/date/str → date，空 → None

    注意 pd.NaT 也是 Timestamp 实例，必须先判 isna 再取 year/month。
    """
    if v is None or pd.isna(v):
        return None
    if isinstance(v, (date, pd.Timestamp)):
        return date(v.year, v.month, v.day)
    return date.fromisoformat(str(v)[:10])


def build_rows(yjbb: pd.DataFrame, zcfz: pd.DataFrame,
               report_date: str) -> list[dict]:
    """业绩报表 + 资产负债表 → 入库行（公告日取两者较晚者，更保守）"""
    z = zcfz.set_index("股票代码") if not zcfz.empty else pd.DataFrame()
    rows = []
    for _, r in yjbb.iterrows():
        code = str(r["股票代码"]).zfill(6)
        announce = _dt(r.get("最新公告日期"))
        if code in z.index:
            z_announce = _dt(z.loc[code].get("公告日期"))
            if z_announce and (announce is None or z_announce > announce):
                announce = z_announce
        row = {
            "code": code,
            "report_date": date.fromisoformat(report_date),
            "roe": _num(r.get("净资产收益率")),
            "profit_growth": _num(r.get("净利润-同比增长")),
            "revenue_growth": _num(r.get("营业总收入-同比增长")),
            "gross_margin": _num(r.get("销售毛利率")),
            "industry": str(r.get("所处行业")) if pd.notna(r.get("所处行业")) else None,
            # PE/PB 留空（Sprint 4 日度估值补充）
            "announce_date": announce,
            # 键必须总是存在：列带 Python default，multi-row INSERT 里
            # 缺键的行会渲染 DEFAULT，与绑定参数混用会被 SQLAlchemy 拒绝
            "debt_ratio": (
                _num(z.loc[code].get("资产负债率")) if code in z.index else None
            ),
        }
        rows.append(row)
    return rows


class FinanceCollector(BaseCollector):
    """拉取财务指标，必须带公告日（announce_date）"""

    def fetch(self, report_periods: list[str] | None = None,
              code: str | None = None, *args, **kwargs) -> list[dict]:
        periods = report_periods or []
        # Tushare 主源：fina_indicator 按股票逐期（需 2000+ 积分；首请求失败快速降级）
        pro = tushare.make_pro(self.db)
        if pro is not None:
            try:
                rows = self._fetch_tushare(pro, periods, code)
                logger.info("Tushare 财务拉取 %s 条", len(rows))
                return rows
            except Exception as e:
                logger.warning("Tushare 财务失败(%s)，降级 AkShare", e)
        all_rows = []
        for p in periods:
            yjbb = ak.stock_yjbb_em(date=p)
            zcfz = ak.stock_zcfz_em(date=p)
            rows = build_rows(yjbb, zcfz, p)
            logger.info("报告期 %s：%s 只股票", p, len(rows))
            all_rows.extend(rows)
        if code:
            all_rows = [r for r in all_rows if r["code"] == code]
        return all_rows

    def _fetch_tushare(self, pro, periods: list[str], code: str | None) -> list[dict]:
        """Tushare 财务：按股票 × 报告期逐查；首请求失败抛异常 → 触发降级

        积分不足时 fina_indicator 首个请求即报错，避免对全市场空转。
        """
        from sqlalchemy import select

        if code:
            codes = [code]
        else:
            codes = sorted(
                self.db.execute(select(StockBasic.code)).scalars().all())
        all_rows: list[dict] = []
        first = True
        for c in codes:
            for p in periods:
                try:
                    all_rows.extend(tushare.fina_indicator_rows(pro, c, p))
                except Exception as e:
                    if first:
                        raise  # 首个请求失败：积分/接口不可用，直接降级 AkShare
                    logger.warning("%s 报告期 %s Tushare 财务失败: %s", c, p, e)
                first = False
        return all_rows

    def save(self, data):
        if not data:
            return True
        # 财务接口（业绩报表/资产负债表）覆盖全市场，含 B 股、北交所新股等
        # stock_basic 没有的行：financial_indicator 有外键约束且行业回填
        # 需要 name/market，因此只保留库中已有股票的记录。
        from sqlalchemy import select
        known = set(self.db.execute(select(StockBasic.code)).scalars().all())
        data = [r for r in data if r["code"] in known]

        # 1. 顺带回填行业（业绩报表自带所处行业）
        #    注意不能用 UPSERT：PG 的 ON CONFLICT 在唯一冲突探测**之前**
        #    先强制 NOT NULL 检查，缺 name 的行即使能命中冲突也会报错，
        #    因此行业回填用纯 UPDATE（语义上也是"更新已有股票"）。
        industries = {r["code"]: r["industry"] for r in data if r.get("industry")}
        if industries:
            from sqlalchemy import text
            # ORM update() 的 executemany 要求参数含主键，且 SQLAlchemy 1.4+
            # 对 per-row bulk update 有限制，直接用原生 SQL 批量更新
            self.db.execute(
                text(
                    "UPDATE stock_basic SET industry = :industry "
                    "WHERE code = :code"
                ),
                [{"code": c, "industry": i} for c, i in industries.items()],
            )
            self.db.commit()
        # 2. 财务指标 UPSERT：同一 (code, report_date) 冲突时，
        #    仅当新行公告日不早于库中行才覆盖（保证取最新披露）。
        #    build_rows 产出的行含 industry（供 stock_basic 用），此处过滤掉。
        table_cols = set(FinancialIndicator.__table__.columns.keys())
        fin_rows = [{k: r[k] for k in table_cols if k in r} for r in data]

        def _where(excluded):
            return and_(
                FinancialIndicator.announce_date.is_(None),
                excluded.announce_date.isnot(None),
            ) | (FinancialIndicator.announce_date <= excluded.announce_date)

        upsert(
            self.db,
            FinancialIndicator,
            fin_rows,
            conflict_cols=["code", "report_date"],
            update_cols=[
                "roe", "profit_growth", "revenue_growth", "debt_ratio",
                "gross_margin", "announce_date",
            ],
            where=_where,
        )
        logger.info("财务数据入库 %s 条", len(data))
        return True


def sync_finance(quarters: int = 4) -> bool:
    """手动触发入口：同步最近 N 个报告期"""
    from app.db import get_session

    return FinanceCollector(get_session()).run(report_periods=quarter_ends(quarters))
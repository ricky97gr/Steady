"""市场热点快照采集器（Issue #4 早盘简报数据源）：每日早晨采集一次

数据源（AkShare，均已验证存在）：
- 隔夜外盘：新浪 美股指数 index_us_stock_sina（.DJI/.IXIC/.INX，取末行）
- A股指数：东财 沪深京指数 spot stock_zh_index_spot_em（上证/深成/创业板）
- 板块涨幅 + 资金流：同花顺 行业板块概览 stock_board_industry_summary_ths（主源），
  东财 行业板块 stock_board_industry_name_em / 同花顺行业资金流 兜底
- 活跃个股：东财 人气榜 stock_hot_rank_em（主源），涨停池 stock_zt_pool_em 兜底
  （早盘时段返回最近交易日涨停个股，语义为「昨日涨停·今日关注」）

列名防御：AkShare 版本演进列名可能变化，_pick 按候选名做「精确 → 子串」匹配，
缺列/异常时该项返回空列表，绝不抛异常中断整批（热点是增强数据，失败可降级）。
"""
import logging
from datetime import date

import akshare as ak
import pandas as pd

from app.collectors.base import BaseCollector
from app.config import HOTSPOT_TOP_N, hotspot_index_list
from app.db import upsert
from app.models.tables import MarketHotspot

logger = logging.getLogger(__name__)

# 美股指数代码 → 中文名（新浪格式 .DJI/.IXIC/.INX/.NDX）
US_INDEX_NAMES = {".DJI": "道琼斯", ".IXIC": "纳斯达克", ".INX": "标普500", ".NDX": "纳指100"}
# A股重要指数（东财 spot 名称）
CN_INDEX_NAMES = ["上证指数", "深证成指", "创业板指"]


def _pick(df: pd.DataFrame, *candidates: str):
    """按候选名取列（忽略大小写）：先精确匹配，再子串双向包含。找不到返回 None。"""
    lower = {str(c).lower(): c for c in df.columns}
    for cand in candidates:
        if cand.lower() in lower:
            return lower[cand.lower()]
    for cand in candidates:
        low = cand.lower()
        for k, orig in lower.items():
            if low in k or k in low:
                return orig
    return None


def _f(x) -> float | None:
    """安全转 float；NaN/空返回 None"""
    try:
        v = float(x)
        return v if pd.notna(v) else None
    except (TypeError, ValueError):
        return None


def _fmt_flow(v) -> str | None:
    """资金净流入格式化：元 → 'X.XX亿'；空返回 None"""
    v = _f(v)
    if v is None:
        return None
    return f"{v / 1e8:.2f}亿"


def _fmt_yi(v) -> str | None:
    """格式化「已是亿元」的值 → 'X.XX亿'（同花顺板块概览净流入单位即亿）"""
    v = _f(v)
    if v is None:
        return None
    return f"{v:.2f}亿"


# ---------- 分项抓取（各自 try/except，失败返回 [] 不中断整批） ----------

def _fetch_indices() -> list[dict]:
    rows = []
    # 隔夜外盘：取最后一根（美股前夜收盘）及前一根算涨跌幅
    for symbol in hotspot_index_list():
        try:
            df = ak.index_us_stock_sina(symbol=symbol)
            if df is None or len(df) < 2:
                logger.warning("外盘 %s 无数据", symbol)
                continue
            last, prev = df.iloc[-1], df.iloc[-2]
            close = _f(last.get("close"))
            pc = _f(prev.get("close"))
            rows.append({
                "name": US_INDEX_NAMES.get(symbol, symbol),
                "code": symbol,
                "close": close,
                "change_pct": round((close / pc - 1) * 100, 2) if close and pc else None,
            })
        except Exception as e:
            logger.warning("外盘 %s 采集失败: %s", symbol, e)
    # A股指数（当日实时行情，早晨基于前收盘）
    try:
        df = ak.stock_zh_index_spot_em(symbol="沪深重要指数")
        name_col, px_col, pct_col = (
            _pick(df, "名称"), _pick(df, "最新价", "最新"), _pick(df, "涨跌幅"))
        if name_col and px_col and pct_col:
            for _, r in df.iterrows():
                if str(r[name_col]) in CN_INDEX_NAMES:
                    rows.append({
                        "name": str(r[name_col]), "code": str(r.get("代码", "")),
                        "close": _f(r[px_col]), "change_pct": _f(r[pct_col]),
                    })
    except Exception as e:
        logger.warning("A股指数采集失败: %s", e)
    return rows


def _fetch_ths_sectors() -> list[dict] | None:
    """同花顺行业板块概览一次拉取，返回归一化 [{name, change_pct, leader, net_inflow}]。
    失败返回 None（由调用方回退）。"""
    try:
        df = ak.stock_board_industry_summary_ths()
        name_col = _pick(df, "板块")
        pct_col = _pick(df, "涨跌幅")
        flow_col = _pick(df, "净流入")
        leader_col = _pick(df, "领涨股")
        if name_col is None:
            return None
        out = []
        for _, r in df.iterrows():
            raw_flow = _f(r[flow_col]) if flow_col else None
            out.append({
                "name": str(r[name_col]),
                "change_pct": _f(r[pct_col]) if pct_col else None,
                "leader": str(r[leader_col]) if leader_col else None,
                "net_inflow": _fmt_yi(raw_flow),   # 展示用：格式化字符串
                "net_inflow_raw": raw_flow,        # 排序用：原始数值（元已是亿）
            })
        return out
    except Exception as e:
        logger.warning("同花顺板块概览失败: %s", e)
        return None


def _sectors_gain_from(ths: list[dict] | None) -> list[dict]:
    """板块涨幅 TOP_N：主源同花顺（涨跌幅列），回退东财行业板块"""
    if ths:
        rows = sorted(ths, key=lambda x: x["change_pct"] or -999, reverse=True)[:HOTSPOT_TOP_N]
        return [{k: r[k] for k in ("name", "change_pct", "leader")} for r in rows]
    try:
        df = ak.stock_board_industry_name_em()
        name_col, pct_col, leader_col = (
            _pick(df, "板块名称", "名称"), _pick(df, "涨跌幅"), _pick(df, "领涨股票"))
        if not (name_col and pct_col):
            logger.warning("东财板块涨幅列缺失，跳过")
            return []
        out = []
        for _, r in df.sort_values(pct_col, ascending=False).head(HOTSPOT_TOP_N).iterrows():
            out.append({
                "name": str(r[name_col]),
                "change_pct": _f(r[pct_col]),
                "leader": str(r[leader_col]) if leader_col else None,
            })
        return out
    except Exception as e:
        logger.warning("东财板块涨幅榜失败: %s", e)
        return []


def _sectors_flow_from(ths: list[dict] | None) -> list[dict]:
    """板块资金净流入 TOP_N：主源同花顺（净流入列），回退东财主力净流入，
    再回退同花顺行业资金流（即时）。"""
    if ths:
        rows = sorted(ths, key=lambda x: x.get("net_inflow_raw") or -999,
                      reverse=True)[:HOTSPOT_TOP_N]
        return [{"name": r["name"], "net_inflow": r["net_inflow"]} for r in rows
                if r["net_inflow"] is not None]
    try:
        df = ak.stock_board_industry_name_em()
        name_col = _pick(df, "板块名称", "名称")
        flow_col = _pick(df, "主力净流入", "净流入")
        if name_col and flow_col:
            out = []
            for _, r in df.sort_values(flow_col, ascending=False).head(HOTSPOT_TOP_N).iterrows():
                out.append({"name": str(r[name_col]), "net_inflow": _fmt_flow(r[flow_col])})
            return out
    except Exception as e:
        logger.warning("东财板块资金流失败: %s", e)
    try:
        df = ak.stock_fund_flow_industry(symbol="即时")
        name_col = _pick(df, "行业")
        flow_col = _pick(df, "净额", "净流入", "净流出")
        if name_col and flow_col:
            out = []
            for _, r in df.sort_values(flow_col, ascending=False).head(HOTSPOT_TOP_N).iterrows():
                out.append({"name": str(r[name_col]), "net_inflow": _fmt_flow(r[flow_col])})
            return out
    except Exception as e:
        logger.warning("同花顺资金流采集失败: %s", e)
    return []


def _fetch_hot_rank() -> list[dict]:
    try:
        df = ak.stock_hot_rank_em()
        rank_col = _pick(df, "序号", "排名")
        code_col = _pick(df, "代码", "股票代码")
        name_col = _pick(df, "名称", "股票名称")
        pct_col = _pick(df, "涨跌幅")
        if not (name_col and code_col):
            logger.warning("人气榜列缺失，跳过")
            return []
        out = []
        for i, (_, r) in enumerate(df.head(HOTSPOT_TOP_N).iterrows(), 1):
            out.append({
                "rank": int(r[rank_col]) if rank_col and _f(r[rank_col]) is not None else i,
                "code": str(r[code_col]),
                "name": str(r[name_col]),
                "change_pct": _f(r[pct_col]) if pct_col else None,
            })
        return out
    except Exception as e:
        logger.warning("人气榜采集失败: %s", e)
        return []


def _fetch_zt_pool(spot_date: date) -> list[dict]:
    """涨停股池兜底：早盘时段 date 取当日，接口返回最近交易日涨停个股
    （「昨日涨停·今日关注」），语义契合早报活跃个股。失败/缺列返回 []。"""
    try:
        df = ak.stock_zt_pool_em(date=spot_date.strftime("%Y%m%d"))
        code_col = _pick(df, "代码", "股票代码")
        name_col = _pick(df, "名称", "股票名称")
        pct_col = _pick(df, "涨跌幅")
        board_col = _pick(df, "连板数")
        ind_col = _pick(df, "所属行业")
        if not (name_col and code_col):
            logger.warning("涨停池列缺失，跳过")
            return []
        out = []
        for i, (_, r) in enumerate(df.head(HOTSPOT_TOP_N).iterrows(), 1):
            days = _f(r[board_col]) if board_col else None
            pct = _f(r[pct_col]) if pct_col else None
            out.append({
                "rank": i,
                "code": str(r[code_col]),
                "name": str(r[name_col]).replace(" ", ""),  # 源数据偶含空格（中 关 村）
                "change_pct": round(pct, 2) if pct is not None else None,
                "board_days": int(days) if days is not None else 1,  # 连板数
                "industry": str(r[ind_col]) if ind_col else None,
            })
        return out
    except Exception as e:
        logger.warning("涨停池采集失败: %s", e)
        return []


def _fetch_hot_stocks(spot_date: date) -> list[dict]:
    """活跃个股：人气榜为主，涨停池兜底（人气榜网络不可达时降级）"""
    rows = _fetch_hot_rank()
    if rows:
        return rows
    return _fetch_zt_pool(spot_date)


class HotspotCollector(BaseCollector):
    """拉取市场热点快照并落 market_hotspot（按 spot_date upsert）"""

    def fetch(self, spot_date: date | None = None, *args, **kwargs):
        spot_date = spot_date or date.today()
        ths = _fetch_ths_sectors()  # 同花顺板块一次拉取，涨幅榜与资金流榜共用
        sections = {
            "indices": _fetch_indices(),
            "sectors_gain": _sectors_gain_from(ths),
            "sectors_flow": _sectors_flow_from(ths),
            "hot_stocks": _fetch_hot_stocks(spot_date),
        }
        logger.info("热点采集完成 %s：外盘 %s / 板块涨幅 %s / 资金流 %s / 人气 %s",
                    spot_date, len(sections["indices"]), len(sections["sectors_gain"]),
                    len(sections["sectors_flow"]), len(sections["hot_stocks"]))
        return {"spot_date": spot_date, "sections": sections}

    def save(self, data):
        if not data:
            return True
        upsert(self.db, MarketHotspot, [data],
               conflict_cols=["spot_date"], update_cols=["sections"])
        self.logger.info("市场热点入库 %s", data["spot_date"])
        return True


def sync_hotspot() -> bool:
    """手动触发入口：采集今日热点"""
    from app.db import get_session

    return HotspotCollector(get_session()).run(spot_date=date.today())


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
    ok = sync_hotspot()
    raise SystemExit(0 if ok else 1)

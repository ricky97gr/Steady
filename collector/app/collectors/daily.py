"""日行情采集器（Sprint 1 实现完整逻辑）"""
import logging

from app.collectors.base import BaseCollector

logger = logging.getLogger(__name__)


class DailyCollector(BaseCollector):
    """按股票代码拉取日K行情，经质量校验后入库"""

    def fetch(self, code: str, start_date: str, end_date: str, *args, **kwargs):
        # TODO(Sprint 1): ak.stock_zh_a_hist(code, period="daily", ...)
        # 复权因子用前复权口径（因子计算侧），原始价存 adj_factor
        logger.warning("DailyCollector.fetch 未实现（待 Sprint 1）: %s", code)
        return []

    def save(self, data):
        # TODO(Sprint 1): 数据质量校验（见文档 §7.7）后 UPSERT
        # - UNIQUE(code, trade_date) 去重
        # - OHLC 一致性、涨跌幅边界、volume>0（停牌不入库）
        pass

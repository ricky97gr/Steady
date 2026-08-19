"""股票列表采集器（Sprint 1 实现完整逻辑）"""
import logging

from app.collectors.base import BaseCollector

logger = logging.getLogger(__name__)


class StockCollector(BaseCollector):
    """从 AkShare 拉取全市场股票列表并入库"""

    def fetch(self, *args, **kwargs):
        # TODO(Sprint 1): ak.stock_info_a_code_name() 拉取列表
        # 标记 universe（沪深300/中证500 成分股）
        logger.warning("StockCollector.fetch 未实现（待 Sprint 1）")
        return []

    def save(self, data):
        # TODO(Sprint 1): UPSERT 到 stock_basic
        pass

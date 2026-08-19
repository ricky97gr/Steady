"""指数行情采集器（Sprint 1 实现完整逻辑）"""
import logging

from app.collectors.base import BaseCollector

logger = logging.getLogger(__name__)


class IndexCollector(BaseCollector):
    """拉取沪深300等指数行情，用于策略收益基准对比"""

    def fetch(self, index_code: str = "000300", *args, **kwargs):
        # TODO(Sprint 1): ak.index_zh_a_hist(symbol="000300", ...)
        # 存储到 daily_price（code 形如 "sh000300" 或以 index_ 前缀区分）
        logger.warning("IndexCollector.fetch 未实现（待 Sprint 1）: %s", index_code)
        return []

    def save(self, data):
        pass

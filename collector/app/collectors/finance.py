"""财务数据采集器（Sprint 1 实现完整逻辑）"""
import logging

from app.collectors.base import BaseCollector

logger = logging.getLogger(__name__)


class FinanceCollector(BaseCollector):
    """拉取财务指标，必须带公告日（announce_date）"""

    def fetch(self, code: str, *args, **kwargs):
        # TODO(Sprint 1): tushare 或 akshare 拉取 PE/PB/ROE/增长率/负债率
        # 注意：必须取 announce_date（公告日），否则因子计算产生未来函数
        logger.warning("FinanceCollector.fetch 未实现（待 Sprint 1）: %s", code)
        return []

    def save(self, data):
        # TODO(Sprint 1): 同一 (code, report_date) 重复时取 announce_date 最新
        pass

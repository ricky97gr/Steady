"""历史数据回填（Sprint 1 实现完整逻辑）

策略：只回填股票池（沪深300 + 中证500），分批限速、断点续传。
"""
import logging

from app.collectors.base import BaseCollector

logger = logging.getLogger(__name__)


class BackfillCollector(BaseCollector):
    """历史数据回填：批量拉取股票池内多年日行情与财务数据"""

    # 回填参数（每批 50-100 只，3 秒/只避免触发 AkShare 限速）
    batch_size = 50
    rate_limit_seconds = 3

    def fetch(self, *args, **kwargs):
        # TODO(Sprint 1):
        # 1. 从 stock_basic 取 universe 为 hs300/zz500 的股票
        # 2. 分批拉取日行情（batch_size=50），每只间隔 rate_limit_seconds
        # 3. 断点续传：进度写入 checkpoint 文件或表，中断后从上次位置继续
        logger.warning("BackfillCollector.fetch 未实现（待 Sprint 1）")
        return []

    def save(self, data):
        pass

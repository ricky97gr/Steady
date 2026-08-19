"""交易日历采集器（Sprint 1 实现完整逻辑）"""
import logging

from app.collectors.base import BaseCollector

logger = logging.getLogger(__name__)


class CalendarCollector(BaseCollector):
    """拉取 A 股交易日历（节假日多于周末，调度与回测以此为基准）"""

    def fetch(self, *args, **kwargs):
        # TODO(Sprint 1): ak.tool_trade_date_hist_sina() 拉取全年交易日
        # 入库到 trade_calendar（is_open = true / false）
        logger.warning("CalendarCollector.fetch 未实现（待 Sprint 1）")
        return []

    def save(self, data):
        pass

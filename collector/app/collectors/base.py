"""采集器基类：统一异常处理与重试逻辑"""
import logging
import time
from abc import ABC, abstractmethod
from datetime import date, datetime


def to_ak_date(value) -> str:
    """把 date / datetime / ISO 字符串统一成 AkShare 需要的 YYYYMMDD"""
    if isinstance(value, datetime):
        return value.strftime("%Y%m%d")
    if isinstance(value, date):
        return value.strftime("%Y%m%d")
    return str(value).replace("-", "").replace("/", "")


class BaseCollector(ABC):
    """所有采集器的基类。

    子类需实现 fetch()（拉取数据）与 save()（入库），
    run() 提供统一的重试与日志框架。
    """

    max_retries = 3
    retry_delay = 5  # seconds

    def __init__(self, db_session):
        self.db = db_session
        self.logger = logging.getLogger(self.__class__.__name__)

    @abstractmethod
    def fetch(self, *args, **kwargs):
        """从数据源拉取数据，返回记录列表"""
        raise NotImplementedError

    @abstractmethod
    def save(self, data):
        """将数据保存到数据库"""
        raise NotImplementedError

    def run(self, *args, **kwargs):
        """执行采集（带重试）"""
        for attempt in range(1, self.max_retries + 1):
            try:
                data = self.fetch(*args, **kwargs)
                self.save(data)
                self.logger.info("采集成功: %s 条", len(data))
                return True
            except Exception as e:
                self.logger.warning("第 %s 次重试: %s", attempt, e)
                # 回滚失败事务，否则后续重试报 current transaction is aborted
                try:
                    self.db.rollback()
                except Exception:
                    pass
                if attempt < self.max_retries:
                    time.sleep(self.retry_delay)
        self.logger.error("采集失败，已达最大重试次数")
        return False

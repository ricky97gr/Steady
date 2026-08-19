"""ORM 模型（与 deploy/postgres/init.sql 表结构一一对应）"""
from datetime import date, datetime

from sqlalchemy import (
    BigInteger,
    Boolean,
    Column,
    Date,
    DateTime,
    Integer,
    Numeric,
    String,
    func,
)
from sqlalchemy.orm import declarative_base

Base = declarative_base()


class StockBasic(Base):
    """股票基本信息"""

    __tablename__ = "stock_basic"

    code = Column(String(10), primary_key=True)
    name = Column(String(50), nullable=False)
    market = Column(String(10))  # SH / SZ / BJ
    industry = Column(String(50))
    list_date = Column(Date)
    status = Column(String(10), default="L")  # L=上市 / D=退市
    universe = Column(String(20))  # hs300 / zz500 / NULL=全市场
    created_at = Column(DateTime, server_default=func.now())
    updated_at = Column(DateTime, server_default=func.now(), onupdate=datetime.now)


class DailyPrice(Base):
    """日行情"""

    __tablename__ = "daily_price"
    __table_args__ = (
        {"comment": "日行情，UNIQUE(code, trade_date) 索引见 init.sql"},
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    code = Column(String(10), nullable=False)
    trade_date = Column(Date, nullable=False)
    open = Column(Numeric(10, 2))
    high = Column(Numeric(10, 2))
    low = Column(Numeric(10, 2))
    close = Column(Numeric(10, 2))
    volume = Column(BigInteger)  # 成交量（手）
    amount = Column(Numeric(15, 2))  # 成交额（元）
    adj_factor = Column(Numeric(10, 4))  # 复权因子


class FinancialIndicator(Base):
    """财务指标（announce_date 公告日，防止未来函数）"""

    __tablename__ = "financial_indicator"

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    code = Column(String(10), nullable=False)
    report_date = Column(Date, nullable=False)  # 报告期
    pe = Column(Numeric(10, 2))
    pb = Column(Numeric(10, 2))
    roe = Column(Numeric(10, 4))
    profit_growth = Column(Numeric(10, 4))
    revenue_growth = Column(Numeric(10, 4))
    debt_ratio = Column(Numeric(10, 4))
    gross_margin = Column(Numeric(10, 4))
    announce_date = Column(Date, nullable=False)  # 公告日
    created_at = Column(DateTime, server_default=func.now())


class DailyValuation(Base):
    """每日估值（东财：PE(TTM)/PE(静)/PB/市值，Sprint 4 新增）"""

    __tablename__ = "daily_valuation"

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    code = Column(String(10), nullable=False)
    trade_date = Column(Date, nullable=False)
    close = Column(Numeric(10, 2))
    total_mv = Column(Numeric(18, 2))  # 总市值（元）
    float_mv = Column(Numeric(18, 2))  # 流通市值（元）
    pe_ttm = Column(Numeric(12, 4))  # 市盈率 TTM
    pe_static = Column(Numeric(12, 4))  # 市盈率（静态）
    pb = Column(Numeric(12, 4))  # 市净率
    created_at = Column(DateTime, server_default=func.now())


class TradeCalendar(Base):
    """交易日历"""

    __tablename__ = "trade_calendar"

    cal_date = Column(Date, primary_key=True)
    is_open = Column(Boolean, nullable=False)
    exchange = Column(String(10), default="SSE")

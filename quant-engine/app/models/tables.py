"""ORM 模型（与 deploy/postgres/init.sql 表结构一一对应，仅引擎用到的表）"""
from datetime import datetime

from sqlalchemy import (
    JSON,
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
    status = Column(String(10), default="L")
    universe = Column(String(20))  # hs300 / zz500 / NULL
    created_at = Column(DateTime, server_default=func.now())
    updated_at = Column(DateTime, server_default=func.now(), onupdate=datetime.now)


class DailyPrice(Base):
    """日行情（含复权因子；因子计算用前复权价）"""

    __tablename__ = "daily_price"

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    code = Column(String(10), nullable=False)
    trade_date = Column(Date, nullable=False)
    open = Column(Numeric(10, 2))
    high = Column(Numeric(10, 2))
    low = Column(Numeric(10, 2))
    close = Column(Numeric(10, 2))
    volume = Column(BigInteger)
    amount = Column(Numeric(15, 2))
    adj_factor = Column(Numeric(10, 4))


class FinancialIndicator(Base):
    """财务指标（announce_date 公告日，防止未来函数）"""

    __tablename__ = "financial_indicator"

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    code = Column(String(10), nullable=False)
    report_date = Column(Date, nullable=False)
    pe = Column(Numeric(10, 2))
    pb = Column(Numeric(10, 2))
    roe = Column(Numeric(10, 4))
    profit_growth = Column(Numeric(10, 4))
    revenue_growth = Column(Numeric(10, 4))
    debt_ratio = Column(Numeric(10, 4))
    gross_margin = Column(Numeric(10, 4))
    announce_date = Column(Date, nullable=False)
    created_at = Column(DateTime, server_default=func.now())


class DailyValuation(Base):
    """每日估值（东财：PE(TTM)/PE(静)/PB/市值）"""

    __tablename__ = "daily_valuation"

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    code = Column(String(10), nullable=False)
    trade_date = Column(Date, nullable=False)
    close = Column(Numeric(10, 2))
    total_mv = Column(Numeric(18, 2))
    float_mv = Column(Numeric(18, 2))
    pe_ttm = Column(Numeric(12, 4))
    pe_static = Column(Numeric(12, 4))
    pb = Column(Numeric(12, 4))
    created_at = Column(DateTime, server_default=func.now())


class TradeCalendar(Base):
    """交易日历"""

    __tablename__ = "trade_calendar"

    cal_date = Column(Date, primary_key=True)
    is_open = Column(Boolean, nullable=False)
    exchange = Column(String(10), default="SSE")


class FactorDefinition(Base):
    """因子定义（权重默认值来自 init.sql 预置）"""

    __tablename__ = "factor_definition"

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    name = Column(String(50), unique=True, nullable=False)
    category = Column(String(20))
    description = Column(String)
    formula = Column(String)
    weight = Column(Numeric(5, 4))
    created_at = Column(DateTime, server_default=func.now())


class FactorValue(Base):
    """因子值（横截面归一化结果）"""

    __tablename__ = "factor_value"

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    code = Column(String(10), nullable=False)
    factor_name = Column(String(50), nullable=False)
    trade_date = Column(Date, nullable=False)
    value = Column(Numeric(15, 6))
    rank = Column(Integer)
    normalized = Column(Numeric(8, 6))
    created_at = Column(DateTime, server_default=func.now())


class Strategy(Base):
    """策略定义（factor_weights/params 为 JSONB）"""

    __tablename__ = "strategy"

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    name = Column(String(50), unique=True, nullable=False)
    description = Column(String)
    factor_weights = Column(JSON)  # JSONB
    params = Column(JSON)  # JSONB
    status = Column(String(10), default="active")
    created_at = Column(DateTime, server_default=func.now())


class StrategySignal(Base):
    """策略信号"""

    __tablename__ = "strategy_signal"

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    strategy_name = Column(String(50), nullable=False)
    code = Column(String(10), nullable=False)
    trade_date = Column(Date, nullable=False)
    score = Column(Numeric(8, 4))
    action = Column(String(10))  # BUY / SELL / HOLD
    reason = Column(String)
    created_at = Column(DateTime, server_default=func.now())


class BacktestJob(Base):
    """回测任务（pending → running → done/failed；同参数唯一幂等）"""

    __tablename__ = "backtest_job"

    id = Column(BigInteger, primary_key=True, autoincrement=True)
    strategy_name = Column(String(64), nullable=False, default="multi_factor")
    start_date = Column(Date, nullable=False)
    end_date = Column(Date, nullable=False)
    top_n = Column(Integer, nullable=False, default=20)
    status = Column(String(16), nullable=False, default="pending")
    error = Column(String)
    created_at = Column(DateTime(timezone=True), server_default=func.now())
    finished_at = Column(DateTime(timezone=True))


class BacktestResult(Base):
    """回测结果（指标 + 净值序列 JSONB，job 删除级联）"""

    __tablename__ = "backtest_result"

    job_id = Column(BigInteger, primary_key=True)
    total_return = Column(Numeric(10, 4))
    annualized_return = Column(Numeric(10, 4))
    max_drawdown = Column(Numeric(10, 4))
    sharpe = Column(Numeric(10, 4))
    trading_days = Column(Integer)
    final_value = Column(Numeric(14, 2))
    trades = Column(Integer)
    positions = Column(Integer)
    benchmark_return = Column(Numeric(10, 4))
    excess_return = Column(Numeric(10, 4))
    nav = Column(JSON)  # JSONB [{"date","nav","benchmark"}]
    created_at = Column(DateTime(timezone=True), server_default=func.now())

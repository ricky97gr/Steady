"""数据库连接管理（SQLAlchemy，配置来自环境变量）"""
import os

from sqlalchemy import create_engine
from sqlalchemy.engine import Engine
from sqlalchemy.orm import sessionmaker, Session


def get_dsn() -> str:
    host = os.getenv("DB_HOST", "localhost")
    port = os.getenv("DB_PORT", "5432")
    user = os.getenv("DB_USER", "quant")
    password = os.getenv("DB_PASSWORD", "")
    name = os.getenv("DB_NAME", "quant_system")
    return f"postgresql+psycopg2://{user}:{password}@{host}:{port}/{name}"


def create_db_engine() -> Engine:
    """创建数据库引擎（连接池：5 + 10）"""
    return create_engine(get_dsn(), pool_size=5, max_overflow=10, pool_pre_ping=True)


def get_session() -> Session:
    """获取一个新的数据库会话"""
    factory = sessionmaker(bind=create_db_engine())
    return factory()

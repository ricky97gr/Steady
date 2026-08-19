"""数据库连接管理（与 collector 共用同一套环境变量约定）"""
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
    return create_engine(get_dsn(), pool_size=5, max_overflow=10, pool_pre_ping=True)


def get_session() -> Session:
    factory = sessionmaker(bind=create_db_engine())
    return factory()

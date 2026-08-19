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


def upsert(session: Session, model, rows: list[dict],
           conflict_cols: list[str], update_cols: list[str]) -> int:
    """Postgres INSERT ... ON CONFLICT DO UPDATE（与 collector/app/db.py 同模式）"""
    from sqlalchemy.dialects.postgresql import insert as pg_insert

    if not rows:
        return 0
    stmt = pg_insert(model).values(list(rows))
    stmt = stmt.on_conflict_do_update(
        index_elements=list(conflict_cols),
        set_={col: stmt.excluded[col] for col in update_cols},
    )
    session.execute(stmt)
    session.commit()
    return len(rows)

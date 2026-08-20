"""任务执行记录（task_run 表）：监控/对账的统一账本

每个关键任务（因子/信号/交易/净值/日报/回测/通知）完成后写一行，
status ∈ success/skipped/failed；同 (task_name, run_date) 幂等 upsert。
- 通知调度器据此做「该做没做」检查与失败告警
- 页面据此展示最近任务执行状态
- detail 为结构化明细，供后续大模型消费
"""
import logging

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert as pg_insert

from app.models.tables import TaskRun

logger = logging.getLogger("task_run")


def record_task(db, task_name: str, run_date, status: str,
                message: str = "", detail=None) -> None:
    """幂等写一条任务执行记录（失败只记日志，不影响业务）"""
    if detail is None:
        detail = {}
    try:
        stmt = pg_insert(TaskRun).values(
            task_name=task_name, run_date=run_date, status=status,
            message=message, detail=detail,
        ).on_conflict_do_update(
            index_elements=["task_name", "run_date"],
            set_={"status": status, "message": message, "detail": detail},
            # created_at 不在更新列 → 冲突时保留首次创建时间，内容取最新
        )
        db.execute(stmt)
        db.commit()
    except Exception:
        db.rollback()
        logger.exception("记录 task_run 失败: %s %s", task_name, run_date)


def get_task_status(db, task_name: str, run_date) -> str | None:
    """最近一次执行状态：success/skipped/failed；从未执行返回 None"""
    try:
        row = db.execute(
            select(TaskRun).where(
                TaskRun.task_name == task_name, TaskRun.run_date == run_date)
        ).scalar()
        return row.status if row else None
    except Exception:
        db.rollback()
        return None


def already_run(db, task_name: str, run_date) -> bool:
    """该任务当日是否已有执行记录（含失败），用于去重"""
    return get_task_status(db, task_name, run_date) is not None


def list_recent(db, limit: int = 20) -> list[TaskRun]:
    """最近任务执行记录（页面展示用）"""
    return db.execute(
        select(TaskRun).order_by(TaskRun.run_date.desc(),
                                 TaskRun.created_at.desc()).limit(limit)
    ).scalars().all()

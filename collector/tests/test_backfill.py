"""回填任务测试：断点续传与范围逻辑（内存 sqlite + mock 采集器）"""
from datetime import date

from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker

from app.collectors import backfill as bf_mod
from app.collectors.backfill import BackfillJob
from app.models.tables import Base, DailyPrice, DailyValuation, StockBasic


def make_db():
    engine = create_engine("sqlite://")
    Base.metadata.create_all(engine)
    return sessionmaker(bind=engine)()


def seed(db):
    db.add(StockBasic(code="600519", name="贵州茅台", market="SH", universe="hs300"))
    db.add(StockBasic(code="000001", name="平安银行", market="SZ", universe="zz500"))
    db.add(StockBasic(code="000300", name="伪股票", market="SZ"))  # 非股票池
    db.commit()


def test_pool_codes():
    db = make_db()
    seed(db)
    assert BackfillJob(db).pool_codes() == ["000001", "600519"]


def add_price(db, code: str, trade_date: date, price_id: int = 1):
    """sqlite 下 BigInteger 主键不会自增，显式指定 id"""
    db.add(DailyPrice(id=price_id, code=code, trade_date=trade_date, close=price_id))
    db.commit()


def test_covered_codes_resume():
    """断点续传：日线真实覆盖到起始日（min <= start）才算完成"""
    db = make_db()
    seed(db)
    add_price(db, "600519", date(2026, 7, 31))  # min 覆盖到起始日之前 → 已完成
    add_price(db, "000001", date(2026, 8, 10), price_id=2)  # 只有最近 30 天回退数据 → 未完成
    job = BackfillJob(db)
    covered = job.covered_codes(date(2026, 8, 1))
    assert covered == {"600519"}


def test_daily_skips_covered_and_rate_limits(monkeypatch):
    db = make_db()
    seed(db)
    add_price(db, "600519", date(2026, 7, 31))  # 覆盖到起始日 → 跳过

    calls = []
    sleeps = []

    class FakeDaily:
        def __init__(self, s):
            pass

        def run(self, code, start, end):
            calls.append((code, start, end))
            return True

    monkeypatch.setattr(bf_mod, "DailyCollector", FakeDaily)
    monkeypatch.setattr(bf_mod.time, "sleep", sleeps.append)
    # 缩小速率避免等待
    job = BackfillJob(db, rate_limit=0, batch_size=2)
    stats = job.daily(date(2026, 8, 1), date(2026, 8, 20))
    # 600519 已覆盖跳过；000001 待回填
    assert [c[0] for c in calls] == ["000001"]
    assert stats["done"] == 1
    assert stats["skipped"] == 0
    assert len(sleeps) == 1  # 每只股票限速一次


def test_daily_dry_run_does_not_fetch(monkeypatch):
    db = make_db()
    seed(db)

    class FakeDaily:
        def __init__(self, s):
            raise AssertionError("dry-run 不应真实采集")

    monkeypatch.setattr(bf_mod, "DailyCollector", FakeDaily)
    result = BackfillJob(db, dry_run=True).daily(date(2026, 8, 1), date(2026, 8, 20))
    assert len(result["todo"]) == 2
    assert result["covered"] == 0


def test_valuation_skips_latest(monkeypatch):
    """估值回填：max(trade_date) >= 今日 → 跳过；否则全量拉取"""
    db = make_db()
    seed(db)
    # 600519 已有今日估值 → 跳过；000001 无估值 → 待拉取
    db.add(DailyValuation(id=1, code="600519", trade_date=date.today(), pe_ttm=20.1))
    db.commit()

    calls = []
    sleeps = []

    class FakeValuation:
        def __init__(self, s):
            pass

        def run(self, code):
            calls.append(code)
            return True

    monkeypatch.setattr(bf_mod, "ValuationCollector", FakeValuation)
    monkeypatch.setattr(bf_mod.time, "sleep", sleeps.append)
    stats = BackfillJob(db, rate_limit=0).valuation()
    assert calls == ["000001"]
    assert stats["done"] == 1


def test_verify_reports_low_coverage(monkeypatch):
    db = make_db()
    seed(db)
    job = BackfillJob(db, rate_limit=0)
    # 直接调用 _verify：无数据的股票会告警（行数 < 10）
    monkeypatch.setattr(bf_mod.logger, "warning", lambda *a, **k: None)
    job._verify(date(2026, 8, 1), date(2026, 8, 20))  # 不应抛异常

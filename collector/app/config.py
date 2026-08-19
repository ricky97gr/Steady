"""采集服务配置（环境变量，覆盖默认值）"""
import os


def _int(name: str, default: int) -> int:
    return int(os.getenv(name, str(default)))


def _str(name: str, default: str) -> str:
    return os.getenv(name, default)


# 请求限速（秒/只）：回填与每日同步共用，避免触发 AkShare 限速
RATE_LIMIT_SECONDS = _int("COLLECTOR_RATE_LIMIT", 3)

# 回填批次大小（只）
BACKFILL_BATCH_SIZE = _int("COLLECTOR_BATCH_SIZE", 50)

# 回填历史区间（默认近 10 年）
BACKFILL_START = _str("COLLECTOR_BACKFILL_START", "20160801")
BACKFILL_END = _str("COLLECTOR_BACKFILL_END", "")

# 财务回填报告期数（默认 20 个季度 ≈ 5 年）
BACKFILL_FINANCE_QUARTERS = _int("COLLECTOR_FINANCE_QUARTERS", 20)

# 每日增量财务同步的报告期数（默认最近 4 个季度，覆盖财报季尾部披露）
FINANCE_SYNC_QUARTERS = _int("COLLECTOR_FINANCE_QUARTERS_SYNC", 4)

# 每日增量同步的指数（沪深300 / 中证500，作收益基准）
INDEX_CODES = _str("COLLECTOR_INDEX_CODES", "sh000300,sh000905")

# 每日增量同步：数据库无历史记录时的回退窗口（天）
DAILY_FALLBACK_DAYS = _int("COLLECTOR_DAILY_FALLBACK_DAYS", 30)

# 每日增量同步的股票间间隔（秒，回填用 RATE_LIMIT_SECONDS）
DAILY_SYNC_INTERVAL = _int("COLLECTOR_DAILY_INTERVAL", 1)


def index_code_list() -> list[str]:
    return [c.strip() for c in INDEX_CODES.split(",") if c.strip()]

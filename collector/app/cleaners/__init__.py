"""数据清洗与质量校验（文档 §7.7）

采集入库前统一执行。
"""
import logging

logger = logging.getLogger(__name__)

# 涨跌停幅度（按板块）：主板 10%，创业板/科创板 20%，北交所 30%，ST 5%
PRICE_LIMIT = {
    "main": 0.10,
    "st": 0.05,
    "chinext": 0.20,   # 创业板 30xxxx
    "star": 0.20,      # 科创板 688xxx
    "bse": 0.30,       # 北交所 8xxxxx / 4xxxxx
}


def board_of(code: str) -> str:
    """根据代码段判断板块"""
    if code.startswith(("688", "689")):
        return "star"
    if code.startswith(("300", "301")):
        return "chinext"
    if code.startswith(("8", "4", "92")):
        return "bse"
    return "main"


def validate_ohlc(row) -> list[str]:
    """OHLC 一致性 + 价格边界校验，返回错误列表（空=通过）"""
    errors = []
    o, h, l, c = row.get("open"), row.get("high"), row.get("low"), row.get("close")
    if None in (o, h, l, c):
        errors.append("价格字段缺失")
        return errors
    if min(o, h, l, c) <= 0:
        errors.append("价格非正")
    if h < max(o, c) or l > min(o, c):
        errors.append("OHLC 不一致")
    return errors


def validate_price_change(code: str, close: float, prev_close: float) -> bool:
    """涨跌幅边界校验：|涨跌幅| <= 该板块涨跌停幅度"""
    if prev_close is None or prev_close <= 0:
        return True
    limit = PRICE_LIMIT.get(board_of(code), 0.10)
    change = abs(close / prev_close - 1)
    return change <= limit + 1e-6


def validate_volume(row) -> bool:
    """成交校验：volume > 0（停牌日无成交记录，不入库）"""
    vol = row.get("volume")
    return vol is not None and vol > 0


def clean_daily_rows(code: str, rows: list[dict]) -> list[dict]:
    """清洗日行情记录：返回通过校验的行，异常行记日志。

    处理策略（文档 §7.7）：
    - OHLC 不一致 / 价格非正 / 缺字段 / volume<=0 → 丢弃
    - 涨跌幅超限（如新股上市前 5 日无涨跌幅限制）→ 告警并保留，人工复核
    """
    clean = []
    for row in rows:
        errs = validate_ohlc(row)
        if errs or not validate_volume(row):
            logger.warning("数据异常丢弃 %s %s: %s", code, row.get("trade_date"),
                           errs or "volume<=0")
            continue
        if not validate_price_change(code, row.get("close"), row.get("prev_close")):
            logger.warning("涨跌幅超限待复核 %s %s: close=%s prev_close=%s",
                           code, row.get("trade_date"),
                           row.get("close"), row.get("prev_close"))
        clean.append(row)
    return clean

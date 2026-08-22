"""测试助手"""
from sqlalchemy.sql import Select


def write_execs(db) -> list:
    """过滤掉只读查询（配置读取等），只留写入语句。

    collector 各采集器 fetch 现在会先读 app_config 拿 Tushare token，
    FakeSession.executed 里会多一条 select；断言"第几次 execute"时应先过滤。
    """
    return [s for s in db.executed if not isinstance(s, Select)]


def row_values(r: dict) -> dict:
    """把 execute 捕获的 insert 行转成 字符串键 dict。

    SQLAlchemy 行为：values 键全部命中表列时行内键为 Column 对象；
    存在非表列键（如 prev_close）时整批退化为字符串键。两种都兼容。
    """
    return {getattr(k, "key", k): v for k, v in r.items()}


def multi_values(stmt) -> list[dict]:
    """_multi_values → 字符串键行列表"""
    return [row_values(r) for r in stmt._multi_values[0]]

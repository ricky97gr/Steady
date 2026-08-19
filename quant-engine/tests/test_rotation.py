"""轮动调仓规则单元测试（docs §8.3 缓冲带边界）"""
from app.strategies.multi_factor import rotation_action


def test_buy_within_buy_buffer():
    assert rotation_action(1, held=False, buy_buffer=15, sell_buffer=30) == "BUY"
    assert rotation_action(15, held=False, buy_buffer=15, sell_buffer=30) == "BUY"  # 边界


def test_hold_outside_buy_buffer():
    assert rotation_action(16, held=False, buy_buffer=15, sell_buffer=30) == "HOLD"
    assert rotation_action(50, held=False, buy_buffer=15, sell_buffer=30) == "HOLD"


def test_hold_inside_sell_buffer():
    assert rotation_action(30, held=True, buy_buffer=15, sell_buffer=30) == "HOLD"  # 边界
    assert rotation_action(1, held=True, buy_buffer=15, sell_buffer=30) == "HOLD"


def test_sell_beyond_sell_buffer():
    assert rotation_action(31, held=True, buy_buffer=15, sell_buffer=30) == "SELL"  # 边界
    assert rotation_action(300, held=True, buy_buffer=15, sell_buffer=30) == "SELL"


def test_buy_after_sell_same_day():
    """同日先卖后买的仓位状态不重叠：卖出后即视为未持仓"""
    assert rotation_action(5, held=False, buy_buffer=15, sell_buffer=30) == "BUY"

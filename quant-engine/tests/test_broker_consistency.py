"""成交规则一致性对照测试：与 backend/internal/service/trading_consistency_test.go 共用同一 fixture。

两端必须逐步得到相同数值（现金/持仓/成交价/费用），否则交付前修改任一侧需同步另一侧。
费率数值来自 deploy/backend/configs/config.yaml（Broker 默认值与其一致）。

round2 与 Go math.Round(v*100)/100 同语义（远离零四舍五入）；
fixture 的 d2 卖出价刻意选 20.00 → 成交价 19.98、金额 19980.00、印花税 9.99 均为
精确两位小数，避免 Python 端不逐笔舍入导致的边界分歧。

Fixture（2 个交易日，top_n=10, max_position_pct=0.20，初始资金 100000）：
  d1 收盘 600000=10.00, 000001=20.00（前收 9.50/19.00，无涨跌停干扰）
      BUY 600000: 预算=min(100000/10, 100000×0.2)=10000 → 1000 股
                  成交价=round2(10.00×1.001)=10.01，金额=10010.00，
                  佣金=round2(max(10010×0.00025,5))=5.00（下限）→ 现金 89985.00，持仓 1000@10.01
      BUY 000001: 总资产=89985.00+10010=99995.00 → 预算=9999.50 → 400 股
                  成交价=round2(20.00×1.001)=20.02，金额=8008.00，佣金=5.00（下限）
                  → 现金 81972.00，持仓 400@20.02
  d2 解冻 T+1；收盘 600000=20.00
      SELL 600000 1000 股：成交价=round2(20.00×0.999)=19.98，金额=19980.00
                    佣金=5.00（下限），印花税=round2(19980×0.0005)=9.99，回款=19980-5-9.99=19965.01
                    → 现金 101937.01，仅剩 000001 400@20.02
  d2 总资产 = 101937.01 + 400×20.02 = 109945.01
"""
import math

from app.backtest.broker import Broker, limit_ratio_of
from app.backtest.portfolio import Portfolio


def round2(v: float) -> float:
    """与 Go math.Round(v*100)/100 同语义"""
    return math.floor(v * 100 + 0.5) / 100


def test_consistency_fixture():
    b = Broker()
    p = Portfolio(initial_cash=100000)

    # ---- d1：BUY 600000 ----
    budget = min(p.total_value / 10, p.total_value * 0.20)
    qty = int(budget / 10.00 // 100) * 100
    assert qty == 1000, qty
    exec_price = round2(10.00 * (1 + Broker.SLIPPAGE))
    assert exec_price == 10.01, exec_price
    amount = round2(exec_price * qty)
    commission = round2(b.calc_commission(amount))
    assert (amount, commission) == (10010.00, 5.00), (amount, commission)
    assert b.execute_buy(p, "600000", 10.00, qty, prev_close=9.50, trade_date="d1")
    assert round2(p.cash) == 89985.00, p.cash
    pos = p.positions["600000"]
    assert (pos.available_qty, round2(pos.cost_price)) == (0, 10.01), (pos.available_qty, pos.cost_price)

    # ---- d1：BUY 000001（资金逐笔演化）----
    budget = min(p.total_value / 10, p.total_value * 0.20)
    qty = int(budget / 20.00 // 100) * 100
    assert qty == 400, qty
    exec_price = round2(20.00 * (1 + Broker.SLIPPAGE))
    assert exec_price == 20.02, exec_price
    amount = round2(exec_price * qty)
    commission = round2(b.calc_commission(amount))
    assert (amount, commission) == (8008.00, 5.00), (amount, commission)
    assert b.execute_buy(p, "000001", 20.00, qty, prev_close=19.00, trade_date="d1")
    assert round2(p.cash) == 81972.00, p.cash

    # ---- d2：解冻 + SELL 600000 ----
    for pos in p.positions.values():  # 引擎 _unfreeze_t1：全部解冻
        pos.available_qty = pos.quantity
    assert p.positions["600000"].available_qty == 1000
    exec_price = round2(20.00 * (1 - Broker.SLIPPAGE))
    assert exec_price == 19.98, exec_price
    amount = round2(exec_price * 1000)
    commission = round2(b.calc_commission(amount))
    tax = round2(b.calc_tax(amount))
    assert (amount, commission, tax) == (19980.00, 5.00, 9.99), (amount, commission, tax)
    assert b.execute_sell(p, "600000", 20.00, 1000, prev_close=10.00)
    assert round2(p.cash) == 101937.01, p.cash
    assert "600000" not in p.positions
    pos = p.positions["000001"]
    assert (pos.quantity, round2(pos.cost_price)) == (400, 20.02), (pos.quantity, pos.cost_price)

    assert round2(p.total_value) == 109945.01, p.total_value


def test_limit_ratio_consistency():
    """涨跌停幅度与 Go LimitRatioOf 一致"""
    assert limit_ratio_of("600519") == 0.10
    assert limit_ratio_of("000001") == 0.10
    assert limit_ratio_of("300750") == 0.20
    assert limit_ratio_of("688981") == 0.20
    assert limit_ratio_of("830799") == 0.30

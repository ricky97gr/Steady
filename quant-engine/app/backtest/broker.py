"""成交模拟：手续费、印花税、滑点、涨跌停（A股规则）

费率与 Backend Service（Go）共用同一份 config.yaml 的数值，
交付前跑一致性对照测试（见文档 §9.4）。
"""
from app.backtest.portfolio import Portfolio

# 板块涨跌停幅度
PRICE_LIMIT = {
    "main": 0.10,     # 主板
    "st": 0.05,       # ST
    "chinext": 0.20,  # 创业板 30xxxx
    "star": 0.20,     # 科创板 688xxx
    "bse": 0.30,      # 北交所 8xxxxx / 4xxxxx
}


def limit_ratio_of(code: str) -> float:
    if code.startswith(("688", "689")):
        return PRICE_LIMIT["star"]
    if code.startswith(("300", "301")):
        return PRICE_LIMIT["chinext"]
    if code.startswith(("8", "4", "92")):
        return PRICE_LIMIT["bse"]
    return PRICE_LIMIT["main"]


class Broker:
    """模拟券商成交"""

    COMMISSION_RATE = 0.00025   # 万2.5
    MIN_COMMISSION = 5.0        # 最低5元
    STAMP_TAX_RATE = 0.0005     # 印花税 万5（仅卖出）
    SLIPPAGE = 0.001            # 滑点 0.1%

    def calc_commission(self, amount: float) -> float:
        return max(amount * self.COMMISSION_RATE, self.MIN_COMMISSION)

    def calc_tax(self, amount: float) -> float:
        return amount * self.STAMP_TAX_RATE

    def _check_price_limit(self, code: str, price: float, prev_close: float, direction: str) -> bool:
        """涨跌停检查：涨停买不进、跌停卖不出"""
        if prev_close is None or prev_close <= 0:
            return True
        limit = limit_ratio_of(code)
        if direction == "BUY" and price >= prev_close * (1 + limit):
            return False
        if direction == "SELL" and price <= prev_close * (1 - limit):
            return False
        return True

    def execute_buy(self, portfolio: Portfolio, code: str, price: float, qty: int,
                    prev_close: float = None, trade_date: str = "") -> bool:
        if prev_close is not None and not self._check_price_limit(code, price, prev_close, "BUY"):
            return False  # 涨停无法成交，跳过
        exec_price = price * (1 + self.SLIPPAGE)
        commission = self.calc_commission(exec_price * qty)
        portfolio.buy(code, exec_price, qty, commission, trade_date)
        return True

    def execute_sell(self, portfolio: Portfolio, code: str, price: float, qty: int,
                     prev_close: float = None) -> bool:
        if prev_close is not None and not self._check_price_limit(code, price, prev_close, "SELL"):
            return False  # 跌停无法成交，跳过
        exec_price = price * (1 - self.SLIPPAGE)
        commission = self.calc_commission(exec_price * qty)
        tax = self.calc_tax(exec_price * qty)
        portfolio.sell(code, exec_price, qty, commission, tax)
        return True

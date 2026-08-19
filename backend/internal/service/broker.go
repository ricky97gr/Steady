package service

import (
	"math"
	"strings"

	"quant-system/backend/internal/config"
)

// 本文件为 quant-engine/app/backtest/broker.py 的 Go 移植，
// 费率从 config.yaml 读取（与 Python 共用同一份数值，见 config.go 注释）。
// 修改任何常量/规则必须同时更新 broker.py 并跑一致性对照测试。

// Round2 保留两位小数（货币金额统一舍入点）
func Round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// Round4 保留四位小数（收益率）
func Round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

// Round6 保留六位小数（净值）
func Round6(v float64) float64 {
	return math.Round(v*1000000) / 1000000
}

// PriceLimitRatio 板块涨跌停幅度（broker.py PRICE_LIMIT）
var PriceLimitRatio = map[string]float64{
	"main":    0.10, // 主板
	"st":      0.05, // ST
	"chinext": 0.20, // 创业板 30xxxx
	"star":    0.20, // 科创板 688xxx
	"bse":     0.30, // 北交所 8xxxxx / 4xxxxx
}

// LimitRatioOf 按代码段返回涨跌停幅度（broker.py limit_ratio_of）
func LimitRatioOf(code string) float64 {
	if len(code) >= 3 {
		prefix := code[:3]
		if prefix == "688" || prefix == "689" {
			return PriceLimitRatio["star"]
		}
		if prefix == "300" || prefix == "301" {
			return PriceLimitRatio["chinext"]
		}
	}
	if code[0] == '8' || code[0] == '4' || strings.HasPrefix(code, "92") {
		return PriceLimitRatio["bse"]
	}
	return PriceLimitRatio["main"]
}

// Broker 模拟券商成交（纯函数，不持有状态）
type Broker struct {
	account config.AccountConfig
}

func NewBroker(account config.AccountConfig) *Broker {
	return &Broker{account: account}
}

// Commission 手续费 = max(金额×费率, 最低佣金)
func (b *Broker) Commission(amount float64) float64 {
	return math.Max(amount*b.account.CommissionRate, b.account.MinCommission)
}

// Tax 印花税（仅卖出，费率 × 金额）
func (b *Broker) Tax(amount float64) float64 {
	return amount * b.account.StampTaxRate
}

// BuyExecPrice 买入成交价 = 收盘价 × (1 + 滑点)，保留两位
func (b *Broker) BuyExecPrice(close float64) float64 {
	return Round2(close * (1 + b.account.Slippage))
}

// SellExecPrice 卖出成交价 = 收盘价 × (1 - 滑点)，保留两位
func (b *Broker) SellExecPrice(close float64) float64 {
	return Round2(close * (1 - b.account.Slippage))
}

// CheckPriceLimit 涨跌停检查：涨停买不进、跌停卖不出（broker.py _check_price_limit）
// prevClose <= 0（无前收，如上市首日）视为无限制
func (b *Broker) CheckPriceLimit(code string, price, prevClose float64, direction string) bool {
	if prevClose <= 0 {
		return true
	}
	limit := LimitRatioOf(code)
	if direction == "BUY" && price >= prevClose*(1+limit) {
		return false
	}
	if direction == "SELL" && price <= prevClose*(1-limit) {
		return false
	}
	return true
}

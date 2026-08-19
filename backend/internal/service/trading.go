package service

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"quant-system/backend/internal/config"
	"quant-system/backend/internal/model"
	"quant-system/backend/internal/repository"
)

// 本文件交易规则移植自 quant-engine/app/backtest/portfolio.py + engine.py
// （T+1 冻结 / 100 股整手 / 加权平均成本 / 逐笔演化），费率见 broker.go。
// 任何规则变更必须同步 Python 侧并跑一致性对照测试。

// 类型化错误（handler 映射为 400）
var (
	ErrInvalidDirection = errors.New("无效的方向，仅支持 BUY/SELL")
	ErrNotLotSize       = errors.New("委托数量必须为100股整数倍")
	ErrInvalidPrice     = errors.New("委托价格必须大于0")
	ErrPriceLimit       = errors.New("委托价格超出涨跌停范围")
	ErrNoPosition       = errors.New("无持仓")
	ErrT1Unavailable    = errors.New("可用持仓不足（T+1 限制）")
	ErrInsufficientCash = errors.New("资金不足")
	ErrOrderNotFound    = errors.New("委托不存在")
	ErrNotCancellable   = errors.New("仅待成交委托可撤销")
	ErrStockMissing     = errors.New("股票不存在")
)

// PositionState 持仓状态（镜像 portfolio.py Position）
type PositionState struct {
	Code         string
	Quantity     int
	AvailableQty int // T+1：当日买入冻结，次一交易日解冻
	CostPrice    float64
	CurrentPrice float64
}

func (p *PositionState) MarketValue() float64 { return float64(p.Quantity) * p.CurrentPrice }
func (p *PositionState) Profit() float64      { return float64(p.Quantity) * (p.CurrentPrice - p.CostPrice) }

// Ledger 纯内存账本（镜像 portfolio.py Portfolio）
type Ledger struct {
	initialCash float64
	cash        float64
	positions   map[string]*PositionState
}

func NewLedger(initialCash, cash float64) *Ledger {
	return &Ledger{initialCash: initialCash, cash: cash, positions: map[string]*PositionState{}}
}

// Buy 买入：整手校验 → 资金校验 → 加权平均成本；新仓当日 available=0（T+1）
func (l *Ledger) Buy(code string, price float64, qty int, commission float64) error {
	if qty%100 != 0 {
		return ErrNotLotSize
	}
	if cost := price*float64(qty) + commission; cost > l.cash {
		return ErrInsufficientCash
	}
	l.cash -= price*float64(qty) + commission
	if pos, ok := l.positions[code]; ok {
		// 已有持仓：加权平均成本，当日买入部分保持冻结
		totalCost := pos.CostPrice*float64(pos.Quantity) + price*float64(qty)
		pos.Quantity += qty
		pos.CostPrice = Round2(totalCost / float64(pos.Quantity))
	} else {
		l.positions[code] = &PositionState{Code: code, Quantity: qty, AvailableQty: 0,
			CostPrice: Round2(price), CurrentPrice: Round2(price)}
	}
	return nil
}

// Sell 卖出：无持仓 / 超可用（T+1）校验，净回款入现金
func (l *Ledger) Sell(code string, price float64, qty int, commission, tax float64) error {
	pos, ok := l.positions[code]
	if !ok {
		return ErrNoPosition
	}
	if qty > pos.AvailableQty {
		return ErrT1Unavailable
	}
	l.cash += price*float64(qty) - commission - tax
	pos.Quantity -= qty
	pos.AvailableQty -= qty
	if pos.Quantity == 0 {
		delete(l.positions, code)
	}
	return nil
}

// UnfreezeT1 解冻：全部持仓 available = quantity（引擎每交易日开始调用）
func (l *Ledger) UnfreezeT1() {
	for _, p := range l.positions {
		p.AvailableQty = p.Quantity
	}
}

func (l *Ledger) MarketValue() float64 {
	var mv float64
	for _, p := range l.positions {
		mv += p.MarketValue()
	}
	return mv
}

func (l *Ledger) TotalAsset() float64 { return l.cash + l.MarketValue() }

// SizeBuyQty 等权 + 单股仓位上限，向下取整到 100 股整数倍（镜像 engine._calc_quantity）
func (l *Ledger) SizeBuyQty(price float64, topN int, maxPositionPct float64) int {
	total := l.TotalAsset()
	budget := total / float64(topN)
	if b := total * maxPositionPct; b < budget {
		budget = b
	}
	return int(budget/price/100) * 100
}

// TradingService 交易服务：自动执行（ExecuteDay）+ 手动委托
type TradingService struct {
	db      *gorm.DB
	broker  *Broker
	initial float64 // 初始资金（净值基准）

	accountRepo *repository.AccountRepository
	positionRepo *repository.PositionRepository
	orderRepo    *repository.OrderRepository
	tradeRepo    *repository.TradeRepository
	navRepo      *repository.AccountNavRepository
	dailyRepo    *repository.DailyRepository
	signalRepo   *repository.SignalRepository
	stockRepo    *repository.StockRepository
}

func NewTradingService(db *gorm.DB, account config.AccountConfig) *TradingService {
	return &TradingService{
		db:           db,
		broker:       NewBroker(account),
		initial:      account.InitialCash,
		accountRepo:  repository.NewAccountRepository(db),
		positionRepo: repository.NewPositionRepository(db),
		orderRepo:    repository.NewOrderRepository(db),
		tradeRepo:    repository.NewTradeRepository(db),
		navRepo:      repository.NewAccountNavRepository(db),
		dailyRepo:    repository.NewDailyRepository(db),
		signalRepo:   repository.NewSignalRepository(db),
		stockRepo:    repository.NewStockRepository(db),
	}
}

// ExecResult ExecuteDay 执行结果
type ExecResult struct {
	TradeDate time.Time
	Skipped   bool   // 该日已执行（幂等跳过）
	BuyCount  int    // 策略买入成交数
	SellCount int    // 策略卖出成交数
	Manual    int    // 手动单成交数
	Rejected  int    // 拒绝单数（涨停/跌停/资金不足/未达价等）
}

// ExecuteDay 日终自动执行（单事务）：
// 1. 行级锁账户 → 幂等闸（净值已存在 → 跳过）
// 2. 解冻 T+1 → 持仓按当日收盘 mark-to-market（停牌保留旧值）
// 3. 策略信号：SELL（按可用量全卖）→ BUY（按 score 降序，资金逐笔演化）
// 4. 手动 PENDING 单按收盘价撮合（BUY 委托价≥收盘 / SELL≤收盘，否则 REJECTED）
// 5. 更新账户 cash/market_value/total_asset/profit/profit_rate
func (s *TradingService) ExecuteDay(accountID uint64, tradeDate time.Time) (*ExecResult, error) {
	res := &ExecResult{TradeDate: tradeDate}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		ar := repository.NewAccountRepository(tx)
		pr := repository.NewPositionRepository(tx)
		or := repository.NewOrderRepository(tx)
		tr := repository.NewTradeRepository(tx)
		dr := repository.NewDailyRepository(tx)
		sr := repository.NewSignalRepository(tx)

		acc, err := ar.LockByID(accountID)
		if err != nil {
			return err
		}

		// 幂等闸：该日净值已快照 = 该日已完整执行
		exists, err := repository.NewAccountNavRepository(tx).Exists(accountID, tradeDate)
		if err != nil {
			return err
		}
		if exists {
			res.Skipped = true
			return nil
		}

		// 装载账本 + 解冻 T+1
		ledger := NewLedger(s.initial, acc.Cash)
		positions, err := pr.ListByAccount(accountID)
		if err != nil {
			return err
		}
		for i := range positions {
			p := &positions[i]
			ledger.positions[p.Code] = &PositionState{
				Code: p.Code, Quantity: p.Quantity, AvailableQty: p.AvailableQty,
				CostPrice: p.CostPrice, CurrentPrice: p.CurrentPrice,
			}
		}
		ledger.UnfreezeT1()

		// mark-to-market：停牌（无当日 bar）保留旧价
		for code, st := range ledger.positions {
			bar, err := dr.GetByDate(code, tradeDate)
			if err != nil {
				return err
			}
			if bar != nil {
				st.CurrentPrice = bar.Close
			}
			if err := pr.Upsert(s.positionModel(accountID, st)); err != nil {
				return err
			}
		}

		// 策略信号（幂等：该日已有策略单则跳过，只处理手动单）
		strategyDone, err := or.HasStrategyOrderOn(accountID, tradeDate)
		if err != nil {
			return err
		}
		if !strategyDone {
			if err := s.execStrategy(tx, ledger, acc, res, tradeDate, dr, sr, or, tr, pr); err != nil {
				return err
			}
		}

		// 手动 PENDING 单撮合
		if err := s.execManual(tx, ledger, acc, res, tradeDate, dr, or, tr, pr); err != nil {
			return err
		}

		// 更新账户
		acc.Cash = Round2(ledger.cash)
		acc.MarketValue = Round2(ledger.MarketValue())
		acc.TotalAsset = Round2(acc.Cash + acc.MarketValue)
		acc.Profit = Round2(acc.TotalAsset - s.initial)
		acc.ProfitRate = Round4((acc.TotalAsset - s.initial) / s.initial)
		return ar.Update(acc)
	})
	return res, err
}

// execStrategy 策略信号执行（SELL 先于 BUY，释放现金）
func (s *TradingService) execStrategy(tx *gorm.DB, ledger *Ledger, acc *model.Account,
	res *ExecResult, date time.Time, dr *repository.DailyRepository,
	sr *repository.SignalRepository, or *repository.OrderRepository,
	tr *repository.TradeRepository, pr *repository.PositionRepository) error {

	strategy, err := sr.GetStrategy("multi_factor")
	if err != nil {
		return err
	}
	topN, maxPct := 20, 0.20
	if strategy != nil {
		p := struct {
			TopN           int     `json:"top_n"`
			MaxPositionPct float64 `json:"max_position_pct"`
		}{}
		if err := json.Unmarshal(strategy.Params, &p); err == nil {
			if p.TopN > 0 {
				topN = p.TopN
			}
			if p.MaxPositionPct > 0 {
				maxPct = p.MaxPositionPct
			}
		}
	}

	exec := func(action string) error {
		items, err := sr.GetSignals("multi_factor", date, action, 500)
		if err != nil {
			return err
		}
		for _, sg := range items {
			rejected, err := s.fillStrategy(tx, ledger, acc, sg, date, topN, maxPct, dr, or, tr, pr)
			if err != nil {
				return err
			}
			if rejected != "" {
				res.Rejected++
			}
		}
		return nil
	}
	if err := exec(model.ActionSell); err != nil {
		return err
	}
	return exec(model.ActionBuy)
}

// fillStrategy 单笔策略信号撮合；rejected 非空表示拒单原因
func (s *TradingService) fillStrategy(tx *gorm.DB, ledger *Ledger, acc *model.Account,
	sg repository.SignalItem, date time.Time, topN int, maxPct float64,
	dr *repository.DailyRepository, or *repository.OrderRepository,
	tr *repository.TradeRepository, pr *repository.PositionRepository) (string, error) {

	closePrice, err := s.barClose(dr, sg.Code, date)
	if err != nil {
		return "", err
	}
	if closePrice == nil { // 停牌/无数据：跳过
		return "", nil
	}

	if sg.Action == model.ActionSell {
		pos, ok := ledger.positions[sg.Code]
		if !ok || pos.AvailableQty <= 0 { // 无持仓 / T+1 当日买入：跳过
			return "", nil
		}
		prevClose, _, err := dr.GetPrevClose(sg.Code, date)
		if err != nil {
			return "", err
		}
		if !s.broker.CheckPriceLimit(sg.Code, *closePrice, prevClose, model.ActionSell) {
			if err := or.Create(s.rejectedOrder(acc.ID, sg.Code, model.ActionSell,
				*closePrice, pos.AvailableQty, "跌停无法成交", sg.Reason, date)); err != nil {
				return "", err
			}
			return "跌停无法成交", nil
		}
		execPrice := s.broker.SellExecPrice(*closePrice)
		qty := pos.AvailableQty
		amount := Round2(execPrice * float64(qty))
		commission := Round2(s.broker.Commission(amount))
		tax := Round2(s.broker.Tax(amount))
		if err := ledger.Sell(sg.Code, execPrice, qty, commission, tax); err != nil {
			return "", err
		}
		if err := s.recordFill(tx, acc.ID, sg.Code, model.ActionSell, execPrice, qty,
			commission, tax, date, "strategy", sg.Reason, or, tr); err != nil {
			return "", err
		}
		// pos 与 ledger 共享指针：清仓后 Quantity==0 → savePosition 删除持仓
		if err := s.savePosition(pr, acc.ID, pos); err != nil {
			return "", err
		}
		return "", nil
	}

	// BUY
	prevClose, _, err := dr.GetPrevClose(sg.Code, date)
	if err != nil {
		return "", err
	}
	if !s.broker.CheckPriceLimit(sg.Code, *closePrice, prevClose, model.ActionBuy) {
		qty := ledger.SizeBuyQty(*closePrice, topN, maxPct)
		if err := or.Create(s.rejectedOrder(acc.ID, sg.Code, model.ActionBuy, *closePrice,
			qty, "涨停无法成交", sg.Reason, date)); err != nil {
			return "", err
		}
		return "涨停无法成交", nil
	}
	execPrice := s.broker.BuyExecPrice(*closePrice)
	qty := ledger.SizeBuyQty(*closePrice, topN, maxPct)
	for qty > 0 { // 资金不足逐档减 100 股
		amount := Round2(execPrice * float64(qty))
		commission := Round2(s.broker.Commission(amount))
		if err := ledger.Buy(sg.Code, execPrice, qty, commission); err == nil {
			if err := s.recordFill(tx, acc.ID, sg.Code, model.ActionBuy, execPrice, qty,
				commission, 0, date, "strategy", sg.Reason, or, tr); err != nil {
				return "", err
			}
			if err := s.savePosition(pr, acc.ID, ledger.positions[sg.Code]); err != nil {
				return "", err
			}
			return "", nil
		}
		qty -= 100
	}
	if err := or.Create(s.rejectedOrder(acc.ID, sg.Code, model.ActionBuy, *closePrice,
		0, "资金不足", sg.Reason, date)); err != nil {
		return "", err
	}
	return "资金不足", nil
}

// execManual 手动 PENDING 单撮合：当日有效，未达价/涨跌停/资金不足 → REJECTED
func (s *TradingService) execManual(tx *gorm.DB, ledger *Ledger, acc *model.Account,
	res *ExecResult, date time.Time, dr *repository.DailyRepository,
	or *repository.OrderRepository, tr *repository.TradeRepository,
	pr *repository.PositionRepository) error {

	pendings, err := or.GetPending(acc.ID)
	if err != nil {
		return err
	}
	for i := range pendings {
		o := &pendings[i]
		reject := func(reason string) error {
			if err := or.UpdateRejected(o.OrderID, reason); err != nil {
				return err
			}
			res.Rejected++
			return nil
		}
		bar, err := dr.GetByDate(o.Code, date)
		if err != nil {
			return err
		}
		if bar == nil {
			return reject("当日无行情（停牌或未上市）")
		}
		closePrice := bar.Close
		if (o.Direction == model.ActionBuy && o.Price < closePrice) ||
			(o.Direction == model.ActionSell && o.Price > closePrice) {
			return reject("未达成交价（当日有效）")
		}
		prevClose, _, err := dr.GetPrevClose(o.Code, date)
		if err != nil {
			return err
		}
		if !s.broker.CheckPriceLimit(o.Code, closePrice, prevClose, o.Direction) {
			if o.Direction == model.ActionBuy {
				return reject("涨停无法成交")
			}
			return reject("跌停无法成交")
		}
		// 成交
		var execPrice float64
		if o.Direction == model.ActionBuy {
			execPrice = s.broker.BuyExecPrice(closePrice)
		} else {
			execPrice = s.broker.SellExecPrice(closePrice)
		}
		amount := Round2(execPrice * float64(o.Quantity))
		commission := Round2(s.broker.Commission(amount))
		var tax float64
		if o.Direction == model.ActionSell {
			tax = Round2(s.broker.Tax(amount))
		}
		var fillErr error
		if o.Direction == model.ActionBuy {
			fillErr = ledger.Buy(o.Code, execPrice, o.Quantity, commission)
		} else {
			fillErr = ledger.Sell(o.Code, execPrice, o.Quantity, commission, tax)
		}
		if errors.Is(fillErr, ErrInsufficientCash) {
			return reject("资金不足")
		}
		if errors.Is(fillErr, ErrT1Unavailable) {
			return reject("可用持仓不足（T+1 限制）")
		}
		if fillErr != nil {
			return fillErr
		}
		if err := s.recordFill(tx, acc.ID, o.Code, o.Direction, execPrice, o.Quantity,
			commission, tax, date, "manual", "", or, tr); err != nil {
			return err
		}
		if err := or.UpdateFilled(o.OrderID, o.Quantity, execPrice); err != nil {
			return err
		}
		pos, ok := ledger.positions[o.Code]
		if ok {
			if err := s.savePosition(pr, acc.ID, pos); err != nil {
				return err
			}
		}
		res.Manual++
	}
	return nil
}

// ---- 手工委托 ----

// PlaceManualOrder 提交手动委托：整手 / 价格 / 涨跌停范围 / 卖出可用量校验
func (s *TradingService) PlaceManualOrder(accountID uint64, code, direction string,
	price float64, qty int) (*model.Order, error) {

	if direction != model.ActionBuy && direction != model.ActionSell {
		return nil, ErrInvalidDirection
	}
	if qty <= 0 || qty%100 != 0 {
		return nil, ErrNotLotSize
	}
	if price <= 0 {
		return nil, ErrInvalidPrice
	}
	exists, err := s.stockRepo.Exists(code)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrStockMissing
	}
	// 涨跌停范围（以上一收盘为基准）
	bar, err := s.dailyRepo.GetLatest(code)
	if err != nil {
		return nil, err
	}
	if bar != nil && bar.Close > 0 {
		ratio := LimitRatioOf(code)
		limitUp := bar.Close * (1 + ratio)
		limitDown := bar.Close * (1 - ratio)
		if (direction == model.ActionBuy && price > limitUp) ||
			(direction == model.ActionSell && price < limitDown) {
			return nil, ErrPriceLimit
		}
	}
	// 卖出可用量（T+1）
	if direction == model.ActionSell {
		pos, err := s.positionRepo.Get(accountID, code)
		if err != nil {
			return nil, err
		}
		if pos == nil || pos.AvailableQty < qty {
			return nil, ErrT1Unavailable
		}
	}
	o := &model.Order{
		OrderID:   uuid.NewString(),
		AccountID: accountID,
		Code:      code,
		Direction: direction,
		OrderType: "LIMIT",
		Price:     Round2(price),
		Quantity:  qty,
		Status:    model.OrderPending,
		Source:    "manual",
	}
	if err := s.orderRepo.Create(o); err != nil {
		return nil, err
	}
	return o, nil
}

// CancelOrder 撤单：仅 PENDING 可撤
func (s *TradingService) CancelOrder(accountID uint64, orderID string) error {
	o, err := s.orderRepo.GetByOrderID(orderID)
	if err != nil {
		return err
	}
	if o == nil || o.AccountID != accountID {
		return ErrOrderNotFound
	}
	if o.Status != model.OrderPending {
		return ErrNotCancellable
	}
	ok, err := s.orderRepo.Cancel(orderID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotCancellable
	}
	return nil
}

// ---- 内部工具 ----

// barClose 当日收盘价；停牌/无数据返回 (nil, nil)
func (s *TradingService) barClose(dr *repository.DailyRepository, code string,
	date time.Time) (*float64, error) {
	bar, err := dr.GetByDate(code, date)
	if err != nil || bar == nil {
		return nil, err
	}
	c := bar.Close
	return &c, nil
}

// recordFill 写成交记录（order 已含成交信息时由调用方更新）
func (s *TradingService) recordFill(tx *gorm.DB, accountID uint64, code, direction string,
	price float64, qty int, commission, tax float64, date time.Time, source, reason string,
	or *repository.OrderRepository, tr *repository.TradeRepository) error {

	amount := Round2(price * float64(qty))
	order := &model.Order{
		OrderID:      uuid.NewString(),
		AccountID:    accountID,
		Code:         code,
		Direction:    direction,
		OrderType:    "MARKET",
		Price:        Round2(price),
		Quantity:     qty,
		FilledQty:    qty,
		AvgFillPrice: Round2(price),
		Status:       model.OrderFilled,
		Reason:       reason,
		Source:       source,
	}
	if err := or.Create(order); err != nil {
		return err
	}
	trade := &model.Trade{
		TradeID:   uuid.NewString(),
		OrderID:   order.OrderID,
		AccountID: accountID,
		Code:      code,
		Direction: direction,
		Price:     Round2(price),
		Quantity:  qty,
		Amount:    amount,
		Commission: Round2(commission),
		Tax:       Round2(tax),
		TradeDate: date,
	}
	if direction == model.ActionBuy {
		trade.NetAmount = Round2(amount + commission)
	} else {
		trade.NetAmount = Round2(amount - commission - tax)
	}
	return tr.Create(trade)
}

// rejectedOrder 拒单（记录原因，不产生成交）
func (s *TradingService) rejectedOrder(accountID uint64, code, direction string,
	price float64, qty int, rejectReason, signalReason string, date time.Time) *model.Order {
	reason := rejectReason
	if signalReason != "" {
		reason = rejectReason + "（" + signalReason + "）"
	}
	return &model.Order{
		OrderID:   uuid.NewString(),
		AccountID: accountID,
		Code:      code,
		Direction: direction,
		OrderType: "MARKET",
		Price:     Round2(price),
		Quantity:  qty,
		Status:    model.OrderRejected,
		Reason:    reason,
		Source:    "strategy",
		CreatedAt: date,
	}
}

// savePosition 持仓落库：清仓（Quantity==0）→ 删除，否则 upsert；nil 表示无该持仓，跳过
func (s *TradingService) savePosition(pr *repository.PositionRepository, accountID uint64,
	st *PositionState) error {
	if st == nil {
		return nil
	}
	if st.Quantity == 0 {
		return pr.Delete(accountID, st.Code)
	}
	return pr.Upsert(s.positionModel(accountID, st))
}

// positionModel PositionState → 持久化模型
func (s *TradingService) positionModel(accountID uint64, st *PositionState) *model.Position {
	mv := Round2(st.MarketValue())
	profit := Round2(st.Profit())
	rate := 0.0
	if st.CostPrice > 0 {
		rate = Round4(st.CurrentPrice/st.CostPrice - 1)
	}
	return &model.Position{
		AccountID:    accountID,
		Code:         st.Code,
		Quantity:     st.Quantity,
		AvailableQty: st.AvailableQty,
		CostPrice:    Round2(st.CostPrice),
		CurrentPrice: Round2(st.CurrentPrice),
		MarketValue:  mv,
		Profit:       profit,
		ProfitRate:   rate,
	}
}

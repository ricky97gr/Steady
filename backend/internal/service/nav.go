package service

import (
	"time"

	"gorm.io/gorm"

	"quant-system/backend/internal/config"
	"quant-system/backend/internal/model"
	"quant-system/backend/internal/repository"
)

// NavService 净值快照：nav = 总资产/初始资金，日收益对照前一日，回撤对照历史峰值
type NavService struct {
	db     *gorm.DB
	initial float64

	accountRepo *repository.AccountRepository
	navRepo     *repository.AccountNavRepository
	positionRepo *repository.PositionRepository
}

func NewNavService(db *gorm.DB, account config.AccountConfig) *NavService {
	return &NavService{
		db:           db,
		initial:      account.InitialCash,
		accountRepo:  repository.NewAccountRepository(db),
		navRepo:      repository.NewAccountNavRepository(db),
		positionRepo: repository.NewPositionRepository(db),
	}
}

// NavResult SnapshotDay 执行结果
type NavResult struct {
	Skipped bool
	Nav     float64
}

// SnapshotDay 生成某交易日净值快照（单事务，幂等：已存在则跳过）
func (s *NavService) SnapshotDay(accountID uint64, tradeDate time.Time) (*NavResult, error) {
	res := &NavResult{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		ar := repository.NewAccountRepository(tx)
		nr := repository.NewAccountNavRepository(tx)
		pr := repository.NewPositionRepository(tx)

		acc, err := ar.LockByID(accountID)
		if err != nil {
			return err
		}
		exists, err := nr.Exists(accountID, tradeDate)
		if err != nil {
			return err
		}
		if exists {
			res.Skipped = true
			return nil
		}

		// 持仓市值（ExecuteDay 已完成 mark-to-market）
		positions, err := pr.ListByAccount(accountID)
		if err != nil {
			return err
		}
		var mv float64
		for _, p := range positions {
			mv += p.MarketValue
		}
		total := Round2(acc.Cash + mv)
		nav := Round6(total / s.initial)

		// 日收益：对照上一净值
		dailyReturn := 0.0
		prev, err := nr.GetLastBefore(accountID, tradeDate)
		if err != nil {
			return err
		}
		if prev != nil {
			dailyReturn = Round4(nav/prev.Nav - 1)
		}

		// 回撤：对照历史峰值
		hist, err := nr.GetRange(accountID, nil, &tradeDate)
		if err != nil {
			return err
		}
		peak := nav
		for _, h := range hist {
			if h.Nav > peak {
				peak = h.Nav
			}
		}
		drawdown := Round4(nav/peak - 1)
		if drawdown > 0 {
			drawdown = 0
		}

		if err := nr.Upsert(&model.AccountNav{
			AccountID:   accountID,
			TradeDate:   tradeDate,
			TotalAsset:  total,
			Cash:        acc.Cash,
			MarketValue: Round2(mv),
			Nav:         nav,
			DailyReturn: dailyReturn,
			Drawdown:    drawdown,
		}); err != nil {
			return err
		}

		// 账户最大回撤（取更负值）
		if drawdown < acc.MaxDrawdown {
			acc.MaxDrawdown = drawdown
			if err := ar.Update(acc); err != nil {
				return err
			}
		}
		res.Nav = nav
		return nil
	})
	return res, err
}

// GetRange 区间净值序列（处理为前端展示 DTO 由 handler 完成）
func (s *NavService) GetRange(accountID uint64, start, end *time.Time) ([]model.AccountNav, error) {
	return s.navRepo.GetRange(accountID, start, end)
}

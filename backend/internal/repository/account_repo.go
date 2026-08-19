package repository

import (
	"gorm.io/gorm"

	"quant-system/backend/internal/model"
)

// AccountRepository 账户数据访问层
type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

// GetByID 查询账户
func (r *AccountRepository) GetByID(id uint64) (*model.Account, error) {
	var acc model.Account
	err := r.db.First(&acc, id).Error
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetPrimary 查询主账户（V1 单账户，取 id 最小者）
func (r *AccountRepository) GetPrimary() (*model.Account, error) {
	var acc model.Account
	err := r.db.Order("id").First(&acc).Error
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

package solmodel

import (
	"context"

	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// avoid unused err
var _ = InitField
var _ MarketKlineTradeProcessModel = (*customMarketKlineTradeProcessModel)(nil)

type (
	// MarketKlineTradeProcessModel is an interface to be customized, add more methods here,
	// and implement the added methods in customMarketKlineTradeProcessModel.
	MarketKlineTradeProcessModel interface {
		marketKlineTradeProcessModel
		customMarketKlineTradeProcessLogicModel
	}

	customMarketKlineTradeProcessLogicModel interface {
		WithSession(tx *gorm.DB) MarketKlineTradeProcessModel
		InsertIgnore(ctx context.Context, data *MarketKlineTradeProcess) (bool, error)
	}

	customMarketKlineTradeProcessModel struct {
		*defaultMarketKlineTradeProcessModel
	}
)

func (c customMarketKlineTradeProcessModel) WithSession(tx *gorm.DB) MarketKlineTradeProcessModel {
	newModel := *c.defaultMarketKlineTradeProcessModel
	c.defaultMarketKlineTradeProcessModel = &newModel
	c.conn = tx
	return &c
}

// NewMarketKlineTradeProcessModel returns a model for the database table.
func NewMarketKlineTradeProcessModel(conn *gorm.DB) MarketKlineTradeProcessModel {
	return &customMarketKlineTradeProcessModel{
		defaultMarketKlineTradeProcessModel: newMarketKlineTradeProcessModel(conn),
	}
}

func (m *defaultMarketKlineTradeProcessModel) customCacheKeys(data *MarketKlineTradeProcess) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

func (m *customMarketKlineTradeProcessModel) InsertIgnore(ctx context.Context, data *MarketKlineTradeProcess) (bool, error) {
	if data == nil {
		return false, nil
	}

	//做
	res := m.conn.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "hash_id"}, {Name: "interval_val"}},
		DoNothing: true,
	}).Create(data)
	if res.Error != nil {
		return false, res.Error
	}

	return res.RowsAffected > 0, nil
}

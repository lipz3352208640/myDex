package solmodel

import (
	"context"

	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ MarketKlineModel = (*customMarketKlineModel)(nil)

type (
	// MarketKlineModel is an interface to be customized, add more methods here,
	// and implement the added methods in customMarketKlineModel.
	MarketKlineModel interface {
		marketKlineModel
		customMarketKlineLogicModel
	}

	customMarketKlineLogicModel interface {
		WithSession(tx *gorm.DB) MarketKlineModel
		FindOneByUniqueKey(ctx context.Context, chainID int64, pairAddr, interval string, candleTime int64) (*MarketKline, error)
		UpdateByID(ctx context.Context, id int64, updates map[string]any) error
	}

	customMarketKlineModel struct {
		*defaultMarketKlineModel
	}
)

func (c customMarketKlineModel) WithSession(tx *gorm.DB) MarketKlineModel {
	newModel := *c.defaultMarketKlineModel
	c.defaultMarketKlineModel = &newModel
	c.conn = tx
	return &c
}

// NewMarketKlineModel returns a model for the database table.
func NewMarketKlineModel(conn *gorm.DB) MarketKlineModel {
	return &customMarketKlineModel{
		defaultMarketKlineModel: newMarketKlineModel(conn),
	}
}

func (m *defaultMarketKlineModel) customCacheKeys(data *MarketKline) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

func (m *customMarketKlineModel) FindOneByUniqueKey(ctx context.Context, chainID int64, pairAddr, interval string, candleTime int64) (*MarketKline, error) {
	return m.FindOneByChainIdPairAddrIntervalValCandleTime(ctx, chainID, pairAddr, interval, candleTime)
}

func (m *customMarketKlineModel) UpdateByID(ctx context.Context, id int64, updates map[string]any) error {
	return m.conn.WithContext(ctx).Model(&MarketKline{}).Where("id = ?", id).Updates(updates).Error
}

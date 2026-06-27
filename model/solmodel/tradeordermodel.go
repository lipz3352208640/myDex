package solmodel

import (
	"context"

	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ TradeOrderModel = (*customTradeOrderModel)(nil)

const (
	tradeOrderStatusWait int64 = 1
	tradeOrderStatusProc int64 = 2
)

type (
	// TradeOrderModel is an interface to be customized, add more methods here,
	// and implement the added methods in customTradeOrderModel.
	TradeOrderModel interface {
		tradeOrderModel
		customTradeOrderLogicModel
	}

	customTradeOrderLogicModel interface {
		WithSession(tx *gorm.DB) TradeOrderModel
		InsertWithLog(ctx context.Context, data *TradeOrder) error
		UpdateOrder(ctx context.Context, order *TradeOrder, updateField []string) error
		ClaimWaitingOrder(ctx context.Context, order *TradeOrder) (bool, error)
		FindOnChainOrderByChainId(ctx context.Context, chainId int64, limit int, offset int) ([]*TradeOrder, error)
	}

	customTradeOrderModel struct {
		*defaultTradeOrderModel
	}
)

func (c customTradeOrderModel) FindOnChainOrderByChainId(ctx context.Context, chainId int64, limit int, offset int) ([]*TradeOrder, error) {
	var tradeOrders []*TradeOrder
	err := c.conn.WithContext(ctx).Model(&TradeOrder{}).Where("chain_id = ? and status = ?", chainId, 3).Limit(limit).Offset(offset).Order("id asc").Find(&tradeOrders).Error
	return tradeOrders, err
}

func (c customTradeOrderModel) InsertWithLog(ctx context.Context, data *TradeOrder) error {
	return c.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Save(&data).Error
		if err != nil {
			return err
		}
		ctx = context.Background()
		return NewTradeOrderLogModel(tx).InsertWithOrder(ctx, data)
	})
}

func (c customTradeOrderModel) UpdateOrder(ctx context.Context, data *TradeOrder, updateField []string) error {
	return c.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&data).Where("id = ?", data.Id).Select(updateField).Updates(&data).Error
		if err != nil {
			return err
		}
		return NewTradeOrderLogModel(tx).InsertWithOrder(ctx, data)
	})
}

func (c customTradeOrderModel) ClaimWaitingOrder(ctx context.Context, data *TradeOrder) (bool, error) {
	claimed := false
	err := c.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&TradeOrder{}).
			Where("id = ? AND status = ?", data.Id, tradeOrderStatusWait).
			Update("status", tradeOrderStatusProc)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}

		claimed = true
		data.Status = tradeOrderStatusProc
		return NewTradeOrderLogModel(tx).InsertWithOrder(context.Background(), data)
	})
	return claimed, err
}

func (c customTradeOrderModel) WithSession(tx *gorm.DB) TradeOrderModel {
	newModel := *c.defaultTradeOrderModel
	c.defaultTradeOrderModel = &newModel
	c.conn = tx
	return c
}

// NewTradeOrderModel returns a model for the database table.
func NewTradeOrderModel(conn *gorm.DB) TradeOrderModel {
	return &customTradeOrderModel{
		defaultTradeOrderModel: newTradeOrderModel(conn),
	}
}

func (m *defaultTradeOrderModel) customCacheKeys(data *TradeOrder) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

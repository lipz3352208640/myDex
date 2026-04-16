package solmodel

import (
	"context"
	"strings"

	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ TradeModel = (*customTradeModel)(nil)

type (
	// TradeModel is an interface to be customized, add more methods here,
	// and implement the added methods in customTradeModel.
	TradeModel interface {
		tradeModel
		customTradeLogicModel
	}

	customTradeLogicModel interface {
		WithSession(tx *gorm.DB) TradeModel
		FindByTxHashAndCreateTimes(ctx context.Context, items []*Trade) ([]*Trade, error)
	}

	customTradeModel struct {
		*defaultTradeModel
	}
)

func (c customTradeModel) WithSession(tx *gorm.DB) TradeModel {
	newModel := *c.defaultTradeModel
	c.defaultTradeModel = &newModel
	c.conn = tx
	return c
}

// NewTradeModel returns a model for the database table.
func NewTradeModel(conn *gorm.DB) TradeModel {
	return &customTradeModel{
		defaultTradeModel: newTradeModel(conn),
	}
}

func (m *defaultTradeModel) customCacheKeys(data *Trade) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

func (m *defaultTradeModel) FindByTxHashAndCreateTimes(ctx context.Context, items []*Trade) ([]*Trade, error) {
	if len(items) == 0 {
		return []*Trade{}, nil
	}

	conditions := make([]string, 0, len(items))
	args := make([]any, 0, len(items)*2)
	for _, item := range items {
		if item == nil || item.TxHash == "" || item.CreatedAt.IsZero() {
			continue
		}
		conditions = append(conditions, "(tx_hash = ? AND created_at >= ?)")
		args = append(args, item.TxHash, item.CreatedAt)
	}

	if len(conditions) == 0 {
		return []*Trade{}, nil
	}

	var trades []*Trade
	err := m.conn.WithContext(ctx).
		Model(&Trade{}).
		Where(strings.Join(conditions, " OR "), args...).
		Order("id asc").
		Find(&trades).Error
	if err != nil {
		return nil, err
	}

	return trades, nil
}

package solmodel

import (
	"context"
	"database/sql"
	"myDex/pkg/xcode"
	"time"

	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ PairModel = (*customPairModel)(nil)

type (
	// PairModel is an interface to be customized, add more methods here,
	// and implement the added methods in customPairModel.
	PairModel interface {
		pairModel
		customPairLogicModel
	}

	customPairLogicModel interface {
		WithSession(tx *gorm.DB) PairModel
		FindMaxSupplyPairInfoByTokenAddrAndChainID(ctx context.Context, chainId int64, tokenAddr string) (*Pair, error)
		FindNearTokenPriceByChainIdAndTokenAddr(ctx context.Context, chainId int64, tokenAddr string, baseTokenAddr string, searchTime time.Time) (*Pair, error)
		FindNearBaseTokenPriceByChainIdAndTokenAddr(ctx context.Context, chainId int64, baseTokenAddr string, searchTime time.Time) (*Pair, error)
	}

	customPairModel struct {
		*defaultPairModel
	}
)

func (c customPairModel) FindNearBaseTokenPriceByChainIdAndTokenAddr(ctx context.Context,
	chainId int64,
	baseTokenAddr string,
	searchTime time.Time) (*Pair, error) {
	var resp Pair
	err := c.conn.WithContext(ctx).Model(&Pair{}).Where("`chain_id` = ? and `base_token_address` = ? and `block_time` > ? and base_token_price > 0", chainId, baseTokenAddr, searchTime).Order("block_time ASC").First(&resp).Error

	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c customPairModel) FindNearTokenPriceByChainIdAndTokenAddr(ctx context.Context,
	chainId int64,
	tokenAddr string,
	baseTokenAddr string,
	searchTime time.Time) (*Pair, error) {
	var resp Pair
	err := c.conn.WithContext(ctx).
		Model(&Pair{}).
		Where("`chain_id` = ? and `base_token_address` = ? and `token_address` = ?  and `block_time` > ? and token_price > 0 ",
			chainId,
			baseTokenAddr,
			tokenAddr,
			searchTime).
		Order("block_time ASC").
		First(&resp).Error

	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c customPairModel) FindMaxSupplyPairInfoByTokenAddrAndChainID(ctx context.Context, chainId int64, tokenAddr string) (*Pair, error) {
	var resp []Pair
	err := c.conn.WithContext(ctx).
		Model(&Pair{}).
		Where("`chain_id` = @chainId and `token_address` = @tokenAddr",
			sql.Named("chainId", chainId),
			sql.Named("tokenAddr", tokenAddr)).Find(&resp).Error

	if err != nil {
		return nil, err
	}

	if len(resp) == 0 {
		return nil, xcode.NotingFoundError
	}

	maxPair := resp[0]
	for i := 1; i < len(resp); i++ {
		if resp[i].Liquidity > maxPair.Liquidity {
			maxPair = resp[i]
		}
	}

	return &maxPair, nil
}

func (c customPairModel) WithSession(tx *gorm.DB) PairModel {
	newModel := *c.defaultPairModel
	c.defaultPairModel = &newModel
	c.conn = tx
	return c
}

// NewPairModel returns a model for the database table.
func NewPairModel(conn *gorm.DB) PairModel {
	return &customPairModel{
		defaultPairModel: newPairModel(conn),
	}
}

func (m *defaultPairModel) customCacheKeys(data *Pair) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

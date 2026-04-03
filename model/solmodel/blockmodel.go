package solmodel

import (
	"context"

	"myDex/pkg/constant"

	"github.com/klen-ygs/gorm-zero/gormc"
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField

type (
	// BlockModel is an interface to be customized, add more methods here,
	// and implement the added methods in customBlockModel.
	BlockModel interface {
		blockModel
		customBlockLogicModel
	}

	customBlockLogicModel interface {
		WithSession(tx *gorm.DB) BlockModel
		GetFirstFailedSlot(ctx context.Context) (*Block, error)
		GetBatchFailedBlockBySlot(ctx context.Context, slot int64, limit int) ([]*Block, error)
		FindOneNearSlot(ctx context.Context, slot int64) (*Block, error)
	}

	customBlockModel struct {
		*defaultBlockModel
	}
)

func (c customBlockModel) GetBatchFailedBlockBySlot(context context.Context, slot int64, limit int) ([]*Block, error) {
	var resp []*Block
	err := c.conn.WithContext(context).Model(&Block{}).Where("status = ?", constant.BlockFailed).
		Where("slot > ?", slot).
		Order("slot Desc").
		Limit(limit).Find(&resp).Error
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c customBlockModel) FindOneNearSlot(context context.Context, slot int64) (*Block, error) {
	var resp *Block
	err := c.conn.WithContext(context).Model(&Block{}).Where("status = ?", constant.BlockProcessed).
		Where("slot < ?", slot).
		Order("slot Desc").
		First(&resp).Error
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c customBlockModel) Insert(ctx context.Context, data *Block) error {
	return c.conn.WithContext(ctx).Create(data).Error
}

func (c customBlockModel) WithSession(tx *gorm.DB) BlockModel {
	newModel := *c.defaultBlockModel
	c.defaultBlockModel = &newModel
	c.conn = tx
	return c
}

func (c customBlockModel) GetFirstFailedSlot(ctx context.Context) (*Block, error) {
	var resp Block
	err := c.conn.WithContext(ctx).Model(&Block{}).Where("status = ?", constant.BlockFailed).First(&resp).Error
	if err == gormc.ErrNotFound {
		return nil, err
	}
	return &resp, nil
}

// NewBlockModel returns a model for the database table.
func NewBlockModel(conn *gorm.DB) BlockModel {
	return &customBlockModel{
		defaultBlockModel: newBlockModel(conn),
	}
}

func (m *defaultBlockModel) customCacheKeys(data *Block) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

package solmodel

import (
	"context"
	"fmt"
	"myDex/pkg/xcode"
	"sort"
	"strings"
	"sync"
	"time"

	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"github.com/samber/lo"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ TradeModel = (*customTradeModel)(nil)

var tableLock sync.Mutex
var tradeLock sync.Mutex

const (
	TradeTableName = "trade"
)

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
		ListByPairAddrAndTotalUsd(ctx context.Context, req *TradeCursorPageReq) (*TradeCursorPageResp, error)
		SaveBatchTrades(ctx context.Context, items []*Trade) error
	}

	customTradeModel struct {
		*defaultTradeModel
	}

	TradeCursorPageReq struct {
		PairAddr    string
		MinTotalUsd *float64
		MaxTotalUsd *float64
		StartTime   time.Time
		EndTime     time.Time
		Cursor      *TradePageCursor
		Limit       int
	}

	TradePageCursor struct {
		BlockTime time.Time
		ID        int64
	}

	TradeCursorPageResp struct {
		Items      []*Trade
		NextCursor *TradePageCursor
		HasMore    bool
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
func (m *defaultTradeModel) SaveBatchTrades(ctx context.Context, items []*Trade) error {

	//将Trade按照day分组
	shrdTableMap, err := makeShardTable(items)
	if err != nil {
		return err
	}

	for shardTableName, items := range shrdTableMap {
		if err := m.createShardTableIfNotExistsAndInsert(ctx, shardTableName, items); err != nil {
			return err
		}
	}

	return nil
}

// createShardTableIfNotExistsAndInsert 自动创建表（如果不存在) 多协程创建，通过双重检验锁
func (m *defaultTradeModel) createShardTableIfNotExistsAndInsert(ctx context.Context, shardTableName string, items []*Trade) error {
	err := m.createShardTableIfNotExists(ctx, shardTableName)
	if err != nil {
		//err := m.createShardTableIfNotExists(ctx, shardTableName)
		return fmt.Errorf("createShardTableIfNotExists error: %w, shardTableName: %v", err, shardTableName)
	}

	//批量插入，每次都是1024条
	err = m.conn.WithContext(ctx).Table(shardTableName).CreateInBatches(items, 1024).Error
	//发生死锁，加全局锁进行重试
	if err != nil && strings.Contains(err.Error(), "Deadlock found when trying to get lock") {
		tradeLock.Lock()
		defer tradeLock.Unlock()
		err = m.conn.WithContext(ctx).Table(shardTableName).CreateInBatches(items, 1024).Error
		if err != nil {
			return fmt.Errorf("createShardTableIfNotExistsAndInsert error: %w, shardTableName: %v", err, shardTableName)
		}
	}
	return nil
}

func (m *defaultTradeModel) createShardTableIfNotExists(ctx context.Context, shardTableName string) error {
	isExists := m.conn.Migrator().HasTable(shardTableName)
	if !isExists {
		tableLock.Lock()
		defer tableLock.Unlock()
		isExists = m.conn.Migrator().HasTable(shardTableName)
		if !isExists {

			if err := m.conn.Table(shardTableName).Migrator().CreateTable(&Trade{}); err != nil {
				return err
			}
			logx.Infof("create ShardTable if not exists success: %v", shardTableName)
		}
	}
	return nil
}

func makeShardTable(items []*Trade) (map[string][]*Trade, error) {
	if len(items) == 0 {
		return nil, xcode.RequestErr
	}
	shardMap := make(map[string][]*Trade)
	for _, item := range items {
		if item == nil || item.BlockTime.IsZero() {
			continue
		}
		shardTableName := getShradTableByTime(item.BlockTime)
		shardMap[shardTableName] = append(shardMap[shardTableName], item)
	}
	return shardMap, nil
}

func getShradTableByTime(blockTime time.Time) string {
	return fmt.Sprintf("%s_%d_%02d_%02d", TradeTableName, blockTime.Year(), blockTime.Month(), blockTime.Day())
}

func listTradeShardTablesByRange(startTime, endTime time.Time) []string {
	start := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	end := time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 0, 0, 0, 0, endTime.Location())

	tables := make([]string, 0)
	for current := end; !current.Before(start); current = current.AddDate(0, 0, -1) {
		tables = append(tables, getShradTableByTime(current))
	}
	return tables
}

func normalizeTradePageLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func (m *defaultTradeModel) ListByPairAddrAndTotalUsd(ctx context.Context, req *TradeCursorPageReq) (*TradeCursorPageResp, error) {
	if req == nil || req.PairAddr == "" || req.StartTime.IsZero() || req.EndTime.IsZero() {
		return nil, xcode.RequestErr
	}
	if req.EndTime.Before(req.StartTime) {
		return nil, xcode.RequestErr
	}
	if req.MinTotalUsd != nil && req.MaxTotalUsd != nil && *req.MinTotalUsd > *req.MaxTotalUsd {
		return nil, xcode.RequestErr
	}

	limit := normalizeTradePageLimit(req.Limit)
	shardTables := listTradeShardTablesByRange(req.StartTime, req.EndTime)

	var tradeRs []*Trade
	seenTrades := make(map[string]struct{})
	for _, shardTableName := range shardTables {
		if len(tradeRs) > limit {
			break
		}
		if !m.conn.WithContext(ctx).Migrator().HasTable(shardTableName) {
			continue
		}

		query := m.conn.WithContext(ctx).
			Table(shardTableName).
			Model(&Trade{}).
			Where("pair_addr = ?", req.PairAddr).
			Where("block_time >= ? AND block_time <= ?", req.StartTime, req.EndTime)

		if req.MinTotalUsd != nil {
			query = query.Where("total_usd >= ?", *req.MinTotalUsd)
		}
		if req.MaxTotalUsd != nil {
			query = query.Where("total_usd <= ?", *req.MaxTotalUsd)
		}
		if req.Cursor != nil && !req.Cursor.BlockTime.IsZero() {
			//同一笔区块可能有多个trade，那每个trade的block_time是一样的
			query = query.Where("(block_time < ?) OR (block_time = ? AND id < ?)", req.Cursor.BlockTime, req.Cursor.BlockTime, req.Cursor.ID)
		}

		queryLimit := limit + 1 - len(tradeRs)
		if queryLimit <= 0 {
			break
		}

		var trades []*Trade
		err := query.
			//先通过block_time倒序，当block_time相同时再通过id倒序
			Order("block_time desc, id desc").
			Limit(queryLimit).
			Find(&trades).Error
		if err != nil {
			return nil, err
		}

		for _, trade := range trades {
			if trade == nil {
				continue
			}
			tradeKey := "tx:" + trade.TxHash
			if trade.Id != 0 {
				tradeKey = fmt.Sprintf("id:%d", trade.Id)
			}
			if _, ok := seenTrades[tradeKey]; ok {
				continue
			}
			seenTrades[tradeKey] = struct{}{}
			tradeRs = append(tradeRs, trade)
			if len(tradeRs) > limit {
				break
			}
		}
	}

	sort.Slice(tradeRs, func(i, j int) bool {
		if tradeRs[i].BlockTime.Equal(tradeRs[j].BlockTime) {
			return tradeRs[i].Id > tradeRs[j].Id
		}
		return tradeRs[i].BlockTime.After(tradeRs[j].BlockTime)
	})

	resp := &TradeCursorPageResp{}
	if len(tradeRs) > limit {
		resp.HasMore = true
		tradeRs = tradeRs[:limit]
	}
	resp.Items = tradeRs
	//返回游标的下一个位置
	if resp.HasMore && len(tradeRs) > 0 {
		lastTrade := tradeRs[len(tradeRs)-1]
		resp.NextCursor = &TradePageCursor{
			BlockTime: lastTrade.BlockTime,
			ID:        lastTrade.Id,
		}
	}

	return resp, nil
}

func (m *defaultTradeModel) FindByTxHashAndCreateTimes(ctx context.Context, items []*Trade) ([]*Trade, error) {
	if len(items) == 0 {
		return []*Trade{}, nil
	}

	// Use order created_at as a fallback shard hint and scan the adjacent days
	// to tolerate small time drifts between order and trade writes.
	shardTableMap := make(map[string][]*Trade)
	for _, item := range items {
		if item == nil || item.TxHash == "" || item.CreatedAt.IsZero() {
			continue
		}
		for i := -1; i <= 1; i++ {
			targetTime := item.CreatedAt.AddDate(0, 0, i)
			shardTableName := getShradTableByTime(targetTime)
			shardTableMap[shardTableName] = append(shardTableMap[shardTableName], item)
		}
	}

	var tradeRs []*Trade
	for shardTableName, items := range shardTableMap {
		if !m.conn.WithContext(ctx).Migrator().HasTable(shardTableName) {
			continue
		}

		//去重，通过txhash
		txHashes := lo.Uniq(lo.FilterMap(items, func(item *Trade, _ int) (string, bool) {
			if item == nil || item.TxHash == "" {
				return "", false
			}
			return item.TxHash, true
		}))

		if len(txHashes) == 0 {
			continue
		}

		var trades []*Trade
		err := m.conn.WithContext(ctx).
			Table(shardTableName).
			Model(&Trade{}).
			Where("tx_hash IN ?", txHashes).
			Order("id asc").
			Find(&trades).Error
		if err != nil {
			return nil, err
		}

		tradeRs = append(tradeRs, trades...)
	}
	tradeRs = lo.Filter(tradeRs, func(trade *Trade, _ int) bool {
		return trade != nil
	})
	//去重，通过id和txhash，优先id，因为有些数据可能没有id（回撤数据），只有txhash
	tradeRs = lo.UniqBy(tradeRs, func(trade *Trade) string {
		if trade.Id != 0 {
			return fmt.Sprintf("id:%d", trade.Id)
		}
		return "tx:" + trade.TxHash
	})
	return tradeRs, nil

}

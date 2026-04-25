package mqs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"myDex/market/internal/svc"
	"myDex/model/solmodel"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type KlineRecord struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ChainID     int64     `gorm:"column:chain_id;not null;uniqueIndex:uk_market_kline,priority:1"`
	PairAddr    string    `gorm:"column:pair_addr;type:varchar(128);not null;uniqueIndex:uk_market_kline,priority:2"`
	TokenAddr   string    `gorm:"column:token_addr;type:varchar(128);not null"`
	Interval    string    `gorm:"column:interval_val;type:varchar(16);not null;uniqueIndex:uk_market_kline,priority:3"`
	CandleTime  int64     `gorm:"column:candle_time;not null;uniqueIndex:uk_market_kline,priority:4"`
	Open        float64   `gorm:"column:open;not null"`
	High        float64   `gorm:"column:high;not null"`
	Low         float64   `gorm:"column:low;not null"`
	Close       float64   `gorm:"column:close;not null"`
	McapOpen    float64   `gorm:"column:mcap_open;not null"`
	McapHigh    float64   `gorm:"column:mcap_high;not null"`
	McapLow     float64   `gorm:"column:mcap_low;not null"`
	McapClose   float64   `gorm:"column:mcap_close;not null"`
	AmountUSD   float64   `gorm:"column:amount_usd;not null"`
	VolumeToken float64   `gorm:"column:volume_token;not null"`
	OpenAt      int64     `gorm:"column:open_at;not null"`
	CloseAt     int64     `gorm:"column:close_at;not null"`
	AvgPrice    float64   `gorm:"column:avg_price;not null"`
	TotalCount  int64     `gorm:"column:total_count;not null"`
	BuyCount    int64     `gorm:"column:buy_count;not null"`
	SellCount   int64     `gorm:"column:sell_count;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (KlineRecord) TableName() string {
	return "market_kline"
}

type KlineTradeProcess struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	HashID    string    `gorm:"column:hash_id;type:varchar(191);not null;uniqueIndex:uk_market_kline_process,priority:1"`
	Interval  string    `gorm:"column:interval_val;type:varchar(16);not null;uniqueIndex:uk_market_kline_process,priority:2"`
	PairAddr  string    `gorm:"column:pair_addr;type:varchar(128);not null"`
	TxHash    string    `gorm:"column:tx_hash;type:varchar(191);not null"`
	BlockTime int64     `gorm:"column:block_time;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (KlineTradeProcess) TableName() string {
	return "market_kline_trade_process"
}

func ensureKlineTables(db *gorm.DB) error {
	return db.AutoMigrate(&KlineRecord{}, &KlineTradeProcess{})
}

func persistTradesAsKlines(ctx context.Context, svcCtx *svc.ServiceContext, trades []*TradeMessage) (int, error) {
	if svcCtx == nil || svcCtx.DB == nil {
		return 0, errors.New("db is nil")
	}

	klineModel := svcCtx.MarketKlineModel
	applied := 0
	err := svcCtx.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		klineModel = klineModel.WithSession(tx)
		for _, trade := range trades {
			if trade == nil || !isValidTrade(trade) {
				continue
			}

			for _, intervalMinute := range klineIntervals {
				interval := minuteToInterval(intervalMinute)
				inserted, err := markTradeProcessed(ctx, tx, trade, interval)
				if err != nil {
					return err
				}
				if !inserted {
					continue
				}

				if err := upsertKlineWithTrade(ctx, klineModel, trade, intervalMinute); err != nil {
					return err
				}
				applied++
			}
		}
		return nil
	})
	return applied, err
}

func markTradeProcessed(ctx context.Context, tx *gorm.DB, trade *TradeMessage, interval string) (bool, error) {
	hashID := trade.HashID
	if hashID == "" {
		hashID = fmt.Sprintf("%s:%s:%d:%s", trade.PairAddr, trade.TxHash, trade.BlockTime, trade.Type)
	}

	record := &solmodel.MarketKlineTradeProcess{
		HashId:      hashID,
		IntervalVal: interval,
		PairAddr:    trade.PairAddr,
		TxHash:      trade.TxHash,
		BlockTime:   trade.BlockTime,
	}

	model := solmodel.NewMarketKlineTradeProcessModel(tx)
	inserted, err := model.InsertIgnore(ctx, record)
	if err != nil {
		return false, err
	}
	return inserted, nil
}

func upsertKlineWithTrade(ctx context.Context, model solmodel.MarketKlineModel, trade *TradeMessage, intervalMinute int) error {
	candleTime := getCandleTime(trade.BlockTime, intervalMinute)
	interval := minuteToInterval(intervalMinute)

	record, err := model.FindOneByUniqueKey(ctx, int64(trade.ChainIDInt), trade.PairAddr, interval, candleTime)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			kline := newKlineByTrade(trade, candleTime, intervalMinute)
			if kline == nil {
				return nil
			}
			return model.Insert(ctx, klineToModel(kline))
		}
		return err
	}

	kline := modelToKline(record)
	updateKlineByTrade(kline, trade)
	updated := klineToModel(kline)

	return model.UpdateByID(ctx, record.Id, map[string]any{
		"token_addr":   updated.TokenAddr,
		"open":         updated.Open,
		"high":         updated.High,
		"low":          updated.Low,
		"close":        updated.Close,
		"mcap_open":    updated.McapOpen,
		"mcap_high":    updated.McapHigh,
		"mcap_low":     updated.McapLow,
		"mcap_close":   updated.McapClose,
		"amount_usd":   updated.AmountUsd,
		"volume_token": updated.VolumeToken,
		"open_at":      updated.OpenAt,
		"close_at":     updated.CloseAt,
		"avg_price":    updated.AvgPrice,
		"total_count":  updated.TotalCount,
		"buy_count":    updated.BuyCount,
		"sell_count":   updated.SellCount,
		"updated_at":   time.Now(),
	})
}

func klineToModel(kline *Kline) *solmodel.MarketKline {
	return &solmodel.MarketKline{
		ChainId:     kline.ChainID,
		PairAddr:    kline.PairAddr,
		TokenAddr:   kline.TokenAddr,
		IntervalVal: kline.Interval,
		CandleTime:  kline.CandleTime,
		Open:        kline.Open,
		High:        kline.High,
		Low:         kline.Low,
		Close:       kline.Close,
		McapOpen:    kline.McapOpen,
		McapHigh:    kline.McapHigh,
		McapLow:     kline.McapLow,
		McapClose:   kline.McapClose,
		AmountUsd:   kline.AmountUSD,
		VolumeToken: kline.VolumeToken,
		OpenAt:      kline.OpenAt,
		CloseAt:     kline.CloseAt,
		AvgPrice:    kline.AvgPrice,
		TotalCount:  kline.TotalCount,
		BuyCount:    kline.BuyCount,
		SellCount:   kline.SellCount,
	}
}

func modelToKline(record *solmodel.MarketKline) *Kline {
	return &Kline{
		ChainID:     record.ChainId,
		PairAddr:    record.PairAddr,
		TokenAddr:   record.TokenAddr,
		Interval:    record.IntervalVal,
		CandleTime:  record.CandleTime,
		Open:        record.Open,
		High:        record.High,
		Low:         record.Low,
		Close:       record.Close,
		McapOpen:    record.McapOpen,
		McapHigh:    record.McapHigh,
		McapLow:     record.McapLow,
		McapClose:   record.McapClose,
		AmountUSD:   record.AmountUsd,
		VolumeToken: record.VolumeToken,
		OpenAt:      record.OpenAt,
		CloseAt:     record.CloseAt,
		AvgPrice:    record.AvgPrice,
		TotalCount:  record.TotalCount,
		BuyCount:    record.BuyCount,
		SellCount:   record.SellCount,
	}
}

func logPersistResult(total int, applied int) {
	logx.Infof("persisted kline batch, total_trades=%d applied_intervals=%d", total, applied)
}

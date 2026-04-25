package mqs

import (
	"fmt"
	"sort"
	"time"
)

var klineIntervals = []int{1, 5, 15, 60, 240, 720, 1440}

const (
	minAllowedPriceRatio = 0.2
	maxAllowedPriceRatio = 5.0
)

type Kline struct {
	ChainID     int64
	PairAddr    string
	TokenAddr   string
	Interval    string
	CandleTime  int64
	Open        float64 //	该周期内第一笔成交价
	High        float64 //	周期内最高价
	Low         float64 //  周期内最低价
	Close       float64 //	该周期内最后一笔成交价
	McapOpen    float64 //  周期内第一笔trade市值
	McapHigh    float64 //  周期内的最高市值
	McapLow     float64 //  周期内的最低市值
	McapClose   float64 //  周期内最后一笔trade市值
	AmountUSD   float64 //  周期内成交额累计
	VolumeToken float64 //  周期内成交量累计
	OpenAt      int64   //  周期内第一笔成交时间
	CloseAt     int64   //  周期内最后一笔成交时间
	AvgPrice    float64 //  周期内成交均价
	TotalCount  int64   //  周期内总成交笔数
	BuyCount    int64   //  周期内买的数量
	SellCount   int64   //  周期内卖的数量
}

func GenerateKlines(trades []*TradeMessage) map[string][]*Kline {
	if len(trades) == 0 {
		return nil
	}

	detectClampTrades(trades)

	pairTrades := make(map[string][]*TradeMessage, len(trades))
	for _, trade := range trades {
		if trade == nil || !isValidTrade(trade) {
			continue
		}
		pairTrades[trade.PairAddr] = append(pairTrades[trade.PairAddr], trade)
	}

	if len(pairTrades) == 0 {
		return nil
	}

	result := make(map[string][]*Kline, len(pairTrades))
	for pairAddr, tradesOfPair := range pairTrades {
		//按照 BlockTime, TxHash, HashID 排序，从小到大排
		sort.Slice(tradesOfPair, func(i, j int) bool {
			if tradesOfPair[i].BlockTime != tradesOfPair[j].BlockTime {
				return tradesOfPair[i].BlockTime < tradesOfPair[j].BlockTime
			}
			if tradesOfPair[i].TxHash != tradesOfPair[j].TxHash {
				return tradesOfPair[i].TxHash < tradesOfPair[j].TxHash
			}
			return tradesOfPair[i].HashID < tradesOfPair[j].HashID
		})

		for _, intervalMinute := range klineIntervals {
			result[pairAddr] = append(result[pairAddr], aggregateTradesToKlines(tradesOfPair, intervalMinute)...)
		}
	}

	return result
}

func aggregateTradesToKlines(trades []*TradeMessage, intervalMinute int) []*Kline {
	if len(trades) == 0 {
		return nil
	}

	klinesByCandle := make(map[int64]*Kline, len(trades))
	for _, trade := range trades {
		candleTime := getCandleTime(trade.BlockTime, intervalMinute)
		kline := klinesByCandle[candleTime]
		if kline == nil {
			kline = newKlineByTrade(trade, candleTime, intervalMinute)
			if kline == nil {
				continue
			}
			klinesByCandle[candleTime] = kline
			continue
		}
		updateKlineByTrade(kline, trade)
	}

	klines := make([]*Kline, 0, len(klinesByCandle))
	for _, kline := range klinesByCandle {
		klines = append(klines, kline)
	}
	sort.Slice(klines, func(i, j int) bool {
		if klines[i].CandleTime == klines[j].CandleTime {
			return klines[i].Interval < klines[j].Interval
		}
		return klines[i].CandleTime < klines[j].CandleTime
	})
	return klines
}

func newKlineByTrade(trade *TradeMessage, candleTime int64, intervalMinute int) *Kline {
	if trade == nil {
		return nil
	}

	open := trade.TokenPriceUSD
	closePrice := trade.TokenPriceUSD
	high := trade.TokenPriceUSD
	low := trade.TokenPriceUSD
	mcapOpen := trade.Fdv
	mcapClose := trade.Fdv
	mcapHigh := trade.Fdv
	mcapLow := trade.Fdv
	openAt := trade.BlockTime
	closeAt := trade.BlockTime

	if trade.Clamp {
		open = 0
		closePrice = 0
		high = 0
		low = 0
		mcapOpen = 0
		mcapClose = 0
		mcapHigh = 0
		mcapLow = 0
		openAt = 1<<63 - 1
		closeAt = -1
	}

	return &Kline{
		ChainID:     int64(trade.ChainIDInt),
		PairAddr:    trade.PairAddr,
		TokenAddr:   trade.PairInfo.TokenAddr,
		Interval:    minuteToInterval(intervalMinute),
		CandleTime:  candleTime,
		Open:        open,
		High:        high,
		Low:         low,
		Close:       closePrice,
		McapOpen:    mcapOpen,
		McapHigh:    mcapHigh,
		McapLow:     mcapLow,
		McapClose:   mcapClose,
		AmountUSD:   trade.TotalUSD,
		VolumeToken: trade.TokenAmount,
		OpenAt:      openAt,
		CloseAt:     closeAt,
		AvgPrice:    trade.TokenPriceUSD,
		TotalCount:  1,
		BuyCount:    boolToCount(trade.Type == "buy"),
		SellCount:   boolToCount(trade.Type == "sell"),
	}
}

func updateKlineByTrade(kline *Kline, trade *TradeMessage) {
	if kline == nil || trade == nil {
		return
	}

	if !trade.Clamp {
		if trade.BlockTime < kline.OpenAt {
			kline.OpenAt = trade.BlockTime
			kline.Open = trade.TokenPriceUSD
			kline.McapOpen = trade.Fdv
		}
		if trade.BlockTime > kline.CloseAt {
			kline.CloseAt = trade.BlockTime
			kline.Close = trade.TokenPriceUSD
			kline.McapClose = trade.Fdv
		}

		if kline.High == 0 || trade.TokenPriceUSD > kline.High {
			kline.High = trade.TokenPriceUSD
		}
		if kline.Low == 0 || trade.TokenPriceUSD < kline.Low {
			kline.Low = trade.TokenPriceUSD
		}
		if kline.McapHigh == 0 || trade.Fdv > kline.McapHigh {
			kline.McapHigh = trade.Fdv
		}
		if kline.McapLow == 0 || trade.Fdv < kline.McapLow {
			kline.McapLow = trade.Fdv
		}
	}

	totalPrice := kline.AvgPrice * float64(kline.TotalCount)
	kline.TotalCount++
	kline.AvgPrice = (totalPrice + trade.TokenPriceUSD) / float64(kline.TotalCount)
	kline.AmountUSD += trade.TotalUSD
	kline.VolumeToken += trade.TokenAmount

	if trade.Type == "buy" {
		kline.BuyCount++
	}
	if trade.Type == "sell" {
		kline.SellCount++
	}
}

// 蜡烛时间计算,交易共享一个起始时间戳
func getCandleTime(blockTime int64, intervalMinute int) int64 {
	t := time.Unix(blockTime, 0).UTC()
	if intervalMinute < 60 {
		//比如2026-04-25 13:42  1min: 13:42  5min: 13:40  6min: 13:42
		minute := t.Minute() - t.Minute()%intervalMinute
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), minute, 0, 0, time.UTC).Unix()
	}

	hourInterval := intervalMinute / 60
	//比如2026-04-25 13:42  1h: 13  3h: 12  24h: 00
	hour := t.Hour() - t.Hour()%hourInterval
	return time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, time.UTC).Unix()
}

func minuteToInterval(intervalMinute int) string {
	switch intervalMinute {
	case 1:
		return "1m"
	case 5:
		return "5m"
	case 15:
		return "15m"
	case 60:
		return "1h"
	case 240:
		return "4h"
	case 720:
		return "12h"
	case 1440:
		return "1d"
	default:
		return fmt.Sprintf("%dm", intervalMinute)
	}
}

func buildTradeDedupKey(trade *TradeMessage) string {
	return fmt.Sprintf("%s:%s:%d:%s", trade.PairAddr, trade.TxHash, trade.BlockTime, trade.Type)
}

func isValidTrade(trade *TradeMessage) bool {
	if trade == nil {
		return false
	}
	if trade.PairAddr == "" || trade.TxHash == "" {
		return false
	}
	if trade.TokenPriceUSD <= 0 || trade.BaseTokenPriceUSD <= 0 {
		return false
	}
	return trade.Type == "buy" || trade.Type == "sell"
}

func boolToCount(ok bool) int64 {
	if ok {
		return 1
	}
	return 0
}


func hasMixedBuySell(trades []*TradeMessage) bool {
	hasBuy := false
	hasSell := false
	for _, trade := range trades {
		if trade == nil {
			continue
		}
		if trade.Type == "buy" {
			hasBuy = true
		}
		if trade.Type == "sell" {
			hasSell = true
		}
		if hasBuy && hasSell {
			return true
		}
	}
	return false
}

func detectClampTrades(trades []*TradeMessage) {
	tradePairMap := make(map[string][]*TradeMessage)
	for _, trade := range trades {
		if trade == nil || trade.PairAddr == "" {
			continue
		}
		trade.Clamp = false
		tradePairMap[trade.PairAddr] = append(tradePairMap[trade.PairAddr], trade)
	}

	for _, pairTrades := range tradePairMap {
		makerFirstTrade := make(map[string]int)
		makerTradeType := make(map[string]int)
		for i, trade := range pairTrades {
			if trade == nil || trade.Maker == "" {
				continue
			}
			switch trade.Type {
			case "buy":
				makerTradeType[trade.Maker] |= 1 << 0
			case "sell":
				makerTradeType[trade.Maker] |= 1 << 1
			}

			if lastIndex, ok := makerFirstTrade[trade.Maker]; ok {
				last := pairTrades[lastIndex]
				if last != nil && last.BlockNum == trade.BlockNum {
					if makerTradeType[trade.Maker] >= (1<<0 | 1<<1) {
						for j := lastIndex; j <= i; j++ {
							if pairTrades[j] != nil && pairTrades[j].PairAddr == trade.PairAddr && pairTrades[j].Maker == trade.Maker && pairTrades[j].BlockNum == trade.BlockNum {
								pairTrades[j].Clamp = true
							}
						}
					}
				} else {
					makerFirstTrade[trade.Maker] = i
					makerTradeType[trade.Maker] = 0
					switch trade.Type {
					case "buy":
						makerTradeType[trade.Maker] |= 1 << 0
					case "sell":
						makerTradeType[trade.Maker] |= 1 << 1
					}
				}
			} else {
				makerFirstTrade[trade.Maker] = i
			}
		}
	}
}

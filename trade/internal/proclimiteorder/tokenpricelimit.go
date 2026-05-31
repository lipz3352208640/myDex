package proclimiteorder

import (
	"context"
	"encoding/json"
	"fmt"

	"myDex/market/market"
	"myDex/model/solmodel"
	tradepkg "myDex/pkg/trade"
	"myDex/trade/internal/entity"
	"myDex/trade/internal/enum"
	"myDex/trade/internal/logic"
	"myDex/trade/internal/svc"
	"myDex/trade/trade"

	"github.com/samber/lo"
	"github.com/zeromicro/go-zero/core/logx"

	"github.com/shopspring/decimal"
)

type TokenPriceLimit struct {
	Svc *svc.ServiceContext
	logx.Logger
}

func NewTokenPriceLimit(svc *svc.ServiceContext) *TokenPriceLimit {
	logger := logx.WithContext(context.Background()).WithFields(logx.LogField{Key: "service", Value: "txManage"})

	return &TokenPriceLimit{
		Svc:    svc,
		Logger: logger,
	}
}

func (s *TokenPriceLimit) Consume(lowerSequence, upperSequence int64, buffer []*entity.OrderMessage) {
	s.Infof("disruptor consume: lower=%d upper=%d", lowerSequence, upperSequence)

	for i := lowerSequence; i <= upperSequence; i++ {

		message := buffer[i%int64(len(buffer))]
		if message == nil {
			s.Errorf("disruptor consume nil message at sequence=%d", i)
			continue
		}
		s.DoConsume(message)
	}
}

func (s *TokenPriceLimit) DoConsume(message *entity.OrderMessage) {
	//step 1：judge swap type is buy or sell

	err := s.Svc.Pool.Submit(func() {
		swapTypes := []trade.SwapType{trade.SwapType_Buy, trade.SwapType_Sell}
		lo.ForEach[trade.SwapType](swapTypes, func(swapType trade.SwapType, _ int) {
			message.SwapType = int64(swapType)
			s.processTokenPriceLimitOrdersFromRedis(message)
		})
	})
	if err != nil {
		s.Error(err)
	}

	// if message.SwapType == int64(trade.SwapType_Buy) || message.SwapType == int64(trade.SwapType_Sell) {
	// 	s.processTokenPriceLimitOrdersFromRedis(message)
	// } else {
	// 	s.Errorf("not support swap type")
	// }

}

func (s *TokenPriceLimit) processTokenPriceLimitOrdersFromRedis(message *entity.OrderMessage) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := buildLimitOrderRedisKey(message)
	if err != nil {
		s.Errorf("build redis key failed: %v", err)
		return
	}
	s.Infof("processing token price limit orders, key=%s", key)

	lockKey := key + ":lock"
	lockValue := fmt.Sprintf("%s:%d:%s", message.TokenCA, message.ChainId, message.CurrentPrice)
	//redis 分布式锁
	locked, err := s.Svc.Redis.SetnxExCtx(ctx, lockKey, lockValue, 10)
	if err != nil {
		s.Errorf("acquire redis lock failed, key=%s err=%v", lockKey, err)
		return
	}
	if !locked {
		return
	}
	defer s.releaseRedisLock(ctx, lockKey, lockValue)

	currentPrice, err := decimal.NewFromString(message.CurrentPrice)
	if err != nil {
		s.Errorf("invalid current price %q: %v", message.CurrentPrice, err)
		return
	}

	s.Infof("currentPrice=%s", currentPrice.String())
	orderStrs, err := s.Svc.Redis.LrangeCtx(ctx, key, 0, -1)
	if err != nil {
		s.Errorf("load limit orders from redis failed, key=%s err=%v", key, err)
		return
	}
	if len(orderStrs) == 0 {
		return
	}

	orders := make([]*entity.RedisTokenPriceLimitOrderInfo, 0, len(orderStrs))
	triggered := make([]*entity.RedisTokenPriceLimitOrderInfo, 0)

	s.Infof("currentPrice=%s orderStrs=%v", currentPrice.String(), orderStrs)
	for _, orderStr := range orderStrs {
		info, err := String2Struct[*entity.RedisTokenPriceLimitOrderInfo](orderStr)

		if err != nil {
			s.Errorf("decode limit order failed: %v", err)
			continue
		}
		orders = append(orders, info)

		limitPrice, err := decimal.NewFromString(info.BasePrice)
		if err != nil {
			s.Errorf("invalid limit price orderId=%d price=%q err=%v", info.OrderId, info.BasePrice, err)
			continue
		}

		if shouldTriggerLimitOrder(trade.SwapType(message.SwapType), limitPrice, currentPrice) {
			triggered = append(triggered, info)
		}
	}
	fmt.Println("triggered:  \n", len(triggered))
	if len(triggered) == 0 {
		return
	}
	fmt.Println("11111")
	remain := make([]*entity.RedisTokenPriceLimitOrderInfo, 0, len(orders)-len(triggered))
	triggeredMap := make(map[int64]struct{}, len(triggered))
	for _, info := range triggered {
		triggeredMap[info.OrderId] = struct{}{}
	}
	for _, info := range orders {
		if _, ok := triggeredMap[info.OrderId]; !ok {
			remain = append(remain, info)
		}
	}
	fmt.Println("21111")
	for _, info := range triggered {
		if err := s.executeLimitTokenPriceOrder(ctx, message, info.OrderId); err != nil {
			s.Errorf("execute limit order failed, orderId=%d err=%v", info.OrderId, err)
			remain = append(remain, info)
		}
	}

	if _, err := s.Svc.Redis.DelCtx(ctx, key); err != nil {
		s.Errorf("clear redis key failed, key=%s err=%v", key, err)
		return
	}

	for _, info := range remain {
		raw, err := info.Serialize()
		if err != nil {
			s.Errorf("serialize remain order failed, orderId=%d err=%v", info.OrderId, err)
			continue
		}
		if _, err := s.Svc.Redis.RpushCtx(ctx, key, raw); err != nil {
			s.Errorf("rewrite remain order failed, orderId=%d err=%v", info.OrderId, err)
			return
		}
	}
}

func String2Struct[T any](data string) (T, error) {
	var t T
	err := json.Unmarshal([]byte(data), &t)
	if err != nil {
		return t, err
	}
	return t, nil
}

func buildLimitOrderRedisKey(message *entity.OrderMessage) (string, error) {
	//message.SwapType = 2
	switch trade.SwapType(message.SwapType) {
	case trade.SwapType_Buy:
		return fmt.Sprintf("%v:%v:%v", tradepkg.RedisLimitOrderBuyPrefix, message.TokenCA, message.ChainId), nil
	case trade.SwapType_Sell:
		return fmt.Sprintf("%v:%v:%v", tradepkg.RedisLimitOrderSellPrefix, message.TokenCA, message.ChainId), nil
	default:
		return "", fmt.Errorf("unsupported swap type: %d", message.SwapType)
	}
}

func decodeRedisLimitOrder(raw string) (*entity.RedisTokenPriceLimitOrderInfo, error) {
	var info entity.RedisTokenPriceLimitOrderInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func shouldTriggerLimitOrder(swapType trade.SwapType, limitPrice, currentPrice decimal.Decimal) bool {
	fmt.Printf("swapType=%v limitPrice=%s currentPrice=%s\n", swapType, limitPrice.String(), currentPrice.String())
	switch swapType {
	case trade.SwapType_Buy:
		return limitPrice.GreaterThanOrEqual(currentPrice)
	case trade.SwapType_Sell:
		return currentPrice.GreaterThanOrEqual(limitPrice)
	default:
		return false
	}
}

func (s *TokenPriceLimit) releaseRedisLock(ctx context.Context, key, value string) {
	const lua = `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`
	if _, err := s.Svc.Redis.EvalCtx(ctx, lua, []string{key}, value); err != nil {
		s.Errorf("release redis lock failed, key=%s err=%v", key, err)
	}
}

func (s *TokenPriceLimit) executeLimitTokenPriceOrder(ctx context.Context, message *entity.OrderMessage, orderID int64) error {
	order, err := s.Svc.TradeOrderModel.FindOne(ctx, orderID)
	if err != nil {
		return err
	}
	fmt.Println("31111")
	if order.Status != int64(enum.OrderStatus_Wait) {
		return nil
	}

	order.Status = int64(enum.OrderStatus_Proc)
	if err := s.Svc.TradeOrderModel.UpdateOrder(ctx, order, []string{"status"}); err != nil {
		return err
	}

	pairInfo, marketReq, err := s.buildTriggeredMarketOrder(ctx, order, message)
	if err != nil {
		order.Status = int64(enum.OrderStatus_Wait)
		_ = s.Svc.TradeOrderModel.UpdateOrder(ctx, order, []string{"status"})
		return err
	}

	fmt.Printf("pairInfo=%+v marketReq=%+v \n", pairInfo, marketReq)
	_, err = logic.NewCreateMarketOrderLogic(ctx, s.Svc).CreateMarketTx(order, pairInfo, marketReq)
	if err != nil {
		order.Status = int64(enum.OrderStatus_Fail)
		order.FailReason = err.Error()
		_ = s.Svc.TradeOrderModel.UpdateOrder(ctx, order, []string{"status", "fail_reason"})
		return err
	}

	return nil
}

func (s *TokenPriceLimit) buildTriggeredMarketOrder(ctx context.Context, order *solmodel.TradeOrder, message *entity.OrderMessage) (*market.PairInfo, *trade.MarketOrderRequest, error) {
	pairInfo, err := s.Svc.Marketclient.FindMaxSupplyPairInfoByTokenAddrAndChainID(ctx, &market.PairInfoRequest{
		ChainId:   order.ChainId,
		TokenAddr: order.TokenCa,
	})
	if err != nil {
		return nil, nil, err
	}

	amountIn := order.OrderAmount.String()
	// if trade.SwapType(order.SwapType) == trade.SwapType_Sell {
	// 	amountIn = order.OrderAmount.String()
	// }

	req := &trade.MarketOrderRequest{
		ChainId:           int32(message.ChainId),
		TokenCa:           order.TokenCa,
		SwapType:          trade.SwapType(order.SwapType),
		AmountIn:          amountIn,
		IsDoubleOut:       order.DoubleOut == 1,
		UserWalletAddress: order.WalletAddress,
	}

	return pairInfo, req, nil
}

package logic

import (
	"context"
	"fmt"
	"time"

	"myDex/market/market"
	"myDex/model/solmodel"
	tradepkg "myDex/pkg/trade"
	"myDex/pkg/xcode"
	"myDex/trade/internal/entity"
	"myDex/trade/internal/enum"
	"myDex/trade/internal/svc"
	"myDex/trade/trade"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
)

type CreateLimitMarketOrderLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateLimitMarketOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateLimitMarketOrderLogic {
	return &CreateLimitMarketOrderLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CreateLimitMarketOrderLogic) CreateLimitMarketOrder(in *trade.LimitMarketOrderRequest) (*trade.LimitMarketOrderResponse, error) {
	// todo: add your logic here and delete this line

	var (
		orderPriceBase, orderValueBase decimal.Decimal
		tradeType                      int64
	)
	//step 1: validate input parameters
	err := l.validateInput(in)
	if err != nil {
		return nil, err
	}

	//step 2：calc token price (token -> baseToken)
	ctx, cancle := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancle()
	pairInfo, err := l.svcCtx.Marketclient.FindMaxSupplyPairInfoByTokenAddrAndChainID(ctx, &market.PairInfoRequest{
		ChainId:   int64(in.ChainId),
		TokenAddr: in.TokenCa,
	})
	if err != nil {
		return nil, xcode.PoolNotFound
	}
	amountIn, err := decimal.NewFromString(in.AmountIn)
	if err != nil {
		return nil, fmt.Errorf("invalid amount format: %v", err)
	}
	baseTokenPrice := pairInfo.BaseTokenPrice
	if len(in.PriceUsd) != 0 {
		priceUsd, err := decimal.NewFromString(in.PriceUsd)
		if err != nil {
			return nil, fmt.Errorf("invalid price format: %v", err)
		}
		if !priceUsd.IsZero() {
			solPriceDecimal := decimal.NewFromFloat(baseTokenPrice)
			orderPriceBase = priceUsd.Div(solPriceDecimal)
			orderValueBase = amountIn
			if in.SwapType == trade.SwapType_Sell {
				orderValueBase = orderPriceBase.Mul(amountIn)
			}
			tradeType = int64(enum.TradeType_Limit)

		}
	}
	if len(in.OrderCap) != 0 {
		cap, err := decimal.NewFromString(in.OrderCap)
		if err != nil {
			return nil, fmt.Errorf("invalid price format: %v", err)
		}
		if !cap.IsZero() {
			totalSupply := pairInfo.Fdv / pairInfo.TokenPrice
			totalSupplyDecimal := decimal.NewFromFloat(totalSupply)

			priceUsd := cap.Div(totalSupplyDecimal)

			solPriceDecimal := decimal.NewFromFloat(baseTokenPrice)
			orderPriceBase = priceUsd.Div(solPriceDecimal)
			orderValueBase = amountIn
			if in.SwapType == trade.SwapType_Sell {
				orderValueBase = orderPriceBase.Mul(amountIn)
			}
			tradeType = int64(enum.TradeType_TokenCapLimit)
		}
	}

	//step 3: create limit order
	tradeOrder := &solmodel.TradeOrder{
		TradeType:      tradeType,
		ChainId:        int64(in.ChainId),
		TokenCa:        in.TokenCa,
		SwapType:       int64(in.SwapType),
		IsAutoSlippage: 0,
		Slippage:       1000, // 10000为满值，
		IsAntiMev:      0,
		GasType:        1,
		Status:         int64(enum.OrderStatus_Wait),
		OrderCap:       decimal.NewFromFloat(pairInfo.Fdv),
		OrderAmount:    amountIn,
		OrderPriceBase: orderPriceBase,
		OrderValueBase: orderValueBase,
		OrderBasePrice: decimal.NewFromFloat(pairInfo.BaseTokenPrice),
		// 是否翻倍出本 1:是 0:否
		DoubleOut:     BoolToInt64(in.IsDoubleOut),
		DexName:       pairInfo.Name,
		PairCa:        pairInfo.Address,
		WalletAddress: in.UserWalletAddress,
	}

	err = l.saveLimitOrder(tradeOrder)
	if err != nil {
		return nil, xcode.ServerErr
	}
	return &trade.LimitMarketOrderResponse{Success: true}, nil
}

func (l *CreateLimitMarketOrderLogic) saveLimitOrder(tradeOrder *solmodel.TradeOrder) error {
	//step 1：save limit order
	ctx, cancle := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancle()
	model := l.svcCtx.TradeOrderModel
	err := model.InsertWithLog(ctx, tradeOrder)
	if err != nil {
		return err
	}
	// step 2：save order to redis list

	switch tradeOrder.TradeType {
	case int64(enum.TradeType_Limit), int64(enum.TradeType_TokenCapLimit):
		tradeOrder.Status = int64(enum.OrderStatus_Fail)
		if err := l.SaveToRedis(tradeOrder); err != nil {
			if err := model.UpdateOrder(ctx, tradeOrder, []string{"status"}); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("not support trade type")
	}
	logx.Infof("save limit order to db and redis, orderId=%d", tradeOrder.Id)
	return nil
}

func (l *CreateLimitMarketOrderLogic) SaveToRedis(tradeOrder *solmodel.TradeOrder) error {

	var (
		key string
	)

	switch tradeOrder.SwapType {
	case int64(trade.SwapType_Buy):
		key = fmt.Sprintf("%v:%v:%v", tradepkg.RedisLimitOrderBuyPrefix, tradeOrder.TokenCa, tradeOrder.ChainId)
	case int64(trade.SwapType_Sell):
		key = fmt.Sprintf("%v:%v:%v", tradepkg.RedisLimitOrderSellPrefix, tradeOrder.TokenCa, tradeOrder.ChainId)
	default:
		return fmt.Errorf("not support trade type")
	}
	redisTraOrder := &entity.RedisTokenPriceLimitOrderInfo{
		OrderId:   tradeOrder.Id,
		BasePrice: tradeOrder.OrderPriceBase.String(),
	}
	jsonStr, err := redisTraOrder.Serialize()
	if err != nil {
		return err
	}
	_, err = l.svcCtx.Redis.RpushCtx(l.ctx, key, jsonStr)
	if err != nil {
		return err
	}
	return nil
}

func (l *CreateLimitMarketOrderLogic) validateInput(in *trade.LimitMarketOrderRequest) error {
	// Implement your validation logic here

	if in == nil {
		return fmt.Errorf("input is nil")
	}
	if len(in.TokenCa) == 0 {
		return fmt.Errorf("token contract address is empty")
	}
	if len(in.AmountIn) == 0 {
		return fmt.Errorf("amount in is empty")
	}
	decimalAmount, err := decimal.NewFromString(in.AmountIn)
	if err != nil {
		return fmt.Errorf("invalid amount format: %v", err)
	}
	if decimalAmount.Cmp(decimal.Zero) <= 0 {
		return fmt.Errorf("amount must be greater than zero")
	}
	if len(in.UserWalletAddress) == 0 {
		return fmt.Errorf("user wallet is empty")
	}
	return nil
}

func (l *CreateLimitMarketOrderLogic) CreateTradeOrder(order *solmodel.TradeOrder) error {
	model := solmodel.NewTradeOrderModel(l.svcCtx.DB)
	if err := model.InsertWithLog(l.ctx, order); err != nil {
		return fmt.Errorf("CreateLimitOrder Insert err: %s", err.Error())
	}

	switch order.TradeType {
	case int64(enum.TradeType_Limit):
		if err := l.SaveToRedis(order); err != nil {
			order.Status = int64(enum.OrderStatus_Fail)
			if err := model.UpdateOrder(l.ctx, order, []string{"status"}); err != nil {
				l.Errorf("addOrderToRedis err:%s", err.Error())
				return err
			}
			return err
		}
	default:
		return fmt.Errorf("err tradetype:%v", order.TradeType)
	}
	return nil
}

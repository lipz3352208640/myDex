package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"myDex/market/market"
	"myDex/model/solmodel"
	"myDex/pkg/constant"
	"myDex/pkg/xcode"
	"myDex/trade/internal/enum"
	"myDex/trade/internal/logic/entity"
	"myDex/trade/internal/svc"
	"myDex/trade/trade"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
)

type CreateMarketOrderLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateMarketOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateMarketOrderLogic {
	return &CreateMarketOrderLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

/**
  首先要明确sell或者buy指令中的账户以及参数，根据指令中的数据进行构建
  Amount      *uint64
	MaxSolCost  *uint64
	TrackVolume *OptionBool

	// [0] = [] global
	//
	// [1] = [WRITE] fee_recipient
	//
	// [2] = [] mint
	//
	// [3] = [WRITE] bonding_curve
	//
	// [4] = [WRITE] associated_bonding_curve
	//
	// [5] = [WRITE] associated_user
	//
	// [6] = [WRITE, SIGNER] user
	//
	// [7] = [] system_program
	//
	// [8] = [] token_program
	//
	// [9] = [WRITE] creator_vault
	//
	// [10] = [] event_authority
	//
	// [11] = [] program
	//
	// [12] = [WRITE] global_volume_accumulator
	//
	// [13] = [WRITE] user_volume_accumulator
	//
	// [14] = [] fee_config
	//
	// [15] = [] fee_program
*/

func (l *CreateMarketOrderLogic) CreateMarketOrder(in *trade.MarketOrderRequest) (*trade.MarketOrderResponse, error) {
	// todo: add your logic here and delete this line
	// 检验输入数量
	amountIn := in.AmountIn
	if len(amountIn) == 0 {
		return nil, xcode.AmountErr
	}

	amountInDecimal, err := decimal.NewFromString(amountIn)
	if err != nil {
		return nil, err
	}

	if !amountInDecimal.IsPositive() {
		return nil, xcode.AmountErr
	}

	//根据trade pair，获取token,sol 价格，以及token,sol池子数量，用于计算最小数量等
	pairInfo, err := l.svcCtx.Marketclient.FindMaxSupplyPairInfoByTokenAddrAndChainID(l.ctx, &market.PairInfoRequest{
		ChainId:   int64(in.ChainId),
		TokenAddr: in.TokenCa,
	})
	if err != nil {
		l.Errorf("get pairInfo err is %v", err.Error())
		return nil, xcode.ServerErr
	}
	//如果当前流动池中的base token价格缺失，获取该base token的最近成交价
	baseTokenPrice := pairInfo.BaseTokenPrice
	tokenPrice := pairInfo.BaseTokenPrice
	if decimal.NewFromFloat(pairInfo.BaseTokenPrice).IsZero() {
		baseTokenPriceResp, err := l.svcCtx.Marketclient.FindNearBaseTokenPrice(l.ctx, &market.BaseTokenPriceRequest{
			ChainId:       int64(in.ChainId),
			BaseTokenAddr: pairInfo.BaseTokenAddress,
		})
		if err != nil {
			return nil, err
		}

		if baseTokenPriceResp.BaseTokenPrice == 0 {
			return nil, xcode.PoolLiquidityNotEnough
		}
		baseTokenPrice = baseTokenPriceResp.BaseTokenPrice

	}

	if decimal.NewFromFloat(pairInfo.TokenPrice).IsZero() {
		baseTokenPriceResp, err := l.svcCtx.Marketclient.FindNearTokenPrice(l.ctx, &market.TokenPriceRequest{
			ChainId:       int64(in.ChainId),
			TokenAddr:     pairInfo.TokenAddress,
			BaseTokenAddr: pairInfo.BaseTokenAddress,
		})
		if err != nil {
			return nil, err
		}
		if baseTokenPriceResp.TokenPrice == 0 {
			return nil, xcode.PoolLiquidityNotEnough
		}
		tokenPrice = baseTokenPriceResp.TokenPrice
	}

	solPriceDecimal := decimal.NewFromFloat(baseTokenPrice)
	tokenPriceDecimal := decimal.NewFromFloat(tokenPrice)
	//挂单价格 1个token兑换几个sol
	orderPriceBase := tokenPriceDecimal.Div(solPriceDecimal)
	//挂单数量(基于sol) sell: 卖出的token * orderPriceBase   buy：下单的数量
	orderValueBase := amountInDecimal
	if in.SwapType == trade.SwapType_Sell {
		orderValueBase = orderPriceBase.Mul(amountInDecimal)
	}

	var isAutoSlippage int64 = 1
	var isAntiMev int64 = 0

	tradeOrder := &solmodel.TradeOrder{
		TradeType:      int64(enum.TradeType_Market),
		ChainId:        int64(in.ChainId),
		TokenCa:        in.TokenCa,
		SwapType:       int64(in.SwapType),
		IsAutoSlippage: isAutoSlippage,
		Slippage:       5000, // 10000为满值，
		IsAntiMev:      isAntiMev,
		GasType:        1,
		Status:         int64(enum.OrderStatus_Proc),
		OrderCap:       decimal.NewFromFloat(pairInfo.Fdv),
		OrderAmount:    amountInDecimal,
		OrderPriceBase: tokenPriceDecimal,
		OrderValueBase: orderValueBase,
		OrderBasePrice: decimal.NewFromFloat(pairInfo.BaseTokenPrice),
		// 是否翻倍出本 1:是 0:否
		DoubleOut:     BoolToInt64(in.IsDoubleOut),
		DexName:       pairInfo.Name,
		PairCa:        pairInfo.Address,
		WalletAddress: in.UserWalletAddress,
	}

	err = l.svcCtx.TradeOrderModel.InsertWithLog(l.ctx, tradeOrder)

	if err != nil {
		l.Errorf("InsertWithLog err:%s", err.Error())

		return nil, xcode.ServerErr
	}

	txhash, err := CreateMarketTx(tradeOrder, pairInfo, in)
	if err != nil {
		l.Errorf("acquire tx hash err:%s", err.Error())
		return nil, xcode.ServerErr
	}

	if len(txhash) == 0 {
		l.Errorf("acquire tx hash is 0, err:%s", err.Error())
		return nil, xcode.ServerErr
	}

	return &trade.MarketOrderResponse{
		TxHash: txhash,
	}, nil
}

func BoolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// 创建市单交易
func (l *CreateMarketOrderLogic) CreateMarketTx(order *solmodel.TradeOrder, pairInfo *market.PairInfo, in *trade.MarketOrderRequest) (string, error) {
	var err error
	defer func() {
		// 如果订单状态是触发中 并且有错误，那么将订单状态改为失败
		if order.Status == int64(enum.OrderStatus_Proc) && err != nil {
			err2 := l.updateDbByTxResult(order, nil, "", err)
			if err2 != nil {
				l.Errorf("update order err err is %v", err2)
			}
		}
	}()

	inProgrmID, outProgramID, err := l.getTokenProgramID(pairInfo, in.SwapType)

	if err != nil {
		return "", err
	}

	inBaseTokenAddree := pairInfo.BaseTokenAddress
	outTokenAddree := pairInfo.TokenAddress
	outTokenDecimal := pairInfo.TokenDecimal
	inBaseTokenDecimal := pairInfo.BaseTokenDecimal
	if in.SwapType == trade.SwapType_Sell {
		inBaseTokenAddree = pairInfo.TokenAddress
		outTokenAddree = pairInfo.BaseTokenAddress
		inBaseTokenDecimal = pairInfo.TokenDecimal
		outTokenDecimal = pairInfo.BaseTokenDecimal
	}

	param := &entity.MarketTx{
		UserId:            uint64(order.Uid),
		UserWalletId:      uint32(order.WalletIndex),
		UserWalletAddress: in.UserWalletAddress,
		InTokenProgram:    inProgrmID,
		OutTokenProgram:   outProgramID,
		InDecimal:         uint8(inBaseTokenDecimal),
		OutDecimal:        uint8(outTokenDecimal),
		InTokenCa:         inBaseTokenAddree,
		OutTokenCa:        outTokenAddree,
		IsAntiMev:         order.IsAntiMev == 1,
		IsAutoSlippage:    order.IsAutoSlippage == 1,
		Slippage:          uint32(order.Slippage),
		SwapType:          int32(in.SwapType),
		TradePoolName:     pairInfo.Name,
		AmountIn:          in.AmountIn,
		Price:             order.OrderPriceBase.String(),
		PairAddr:          pairInfo.Address,
	}

	//如果选择开启自动滑点，如果设置过小，增大滑点重试
	tryTimes := 0
	for tryTimes == 0 || (order.IsAutoSlippage == 1 && errors.Is(err, xcode.SlippageLimit) && tryTimes < 3) {
		tryTimes++
		switch tryTimes {
		case 1:
		case 2:
			order.Slippage = 4500
			l.Info("AutoSlippageRetry")
		case 3:
			order.Slippage = 7000
			l.Info("AutoSlippageRetry")
		}
		txHash, err = l.createAndSendTx(param)
		if err != nil {
			err = convertSwapErr(pairInfo.Name, err)
		}
	}

}

func (l *CreateMarketOrderLogic) createAndSendTx(marketTx *entity.MarketTx) {
	chainId := marketTx.ChainId
	if chainId == constant.SolChainIdInt {
		l.svcCtx.TxMananger.BuildUnsignedTransaction(l.ctx, marketTx)
	}
}

func (l *CreateMarketOrderLogic) getTokenProgramID(pairInfo *market.PairInfo, swapType trade.SwapType) (string, string, error) {

	//获取in和out token的合于地址，用于创建ata账户指令
	inTokenAddree := pairInfo.BaseTokenAddress
	outTokenAddree := pairInfo.TokenAddress
	if swapType == trade.SwapType_Sell {
		inTokenAddree = pairInfo.TokenAddress
		outTokenAddree = pairInfo.BaseTokenAddress
	}
	outTokenInfo, err := l.svcCtx.Marketclient.FindTokenInfo(l.ctx, &market.TokenInfoRequest{
		ChainId:   pairInfo.ChainId,
		TokenAddr: outTokenAddree,
	})
	if err != nil {
		return "", "", err
	}
	if len(outTokenInfo.Program) == 0 {
		return "", "", fmt.Errorf("get token program is empty")
	}

	inTokenInfo, err := l.svcCtx.Marketclient.FindTokenInfo(l.ctx, &market.TokenInfoRequest{
		ChainId:   pairInfo.ChainId,
		TokenAddr: inTokenAddree,
	})
	if err != nil {
		return "", "", err
	}
	if len(inTokenInfo.Program) == 0 {
		return "", "", fmt.Errorf("get token program is empty")
	}
	return inTokenInfo.Program, outTokenInfo.Program, nil
}

func (l *CreateMarketOrderLogic) updateDbByTxResult() {

}

func convertSwapErr(poolName string, err error) error {
	result := err.Error()
	if strings.Contains(result, "liquidity") {
		return xcode.PoolLiquidityNotEnough
	}
	if strings.Contains(result, "insufficient") {
		return xcode.BalanceNotEnough
	}
	if strings.Contains(result, "frozen") {
		return xcode.TokenAccountFrozen
	}
	if strings.Contains(result, "slippage") || strings.Contains(result, "TooLittleOutputReceived") {
		return xcode.SlippageLimit
	}
	switch poolName {
	case constant.PumpFunName:
		if strings.Contains(result, "TooLittleSolReceived") || strings.Contains(result, "attempt to subtract with overflow") {
			return xcode.PumpPoolZeroErr
		}
	default:
	}
	return err
}

package logic

import (
	"context"

	"myDex/market/market"
	"myDex/model/solmodel"
	"myDex/pkg/xcode"
	"myDex/trade/internal/enum"
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
		l.Errorf("get pairInfo err is %w", err.Error())
		return nil, xcode.ServerErr
	}

	solPriceDecimal := decimal.NewFromFloat(pairInfo.BaseTokenPrice)
	tokenPriceDecimal := decimal.NewFromFloat(pairInfo.TokenPrice)
	//挂单价格 1个token兑换几个sol
	orderPriceBase := tokenPriceDecimal.Div(solPriceDecimal)
	//挂单数量(基于sol) sell: 卖出的token * orderPriceBase   buy：下单的数量
	orderValueBase := amountInDecimal
	if in.SwapType == trade.SwapType_Sell {
		orderValueBase = orderPriceBase.Mul(amountInDecimal)
	}

	var isAutoSlippage int64 = 0
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

	txhash,err := CreateMarketTx(tradeOrder, pairInfo)
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

func CreateMarketTx(order *solmodel.TradeOrder, pairInfo *market.PairInfo) (txHash string, err error) {

}

package logic

import (
	"context"

	"myDex/market/internal/svc"
	"myDex/market/market"
	"myDex/model/solmodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type FindMaxSupplyPairInfoByTokenAddrAndChainIDLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFindMaxSupplyPairInfoByTokenAddrAndChainIDLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FindMaxSupplyPairInfoByTokenAddrAndChainIDLogic {
	return &FindMaxSupplyPairInfoByTokenAddrAndChainIDLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FindMaxSupplyPairInfoByTokenAddrAndChainIDLogic) FindMaxSupplyPairInfoByTokenAddrAndChainID(in *market.PairInfoRequest) (*market.PairInfo, error) {
	// todo: add your logic here and delete this line

	var pairInfo *solmodel.Pair
	pairInfo, err := l.svcCtx.PairInfoModel.FindMaxSupplyPairInfoByTokenAddrAndChainID(l.ctx, in.ChainId, in.TokenAddr)
	if err != nil {
		return nil, err
	}

	return &market.PairInfo{
		ChainId:                pairInfo.ChainId,
		Address:                pairInfo.Address,
		Name:                   pairInfo.Name,
		FactoryAddress:         pairInfo.FactoryAddress,
		BaseTokenAddress:       pairInfo.BaseTokenAddress,
		TokenAddress:           pairInfo.TokenAddress,
		InitBaseTokenAmount:    pairInfo.InitBaseTokenAmount,
		BaseTokenSymbol:        pairInfo.BaseTokenSymbol,
		TokenSymbol:            pairInfo.TokenSymbol,
		BaseTokenPrice:         pairInfo.BaseTokenPrice,
		TokenPrice:             pairInfo.TokenPrice,
		InitTokenAmount:        pairInfo.InitTokenAmount,
		BaseTokenIsNativeToken: pairInfo.BaseTokenIsNativeToken == 1,
		BaseTokenIsToken0:      pairInfo.BaseTokenIsToken0 == 1,
		BaseTokenDecimal:       pairInfo.BaseTokenDecimal,
		TokenDecimal:           pairInfo.TokenDecimal,
		CurrentBaseTokenAmount: pairInfo.CurrentBaseTokenAmount,
		CurrentTokenAmount:     pairInfo.CurrentTokenAmount,
		BlockNum:               pairInfo.BlockNum,
		BlockTime:              pairInfo.BlockTime.Unix(),
		HighestTokenPrice:      pairInfo.HighestTokenPrice,
		LatestTradeTime:        pairInfo.LatestTradeTime.Unix(),
		Fdv:                    pairInfo.Fdv,
		MktCap:                 pairInfo.MktCap,
	}, nil
}

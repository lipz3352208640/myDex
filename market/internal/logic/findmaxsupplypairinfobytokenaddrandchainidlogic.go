package logic

import (
	"context"

	"myDex/market/internal/svc"
	"myDex/market/market"

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

	return &market.PairInfo{}, nil
}

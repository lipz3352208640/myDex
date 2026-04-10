package logic

import (
	"context"
	"time"

	"myDex/market/internal/svc"
	"myDex/market/market"
	"myDex/model/solmodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type FindNearTokenPriceLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFindNearTokenPriceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FindNearTokenPriceLogic {
	return &FindNearTokenPriceLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FindNearTokenPriceLogic) FindNearTokenPrice(in *market.TokenPriceRequest) (*market.TokenPriceResponse, error) {
	// todo: add your logic here and delete this line
	searchTime, err := time.Parse(time.DateTime, in.QueryTime)
	if err != nil {
		return nil, nil
	}

	var pairInfo *solmodel.Pair
	pairInfo, err = l.svcCtx.PairInfoModel.FindNearTokenPriceByChainIdAndTokenAddr(l.ctx, in.ChainId, in.BaseTokenAddr, in.BaseTokenAddr, searchTime)
	if err != nil {
		return nil, err
	}

	return &market.TokenPriceResponse{
		TokenPrice: pairInfo.BaseTokenPrice,
	}, nil
}

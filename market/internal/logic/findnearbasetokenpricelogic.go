package logic

import (
	"context"
	"time"

	"myDex/market/internal/svc"
	"myDex/market/market"
	"myDex/model/solmodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type FindNearBaseTokenPriceLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFindNearBaseTokenPriceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FindNearBaseTokenPriceLogic {
	return &FindNearBaseTokenPriceLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FindNearBaseTokenPriceLogic) FindNearBaseTokenPrice(in *market.BaseTokenPriceRequest) (*market.BaseTokenPriceResponse, error) {
	// todo: add your logic here and delete this line
	searchTime, err := time.Parse(time.DateTime, in.QueryTime)
	if err != nil {
		return nil, nil
	}

	var pairInfo *solmodel.Pair
	pairInfo, err = l.svcCtx.PairInfoModel.FindNearBaseTokenPriceByChainIdAndTokenAddr(l.ctx, in.ChainId, in.BaseTokenAddr, searchTime)
	if err != nil {
		return nil, err
	}

	return &market.BaseTokenPriceResponse{
		BaseTokenPrice: pairInfo.BaseTokenPrice,
	}, nil
}

package logic

import (
	"context"

	"myDex/market/internal/svc"
	"myDex/market/market"
	"myDex/model/solmodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type FindTokenInfoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFindTokenInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FindTokenInfoLogic {
	return &FindTokenInfoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FindTokenInfoLogic) FindTokenInfo(in *market.TokenInfoRequest) (*market.TokenInfoResponse, error) {
	// todo: add your logic here and delete this line

	var tokenInfo *solmodel.Token
	tokenInfo, err := l.svcCtx.TokenInfoModel.FindTokenInfoByTokenAddrAndChainID(l.ctx, in.ChainId, in.TokenAddr)
	if err != nil {
		return nil, err
	}

	return &market.TokenInfoResponse{
		ChainId: tokenInfo.ChainId,
		Address: tokenInfo.Address,
		Program: tokenInfo.Program,
		Name:    tokenInfo.Name,
	}, nil
}

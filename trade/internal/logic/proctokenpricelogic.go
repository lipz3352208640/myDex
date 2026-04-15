package logic

import (
	"context"
	"fmt"

	"myDex/trade/internal/entity"
	"myDex/trade/internal/svc"
	"myDex/trade/trade"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
)

type ProcTokenPriceLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewProcTokenPriceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProcTokenPriceLogic {
	return &ProcTokenPriceLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ProcTokenPriceLogic) ProcTokenPrice(in *trade.ProcTokenPriceRequest) (*trade.ProcTokenPriceResponse, error) {
	// todo: add your logic here and delete this line

	//step 1：use disruptor to publish message
	if l.svcCtx.DisruptorWrapper == nil {
		return nil, nil
	}
	if in.ChainId == 0 {
		l.Errorf("chainID is nil")
		return nil, fmt.Errorf("chainID is nil")
	}
	if in.SwapType <= 0 {
		l.Errorf("invalid swapType ")
		return nil, fmt.Errorf("chainID is nil")
	}
	if len(in.TokenCa) == 0 {
		l.Errorf("chainID is nil")
		return nil, fmt.Errorf("token address is nil ")
	}

	if len(in.Price) == 0 {
		return nil, fmt.Errorf("price is nil")
	}
	_, err := decimal.NewFromString(in.Price)
	if err != nil {
		return nil, fmt.Errorf("invalid price format: %v", err)
	}

	message := &entity.OrderMessage{
		TokenCA:      in.TokenCa,
		CurrentPrice: in.Price,
		ChainId:      int(in.ChainId),
		SwapType:     int64(in.SwapType),
	}
	l.svcCtx.DisruptorWrapper.Publish(message)

	return &trade.ProcTokenPriceResponse{}, nil
}

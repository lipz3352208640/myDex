package logic

import (
	"context"

	"myDex/myConsumer/internal/svc"
	"myDex/myConsumer/myConsumer"

	"github.com/zeromicro/go-zero/core/logx"
)

type PingLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewPingLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PingLogic {
	return &PingLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *PingLogic) Ping(in *myConsumer.Request) (*myConsumer.Response, error) {
	// todo: add your logic here and delete this line

	return &myConsumer.Response{}, nil
}

package main

import (
	"flag"
	"fmt"

	"myDex/pkg/queue"
	"myDex/trade/internal/config"
	"myDex/trade/internal/entity"
	"myDex/trade/internal/proclimiteorder"
	"myDex/trade/internal/server"
	"myDex/trade/internal/svc"
	"myDex/trade/trade"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/trade.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	ctx := svc.NewServiceContext(c)
	ctx.DisruptorWrapper = newDisruptor(ctx)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		trade.RegisterTradeServer(grpcServer, server.NewTradeServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}

func newDisruptor(svc *svc.ServiceContext) *queue.DisruptorWrapper[*entity.OrderMessage] {
	bufferSize := 1024
	limitSubscriber := proclimiteorder.NewTokenPriceLimit(svc)
	consumerGroup := []queue.Consumer[*entity.OrderMessage]{limitSubscriber}
	disruptorWrapper, err := queue.NewDisruptorWrapper[*entity.OrderMessage](int64(bufferSize), consumerGroup...)
	if err != nil {
		logx.Errorf("Failed to create DisruptorWrapper: %v", err)
		return nil
	}
	go disruptorWrapper.Start()
	proc.AddShutdownListener(func() {
		disruptorWrapper.Stop()
	})

	return disruptorWrapper
}

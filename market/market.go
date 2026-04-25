package main

import (
	"flag"
	"fmt"

	"myDex/market/internal/config"
	"myDex/market/internal/mqs"
	"myDex/market/internal/server"
	"myDex/market/internal/svc"
	"myDex/market/market"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/market.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	ctx := svc.NewServiceContext(c)

	group := service.NewServiceGroup()
	defer group.Stop()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		market.RegisterMarketServer(grpcServer, server.NewMarketServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	group.Add(s)

	if len(c.Kafka.Brokers) > 0 {
		consumerCount := c.Kafka.Consumers
		if consumerCount <= 0 {
			consumerCount = 1
		}

		for i := 0; i < consumerCount; i++ {
			consumer, err := mqs.NewKafkaTradeConsumer(c.Kafka, ctx, i)
			if err != nil {
				panic(err)
			}
			group.Add(consumer)
		}
		fmt.Printf("Kafka consumers enabled, topic=%s, group=%s, consumers=%d\n",
			c.Kafka.Topic, c.Kafka.Group, consumerCount)
	}

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	group.Start()
}

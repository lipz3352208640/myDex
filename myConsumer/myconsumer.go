package main

import (
	"flag"
	"fmt"

	"myDex/myConsumer/internal/config"
	"myDex/myConsumer/internal/logic/block"
	"myDex/myConsumer/internal/logic/slot"
	"myDex/myConsumer/internal/server"
	"myDex/myConsumer/internal/svc"
	"myDex/myConsumer/myConsumer"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/myconsumer.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	ctx := svc.NewServiceContext(c)

	//构建service组，增加业务service
	group := service.NewServiceGroup()
	defer group.Stop()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		myConsumer.RegisterMyConsumerServer(grpcServer, server.NewMyConsumerServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	group.Add(s)

	{

		//用生产者，消费者模型, channel 缓冲区不能为空，解决消费不及时，而导致数据阻塞堆积的问题
		slotchannel := make(chan uint64, 100)
		group.Add(block.NewBlockService(ctx, slotchannel, "consume-slot", ctx.Config.Thread.Count.Consumer))

		errSlotchannel := make(chan uint64, 1)
		group.Add(block.NewBlockService(ctx, errSlotchannel, "consume-Failed-slot", ctx.Config.Thread.Count.ConsumerFailedBlock))

		//加入slot业务service
		group.Add(slot.NewSlotGroupService(ctx, slotchannel, errSlotchannel))

	}

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	group.Start()
}

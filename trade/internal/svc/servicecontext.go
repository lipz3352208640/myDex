package svc

import (
	"fmt"
	"log"
	"myDex/market/market"
	"myDex/model/solmodel"
	"myDex/pkg/queue"
	"myDex/trade/internal/chain/solana"
	"myDex/trade/internal/chain/solana/arbitrage"
	"myDex/trade/internal/chain/solana/arbrunner"
	"myDex/trade/internal/chain/solana/jupiterarb"
	"myDex/trade/internal/chain/solana/jupiterclient"
	"myDex/trade/internal/config"
	"myDex/trade/internal/entity"
	"os"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ServiceContext struct {
	Config           config.Config
	Marketclient     market.MarketClient
	DB               *gorm.DB
	TradeOrderModel  solmodel.TradeOrderModel
	Redis            *redis.Redis
	DisruptorWrapper *queue.DisruptorWrapper[*entity.OrderMessage]
	TxMananger       *solana.TxManager
	JupiterClient    *jupiterclient.Client
	ArbDetector      *arbitrage.Detector
	JupiterArb       *jupiterarb.Builder
	ArbRunner        *arbrunner.Runner
	//goroutine 协程池
	Pool *ants.Pool
}

func NewServiceContext(c config.Config) *ServiceContext {
	redisService := c.Redis.NewRedis()
	marketClient := market.NewMarketClient(zrpc.MustNewClient(c.MarketService).Conn())

	pool, err := ants.NewPool(0)
	if err != nil {
		logx.Error("NewPool error:", err)
		os.Exit(1)
	}
	// 携程池的退出逻辑
	proc.AddShutdownListener(func() {
		logx.Info("stop pool")
		//关闭线程池，最多等待30s进行任务收尾工作
		if err := pool.ReleaseTimeout(30 * time.Second); err != nil {
			logx.Errorf("stop pool timeout err:%s , cap is %d", err.Error(), pool.Cap())
		}
	})

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.Mysql.User, c.Mysql.Password, c.Mysql.Host, c.Mysql.Port, c.Mysql.Dbname)
	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             time.Second * 3, // Slow SQL threshold
			LogLevel:                  logger.Warn,     // Log level
			IgnoreRecordNotFoundError: true,            // Ignore ErrRecordNotFound error for logger
			ParameterizedQueries:      false,           // Don't include params in the SQL log
			Colorful:                  true,
		},
	)
	//创建mysql连接
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})

	if err != nil {
		logx.Errorf("connect to mysql error: %v, dsn: %v", err, dsn)
		logx.Must(err)
	}

	//返回底层数据库的sql.DB对象，并设置连接池参数
	sqlDB, _ := db.DB()
	//设置最大空闲连接数
	sqlDB.SetMaxIdleConns(200)
	//数据库最大连接数(空闲连接和使用的连接)
	sqlDB.SetMaxOpenConns(500)
	//设置连接最大可存活时间，避免长连接不释放
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	fmt.Println("rpc endpoint:", c.Helius.NodeUrl[0])

	jupiterHTTPClient := jupiterclient.NewClient(
		nil,
		c.Jupiter.QuoteURL,
		c.Jupiter.SwapInstructionsURL,
	)

	txManager := solana.NewTxManager(db, c.Helius.NodeUrl[1], c.Helius.NodeUrl[1], c.SimulateOnly, c.Helius.JitoUrl)
	arbDetector := arbitrage.NewDetector(jupiterHTTPClient, c.Jupiter.ProfitThresholdLamports, c.Jupiter.TipBps)
	jupiterBuilder := jupiterarb.NewBuilder(jupiterHTTPClient)

	return &ServiceContext{
		Config:          c,
		Marketclient:    marketClient,
		DB:              db,
		TradeOrderModel: solmodel.NewTradeOrderModel(db),
		Redis:           redisService,
		Pool:            pool,
		TxMananger:      txManager,
		JupiterClient:   jupiterHTTPClient,
		ArbDetector:     arbDetector,
		JupiterArb:      jupiterBuilder,
		ArbRunner:       arbrunner.NewRunner(arbDetector, jupiterBuilder, txManager, c.Jupiter.JitoBundleURL),
	}
}

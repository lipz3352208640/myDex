package svc

import (
	"fmt"
	"log"
	"myDex/market/market"
	"myDex/model/solmodel"
	"myDex/trade/internal/chain/solana"
	"myDex/trade/internal/config"
	"os"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ServiceContext struct {
	Config          config.Config
	Marketclient    market.MarketClient
	DB              *gorm.DB
	TradeOrderModel solmodel.TradeOrderModel
	TxMananger      *solana.TxManager
}

func NewServiceContext(c config.Config) *ServiceContext {
	marketClient := market.NewMarketClient(zrpc.MustNewClient(c.MarketService).Conn())

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

	return &ServiceContext{
		Config:          c,
		Marketclient:    marketClient,
		DB:              db,
		TradeOrderModel: solmodel.NewTradeOrderModel(db),
		TxMananger:      solana.NewTxManager(db, c.Helius.NodeUrl[0], c.SimulateOnly),
	}
}

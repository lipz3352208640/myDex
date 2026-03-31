package svc

import (
	"fmt"
	"log"
	"myDex/model/solmodel"
	"myDex/myConsumer/internal/config"
	"net/http"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config     config.Config
	solClients []*client.Client
	BlockModel solmodel.BlockModel
}

func NewServiceContext(c config.Config) *ServiceContext {
	var solClients []*client.Client
	for _, node := range c.Helius.NodeUrl {
		c := client.New(rpc.WithEndpoint(node), rpc.WithHTTPClient(&http.Client{
			Timeout: 10 * time.Second,
		}))
		solClients = append(solClients, c)
	}
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
		Config:     c,
		solClients: solClients,
		BlockModel: solmodel.NewBlockModel(db),
	}
}

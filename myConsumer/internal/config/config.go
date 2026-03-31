package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	Helius Entity       `json:"Helius,optional"`
	Thread ThreadEntity `json:"thread"`
	Mysql  MysqlConfig  `json:"mysql"`
	Sol    SolEntity    `json:"sol"`
}

type Entity struct {
	NodeUrl []string `json:"NodeUrl"`
	WSUrl   string   `json:"WSUrl,optional" json:",env=SOL_WSURL"`
}

type ThreadEntity struct {
	Count struct {
		Consumer            int `json:"consumer"`
		ConsumerFailedBlock int `json:"consumerFailedBlock"`
	} `json:"count"`
}

type MysqlConfig struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Dbname   string `json:"dbname"`
}

type SolEntity struct {
	StartBlock int `json:"startBlock"`
}

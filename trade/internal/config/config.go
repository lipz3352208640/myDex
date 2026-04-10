package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	MarketService zrpc.RpcClientConf `json:"market_service"`
	Mysql         MysqlConfig        `json:"mysql"`
	Helius        Entity             `json:"Helius,optional"`
}

type MysqlConfig struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Dbname   string `json:"dbname"`
}

type Entity struct {
	NodeUrl []string `json:"NodeUrl"`
	WSUrl   string   `json:"WSUrl,optional" json:",env=SOL_WSURL"`
}

package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	Mysql MysqlConfig `json:"mysql"`
}

type Entity struct {
	NodeUrl []string `json:"NodeUrl"`
	WSUrl   string   `json:"WSUrl,optional" json:",env=SOL_WSURL"`
}

type MysqlConfig struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Dbname   string `json:"dbname"`
}

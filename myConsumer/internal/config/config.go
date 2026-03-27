package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	Helius Entity `json:"Helius,optional"`
	Thread ThreadEntity `json:"thread"`
}

type Entity struct {
	WSUrl string `json:"WSUrl,optional" json:",env=SOL_WSURL"`
}

type ThreadEntity struct {
	Count struct {
		Consumer int `json:"consumer"`
	} `json:"count"`
}

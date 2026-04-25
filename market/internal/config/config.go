package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	Mysql MysqlConfig `json:"mysql"`
	Kafka KafkaConfig `json:"kafka,optional"`
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

type KafkaConfig struct {
	Brokers              []string `json:"brokers,optional"`
	Group                string   `json:"group,optional"`
	Topic                string   `json:"topic,optional"`
	Username             string   `json:"username,optional"`
	Password             string   `json:"password,optional"`
	Offset               string   `json:"offset,optional"`
	Consumers            int      `json:"consumers,optional"`
	RebalanceStrategy    string   `json:"rebalanceStrategy,optional"`
	MaxProcessingTimeMs  int      `json:"maxProcessingTimeMs,optional"`
	AutoCommitIntervalMs int      `json:"autoCommitIntervalMs,optional"`
	SessionTimeoutMs     int      `json:"sessionTimeoutMs,optional"`
}

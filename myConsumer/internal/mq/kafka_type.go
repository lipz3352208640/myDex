package mq

type KqConf struct {
	Brokers  []string `json:",env=KAFKA_BROKERS"`
	Group    string   `json:",env=KAFKA_GROUP"`
	CaFile   string   `json:",optional,env=KAFKA_CAFILE"`
	Username string   `json:",optional,env=KAFKA_USERNAME"`
	Password string   `json:",optional,env=KAFKA_PASSWORD"`
	Topic    string   `json:",env=KAFKA_TOPIC"`
}

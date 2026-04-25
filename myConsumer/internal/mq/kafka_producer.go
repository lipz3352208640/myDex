package mq

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/IBM/sarama"
	"github.com/zeromicro/go-zero/core/logx"
)

var defaultKafka *KafkaProducer

type KafkaProducer struct {
	client sarama.SyncProducer
}

type accessLogEntry struct {
	encoded []byte
}

func (ale *accessLogEntry) Length() int {
	return len(ale.encoded)
}

func (ale *accessLogEntry) Encode() ([]byte, error) {
	return ale.encoded, nil
}

func NewKafka(kqConf KqConf) (*KafkaProducer, error) {
	if len(kqConf.Brokers) == 0 {
		return nil, errors.New("kafka brokers is empty")
	}

	fmt.Println("kafka is initializing")
	client, err := newAccessLogProducer(kqConf.Brokers, kqConf.CaFile, kqConf.Username, kqConf.Password)
	if err != nil {
		logx.Errorf("kafka start error: %v", err)
		return nil, err
	}

	return &KafkaProducer{client: client}, nil
}

func SetDefaultKafka(producer *KafkaProducer) {
	defaultKafka = producer
}

func (k *KafkaProducer) Raw() sarama.SyncProducer {
	if k == nil {
		return nil
	}
	return k.client
}

func (k *KafkaProducer) Close() error {
	if k == nil || k.client == nil {
		return nil
	}
	return k.client.Close()
}

func (k *KafkaProducer) SendMessage(topic string, key string, data []byte) error {
	if k == nil || k.client == nil {
		return errors.New("kafka producer is nil")
	}

	message := &sarama.ProducerMessage{
		Topic: topic,
		Key:   &accessLogEntry{encoded: []byte(key)},
		Value: &accessLogEntry{encoded: data},
	}

	partition, offset, err := k.client.SendMessage(message)
	//设置的是，自动重试，返回err，就会自动重试
	if err != nil {
		logx.Errorf("[kafka] send event log to kafka failed: error:%v", err)
		return err
	}
	logx.Infof("[kafka] send event log to kafka success: %v:%v:%v, %v, len(data): %v",
		topic, partition, offset, key, len(data))
	//返回nil, 发送成功
	return nil
}

func SendEventLogKafkaInfoMessage(topic string, key string, data []byte) error {
	if defaultKafka == nil {
		return errors.New("default kafka producer is nil")
	}
	return defaultKafka.SendMessage(topic, key, data)
}

func newAccessLogProducer(brokers []string, _, username, password string) (sarama.SyncProducer, error) {
	config := sarama.NewConfig()

	// 网络配置 DialTimeout：握手时间 ReadTimeout：读超时 WriteTimeout：写超时 KeepAlive：存活时间
	config.Net.DialTimeout = 30 * time.Second
	config.Net.ReadTimeout = 30 * time.Second
	config.Net.WriteTimeout = 30 * time.Second
	config.Net.KeepAlive = 30 * time.Second

	//生产者配置  Timeout：生产者等待broker的确认超时时间 MaxMessageBytes：单挑最大消息字节 Partitioner：分区器
	sarama.Logger = log.New(os.Stdout, "[sarama] ", log.LstdFlags)
	config.Producer.Timeout = time.Second              // Producer timeout
	config.Producer.MaxMessageBytes = 1024 * 1024 * 10 // Max message size: 10MB
	config.Producer.Partitioner = sarama.NewHashPartitioner
	config.Metadata.AllowAutoTopicCreation = true

	// Retry configurations  Backoff: 退避时间 Errors: 错误返回 Successes: 成功返回
	config.Producer.Retry.Max = 3
	config.Producer.Retry.Backoff = 100 * time.Millisecond
	config.Producer.Return.Errors = true
	config.Producer.Return.Successes = true

	// TLS.Enable = false 不走TLS加密连接，走明文tcp连接 , TLS.Enable = true  &tls.Config才生效
	// InsecureSkipVerify: true 服务端证书校验不通过也先放行
	config.Net.TLS.Enable = false
	config.Net.TLS.Config = &tls.Config{
		InsecureSkipVerify: true, // WARNING: for test only!
	}

	//一个broker 最多允许还没返回的请求个数，对于同步生产者，不需要很大，对于异步生产者越大，并发能力越强
	config.Net.MaxOpenRequests = 1024

	//客户端内部缓冲区，表示客户端内部goroutine能暂存多少消息
	config.ChannelBufferSize = 256

	// SASL configurations 开启SASL 认证  连Kafka 时要带用户名密码
	config.Net.SASL.Enable = true
	// SASL/PLAIN认证机制 -> 用户名+密码方式  tls开启的话，通过tls传输用户名，密码进行加密
	config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
	config.Net.SASL.User = username
	config.Net.SASL.Password = password

	//broker 监控可以看到哪个客户端在连接
	config.ClientID = "producer-dex-consumer"

	// config.Version = sarama.V3_0_0_0

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		logx.Errorf("newAccessLogProducer error: %v", err)
		return nil, err
	}
	// print the producer
	fmt.Println("producer  connect success")

	return producer, nil
}

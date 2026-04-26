package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"myDex/market/internal/config"
	"myDex/market/internal/svc"

	"github.com/IBM/sarama"
	"github.com/zeromicro/go-zero/core/logx"
)

type KafkaTradeConsumer struct {
	index    int
	cfg      config.KafkaConfig
	svcCtx   *svc.ServiceContext
	group    sarama.ConsumerGroup
	ctx      context.Context
	cancel   context.CancelFunc
	errsDone chan struct{}
}

func NewKafkaTradeConsumer(cfg config.KafkaConfig, svcCtx *svc.ServiceContext, index int) (*KafkaTradeConsumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafka brokers is empty")
	}
	if cfg.Topic == "" {
		return nil, errors.New("kafka topic is empty")
	}
	if cfg.Group == "" {
		return nil, errors.New("kafka group is empty")
	}
	if svcCtx == nil || svcCtx.DB == nil {
		return nil, errors.New("svc db is nil")
	}
	if err := ensureKlineTables(svcCtx.DB); err != nil {
		return nil, fmt.Errorf("ensure kline tables: %w", err)
	}

	group, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.Group, newKafkaConsumerGroupConfig(cfg, index))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &KafkaTradeConsumer{
		index:    index,
		cfg:      cfg,
		svcCtx:   svcCtx,
		group:    group,
		ctx:      ctx,
		cancel:   cancel,
		errsDone: make(chan struct{}),
	}, nil
}

func (k *KafkaTradeConsumer) Start() {
	logx.Infof("starting kafka consumer, index=%d, topic=%s, group=%s, brokers=%v", k.index, k.cfg.Topic, k.cfg.Group, k.cfg.Brokers)

	go k.watchErrors()

	go func() {
		handler := &tradeMessageHandler{
			svcCtx: k.svcCtx,
		}

		for {
			//Sarama 进行如下操作
			// 1.该consumer加入到consumer group
			// 2.和broker协调partition的分配
			// 3.分配到partition claim
			// 4.对每个claim调用ConsumeClaim
			if err := k.group.Consume(k.ctx, []string{k.cfg.Topic}, handler); err != nil {
				//如果 上下文被取消，或者 消费失败的话，重试
				if errors.Is(err, context.Canceled) || k.ctx.Err() != nil {
					return
				}
				logx.Errorf("kafka consume failed: %v", err)
				time.Sleep(time.Second)
			}

			if k.ctx.Err() != nil {
				return
			}
		}
	}()
}

func (k *KafkaTradeConsumer) Stop() {
	k.cancel()
	if err := k.group.Close(); err != nil {
		logx.Errorf("close kafka consumer group failed: %v", err)
	}
	//阻塞等待，处理错误消息的协程处理完任务
	<-k.errsDone
}

// 监听错误消息
func (k *KafkaTradeConsumer) watchErrors() {
	defer close(k.errsDone)
	//消费者组异步运行产生的错误，如 broker 连接失败， rebanlance 异常， offset 提交失败等
	for err := range k.group.Errors() {
		if err != nil {
			logx.Errorf("kafka consumer group error: %v", err)
		}
	}
}

type tradeMessageHandler struct {
	svcCtx *svc.ServiceContext
}

// 这一轮消费 开始前调用
func (h *tradeMessageHandler) Setup(_ sarama.ConsumerGroupSession) error {
	return nil
}

// 这一轮消费 结束后调用
func (h *tradeMessageHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
	return nil
}

func (h *tradeMessageHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	logx.Infof("consume claim: topic=%s partition=%d", claim.Topic(), claim.Partition())
	for message := range claim.Messages() {
		if err := h.handleMessage(message); err != nil {
			logx.Errorf("handle kafka message failed, topic=%s partition=%d offset=%d err=%v",
				message.Topic, message.Partition, message.Offset, err)
			continue
		}
		//在消息处理后，进行offset提交，避免消息处理失败，offset已经提交，下次消费数据丢失
		session.MarkMessage(message, "")
	}
	return nil
}

func (h *tradeMessageHandler) handleMessage(message *sarama.ConsumerMessage) error {
	logx.Infof("entered handleMessage, topic=%s partition=%d offset=%d", message.Topic, message.Partition, message.Offset)

	var trades []*TradeMessage
	if err := json.Unmarshal(message.Value, &trades); err != nil {
		return fmt.Errorf("unmarshal trade batch: %w", err)
	}
	if len(trades) == 0 {
		return nil
	}

	validTrades := make([]*TradeMessage, 0, len(trades))
	for _, trade := range trades {
		if isValidTrade(trade) {
			validTrades = append(validTrades, trade)
		}
	}
	if len(validTrades) == 0 {
		return nil
	}

	klineMap := GenerateKlines(validTrades)
	klineCount := 0
	for _, klines := range klineMap {
		klineCount += len(klines)
	}


	applied, err := persistTradesAsKlines(context.Background(), h.svcCtx, validTrades)
	if err != nil {
		return fmt.Errorf("persist klines: %w", err)
	}

	first := validTrades[0]
	logPersistResult(len(validTrades), applied)
	logx.Infof("consumed kafka trade batch, topic=%s partition=%d offset=%d total=%d valid=%d pairs=%d klines=%d applied=%d first_pair=%s block_time=%d",
		message.Topic, message.Partition, message.Offset, len(trades), len(validTrades), len(klineMap), klineCount, applied, first.PairAddr, first.BlockTime)
	return nil
}

func newKafkaConsumerGroupConfig(cfg config.KafkaConfig, index int) *sarama.Config {
	saramaCfg := sarama.NewConfig()
	saramaCfg.Version = sarama.V3_0_0_0
	saramaCfg.Consumer.Return.Errors = true
	saramaCfg.Consumer.Offsets.AutoCommit.Enable = true
	saramaCfg.Consumer.Offsets.Initial = sarama.OffsetNewest
	saramaCfg.Net.DialTimeout = 10 * time.Second
	saramaCfg.Net.ReadTimeout = 30 * time.Second
	saramaCfg.Net.WriteTimeout = 30 * time.Second
	saramaCfg.ClientID = fmt.Sprintf("market-consumer-%d", index)

	if strings.EqualFold(cfg.Offset, "first") {
		saramaCfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	}

	// 单条消息最大处理时间窗口： 消息会往consumer 的Messages channel 里写，如果消息处理超过消息超过这个时间，partition的拉取会变慢或者暂停
	if cfg.MaxProcessingTimeMs > 0 {
		saramaCfg.Consumer.MaxProcessingTime = time.Duration(cfg.MaxProcessingTimeMs) * time.Millisecond
	}

	//自动提交offset的周期
	if cfg.AutoCommitIntervalMs > 0 {
		saramaCfg.Consumer.Offsets.AutoCommit.Interval = time.Duration(cfg.AutoCommitIntervalMs) * time.Millisecond
	}

	//会话超时时间
	if cfg.SessionTimeoutMs > 0 {
		saramaCfg.Consumer.Group.Session.Timeout = time.Duration(cfg.SessionTimeoutMs) * time.Millisecond
		heartbeat := saramaCfg.Consumer.Group.Session.Timeout / 3
		if heartbeat < time.Second {
			heartbeat = time.Second
		}
		if heartbeat >= saramaCfg.Consumer.Group.Session.Timeout {
			heartbeat = saramaCfg.Consumer.Group.Session.Timeout / 2
		}
		saramaCfg.Consumer.Group.Heartbeat.Interval = heartbeat
	}

	switch strings.ToLower(cfg.RebalanceStrategy) {
	case "", "range":
		saramaCfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRange()}
	case "roundrobin", "round_robin":
		saramaCfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	case "sticky":
		saramaCfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategySticky()}
	default:
		logx.Infof("unknown rebalance strategy %q, fallback to range", cfg.RebalanceStrategy)
		saramaCfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRange()}
	}

	if cfg.Username != "" || cfg.Password != "" {
		saramaCfg.Net.SASL.Enable = true
		saramaCfg.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		saramaCfg.Net.SASL.User = cfg.Username
		saramaCfg.Net.SASL.Password = cfg.Password
	}

	return saramaCfg
}

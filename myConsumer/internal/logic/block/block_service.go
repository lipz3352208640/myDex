package block

import (
	"context"
	"errors"
	"myConsumer/internal/logic/constant"
	"myConsumer/internal/svc"
	"net/http"
	"strings"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/samber/lo"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
)

type BlockService struct {
	sc  *svc.ServiceContext
	ctx context.Context
	c   *client.Client
	logx.Logger
	cancle   func(err error)
	slotChan chan uint64
}

func NewBlockService(sc *svc.ServiceContext, slotChan chan uint64) *BlockService {
	ctx, cancle := context.WithCancelCause(context.Background())
	rpcURL := strings.Replace(sc.Config.Helius.WSUrl, "wss://", "https://", 1)
	return &BlockService{
		sc:  sc,
		ctx: ctx,
		c: client.New(rpc.WithEndpoint(rpcURL), rpc.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Second,
		})),
		Logger:   logx.WithContext(ctx).WithFields(logx.Field("service", "block")),
		cancle:   cancle,
		slotChan: slotChan,
	}
}

func (b *BlockService) Start() {

	for i := 1; i <= b.sc.Config.Thread.Count.Consumer; i++ {
		workerID := i
		threading.GoSafe(func() {
			b.ConsumeSlot(workerID)
		})
	}
}

func (b *BlockService) ConsumeSlot(workID int) {

	for {
		select {
		case <-b.ctx.Done():
			return
		case slot, ok := <-b.slotChan:
			if !ok {
				return
			}
			b.ParseTransacton(slot, workID)
		}
	}
}

func (b *BlockService) Stop() {
	b.Info("stop block service")
	b.cancle(errors.New("stop block service"))
}

func (b *BlockService) ParseTransacton(slot uint64, workID int) {
	block, err := b.GetSolBlockInfo(slot)
	if err != nil || block == nil {
		b.Errorf("[work-%d] get block info fail, slot=%d err=%v", workID, slot, err)
		return
	}

	b.Infof("[work-%d] get block success, slot=%d blockhash=%s signatures=%d transactions=%d",
		workID, slot, block.Blockhash, len(block.Signatures), len(block.Transactions))

	if len(block.Transactions) == 0 {
		b.Infof("[work-%d] block has no transaction details, slot=%d signatures=%d",
			workID, slot, len(block.Signatures))
		return
	}

	for _, transcation := range block.Transactions {
		select {
		case <-b.ctx.Done():
			return
		default:
		}

		if transcation.Meta.Err != nil {
			logx.Errorf("[work-%d] transaction has error, slot=%d err=%v", workID, slot, transcation.Meta.Err)
			return
		}

		instructions := transcation.Transaction.Message.Instructions

		if len(instructions) > 0 {
			accountKeys := transcation.AccountKeys
			lo.ForEach(instructions, func(instruction types.CompiledInstruction, _ int) {
				programId := accountKeys[instruction.ProgramIDIndex].String()
				switch programId {
				case constant.PumpAddress:
					b.Infof("[work-%d] transaction is pumpfun program, slot=%d signatures=%d",
						workID, slot, len(block.Signatures))
					pumpData, err := ParsePumpInstruction(transcation.Transaction.Message.Header, accountKeys, instruction)
					if err != nil {
						b.Errorf("[work-%d] parse pump instruction fail, slot=%d err=%v", workID, slot, err)
						return
					}
					b.Infof("[work-%d] parsed pump data: %s", workID, pumpData)
				default:
					return
				}
			})

		}
	}
}

func (b *BlockService) GetSolBlockInfo(slot uint64) (*client.Block, error) {
	const maxRetries = 10
	var retryCount int = 0

	var (
		resp *client.Block
		err  error
	)

	for {
		resp, err = b.c.GetBlockWithConfig(b.ctx, slot, client.GetBlockConfig{
			Commitment: rpc.CommitmentConfirmed,
			//返回完整的交易明细
			TransactionDetails: rpc.GetBlockConfigTransactionDetailsFull,
		})
		if isRetryableGetBlockErr(err) {
			retryCount++
			if retryCount > maxRetries {
				return nil, err
			}

			//保证卡在等待重试的work立即结束
			select {
			case <-b.ctx.Done():
				return nil, b.ctx.Err()
			case <-time.After(time.Second):
			}
		} else if err != nil {
			//非rpc异常或者请求限制等，直接返回
			return nil, err
		} else {
			return resp, nil
		}
	}

}

func isRetryableGetBlockErr(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	//rpc数据没准备好或者限流
	return strings.Contains(msg, "block not available for slot") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "429")
}

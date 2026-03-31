package block

import (
	"context"
	"errors"
	"fmt"
	"myDex/model/solmodel"
	"myDex/myConsumer/internal/svc"
	"myDex/pkg/constant"
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
	name           string
	cancle         func(err error)
	slotChan       chan uint64
	goroutineCount int
}

func NewBlockService(sc *svc.ServiceContext, slotChan chan uint64, name string, count int) *BlockService {
	ctx, cancle := context.WithCancelCause(context.Background())
	rpcURL := strings.Replace(sc.Config.Helius.WSUrl, "wss://", "https://", 1)
	return &BlockService{
		sc:  sc,
		ctx: ctx,
		c: client.New(rpc.WithEndpoint(rpcURL), rpc.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Second,
		})),
		Logger:         logx.WithContext(ctx).WithFields(logx.Field("service", fmt.Sprintf("block-%s", name))),
		name:           name,
		cancle:         cancle,
		slotChan:       slotChan,
		goroutineCount: count,
	}
}

func (b *BlockService) Start() {

	for i := 1; i <= b.goroutineCount; i++ {
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
			b.handleTransacton(slot, workID)
		}
	}
}

func (b *BlockService) Stop() {
	b.Info("stop block service")
	b.cancle(errors.New("stop block service"))
}

func (b *BlockService) handleTransacton(slot uint64, workID int) {

	if slot == 0 {
		return
	}

	//判断该slot是否已入库
	dbBlock, err := b.sc.BlockModel.FindOneBySlot(b.ctx, int64(slot))
	if err != nil {
		if !errors.Is(err, solmodel.ErrNotFound) {
			b.Errorf("[work-%d] query block by slot fail, slot=%d err=%v", workID, slot, err)
			return
		}
		//如果是未找到错误，说明数据库里没有这个slot的记录，继续往下走获取区块信息并入库
		dbBlock = &solmodel.Block{
			Slot: int64(slot),
		}
	}

	block, err := b.GetSolBlockInfo(slot)
	if err != nil || block == nil {
		if strings.Contains(strings.ToLower(err.Error()), "was skipped") {
			dbBlock.Status = constant.BlockSkipped
			b.Infof("getSolBlockInfo by slot:%d was skipped, err:%s", slot, err.Error())
			b.sc.BlockModel.Insert(b.ctx, dbBlock)
			return
		}
		dbBlock.Status = constant.BlockFailed

		if block != nil && block.BlockTime != nil {
			fmt.Println("输出的时间：", *block.BlockTime)
			dbBlock.BlockTime = *block.BlockTime
		} else {
			dbBlock.BlockTime = time.Now()
			dbBlock.ErrMessage = err.Error()
		}
		b.sc.BlockModel.Insert(b.ctx, dbBlock)
		return
	}

	b.Infof("[work-%d] get block success, slot=%d blockhash=%s signatures=%d transactions=%d",
		workID, slot, block.Blockhash, len(block.Signatures), len(block.Transactions))

	if len(block.Transactions) == 0 {
		b.Infof("[work-%d] block has no transaction details, slot=%d signatures=%d",
			workID, slot, len(block.Signatures))
		return
	}

	if block.BlockHeight != nil {
		dbBlock.BlockHeight = *block.BlockHeight
	}

	if block.BlockTime != nil {
		dbBlock.BlockTime = *block.BlockTime
	} else {
		dbBlock.BlockTime = time.Now()
	}

	//入库
	dbBlock.Status = constant.BlockProcessed
	b.sc.BlockModel.Insert(b.ctx, dbBlock)

	//解析交易指令
	b.parseTxInstruction(block, workID, slot)

}

func (b *BlockService) parseTxInstruction(block *client.Block, workID int, slot uint64) {
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
			Commitment: rpc.CommitmentFinalized,
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

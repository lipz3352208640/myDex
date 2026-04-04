package block

import (
	"context"
	"errors"
	"fmt"
	"myDex/model/solmodel"
	"myDex/myConsumer/internal/logic/entity"
	"myDex/myConsumer/internal/svc"
	"myDex/pkg/constant"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/mr-tron/base58"
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
	pumpService    *PumpFunService
}

func NewBlockService(sc *svc.ServiceContext, slotChan chan uint64, name string, count int) *BlockService {
	ctx, cancle := context.WithCancelCause(context.Background())
	rpcURL := strings.Replace(sc.Config.Helius.WSUrl, "wss://", "https://", 1)
	return &BlockService{
		sc:  sc,
		ctx: ctx,
		c: client.New(rpc.WithEndpoint(rpcURL), rpc.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
				DialContext: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).DialContext,
			},
		})),
		Logger:         logx.WithContext(ctx).WithFields(logx.Field("service", fmt.Sprintf("block-%s", name))),
		name:           name,
		cancle:         cancle,
		slotChan:       slotChan,
		goroutineCount: count,
		pumpService:    NewPumpFunService(),
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
			if block != nil && block.BlockTime != nil {
				dbBlock.BlockTime = *block.BlockTime
			} else {
				dbBlock.BlockTime = time.Now()
			}
			dbBlock.ErrMessage = err.Error()
			b.Infof("getSolBlockInfo by slot:%d was skipped, err:%s", slot, err.Error())
			if err := b.saveOrUpdateSlot(dbBlock); err != nil {
				b.Errorf("[work-%d] insert or update skipped block fail, slot=%d err=%v", workID, slot, err)
			}
			return
		}
		dbBlock.Status = constant.BlockFailed

		if block != nil && block.BlockTime != nil {
			dbBlock.BlockTime = *block.BlockTime
		} else {
			dbBlock.BlockTime = time.Now()
		}
		dbBlock.ErrMessage = err.Error()
		if err := b.saveOrUpdateSlot(dbBlock); err != nil {
			b.Errorf("[work-%d] insert or update failed block fail, slot=%d err=%v", workID, slot, err)
		}
		return
	}

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
	if insertErr := b.saveOrUpdateSlot(dbBlock); insertErr != nil {
		b.Errorf("[work-%d] insert or update processed block fail, slot=%d err=%v", workID, slot, insertErr)
		return
	}

	//获取交易价格
	tokenAccountMap, price := GetSolPrice(block, dbBlock, b)

	if price <= 0 {
		return
	}

	fmt.Println("SOL 价格：", price)

	txDecode := &entity.TxDecodeEntity{
		Price:           price,
		TokenAccountMap: tokenAccountMap,
		Block:           dbBlock,
		Slot:            slot,
	}

	//解析交易指令
	b.parseTxInstruction(block, txDecode, workID)

}

func (b *BlockService) saveOrUpdateSlot(block *solmodel.Block) error {
	var err error
	if block.Id != 0 {
		err = b.sc.BlockModel.Update(b.ctx, block)
	} else {
		err = b.sc.BlockModel.Insert(b.ctx, block)
	}
	return err
}

func (b *BlockService) parseTxInstruction(block *client.Block, tx *entity.TxDecodeEntity, workID int) {
	for _, transcation := range block.Transactions {
		select {
		case <-b.ctx.Done():
			return
		default:
		}

		if transcation.Meta == nil || transcation.Meta.Err != nil {
			logx.Errorf("[work-%d] transaction has error, slot=%d err=%v", workID, tx.Slot, transcation.Meta.Err)
			return
		}

		instructions := transcation.Transaction.Message.Instructions

		if len(transcation.Transaction.Signatures) == 0 {
			logx.Errorf("[work-%d] transaction signature is entity, slot=%d err=%v", workID, tx.Slot, transcation.Meta.Err)
			return
		}

		tx.Signature = base58.Encode(transcation.Transaction.Signatures[0])
		tx.TranscationMeta = transcation.Meta
		tx.PumpEventIndex = 0

		if len(instructions) > 0 {
			accountKeys := transcation.AccountKeys
			tx.AccountKeys = accountKeys
			lo.ForEach(instructions, func(instruction types.CompiledInstruction, _ int) {
				programId := accountKeys[instruction.ProgramIDIndex].String()
				switch programId {
				case constant.PumpAddress:
					b.Infof("[work-%d] transaction is pumpfun program, slot=%d signatures=%d",
						workID, tx.Slot, len(block.Signatures))

					tx.Instruction = &instruction
					price := b.pumpService.DecodePumpTranscation(tx)
					//pumpData, err := b.ParsePumpInstruction(transcation.Transaction.Message.Header, accountKeys, instruction)
					if price == 0 {
						b.Errorf("[work-%d] parse pump instruction token price, slot=%d", workID, tx.Slot)
						return
					}
					//b.Infof("[work-%d] parsed pump data: %s", workID, pumpData)
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

			// switch slot % 3 {
			// case 0:
			// 	return resp, nil
			// case 1:
			// 	return resp, errors.New("was skipped")
			// default:
			// 	return resp, errors.New("custom error")
			// }
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

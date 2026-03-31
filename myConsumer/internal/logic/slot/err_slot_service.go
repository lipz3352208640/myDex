package slot

import (
	"errors"
	"myDex/model/solmodel"
	"myDex/myConsumer/internal/svc"
	"time"

	"github.com/zeromicro/go-zero/core/threading"
)

type ErrSlotService struct {
	*SlotService
}

func NewErrSlotService(sc *svc.ServiceContext, errSlotChan chan uint64, name string) *ErrSlotService {
	return &ErrSlotService{
		SlotService: NewSlotService(sc, errSlotChan, name),
	}
}

func (e *ErrSlotService) Start() {
	threading.GoSafe(func() {
		e.HandleSlotNotCompleted()
	})
}

func (e *ErrSlotService) Stop() {
	e.Info("errSlotService service close")
	e.cancle(errors.New("errSlotService service stop"))
}

func (e *ErrSlotService) HandleSlotNotCompleted() {
	startBlock := e.ctx.Config.Sol.StartBlock
	//获取第一个失败的区块
	if startBlock == 0 {
		block, err := e.ctx.BlockModel.GetFirstFailedSlot(e.context)
		if block == nil || err != nil {
			return
		} else {
			startBlock = int(block.Slot)
		}
	}
	//扫描计时器。每5s扫描一次库中失败记录
	scanTimer := time.NewTicker(5 * time.Second).C
	//发送计时器。每1s将扫描中的数据发送到失败队列中
	sendTimer := time.NewTicker(1 * time.Second).C

	for {
		select {
		case <-e.context.Done():
			return
		case <-scanTimer:
			blocks, err := e.ctx.BlockModel.GetBatchFailedBlockBySlot(e.context, int64(startBlock-100), 50)
			if len(blocks) == 0 || errors.Is(err, solmodel.ErrNotFound) {
				return
			}
			if err != nil {
				e.Errorf("process GetBatchFailedBlockBySlot faild is %w", err)
			}

			for _, block := range blocks {
				select {
				case <-e.context.Done():
					return
				case <-sendTimer:
					e.slotChan <- uint64(block.Slot)
				}
			}
		}
	}

}

func (e *ErrSlotService) End() {
	e.Info("errSlotService service close")
	e.cancle(errors.New("errSlotService service stop"))
}

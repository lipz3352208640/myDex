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
	startSlot := int64(e.ctx.Config.Sol.StartBlock)
	if startSlot > 0 {
		startSlot--
	}

	scanTimer := time.NewTicker(5 * time.Second)
	defer scanTimer.Stop()

	for {
		select {
		case <-e.context.Done():
			return
		case <-scanTimer.C:
			blocks, err := e.ctx.BlockModel.GetBatchFailedBlockBySlot(e.context, startSlot, 50)
			if err != nil && !errors.Is(err, solmodel.ErrNotFound) {
				e.Errorf("process GetBatchFailedBlockBySlot faild is %w", err)
				continue
			}
			if len(blocks) == 0 {
				continue
			}

			for _, block := range blocks {
				select {
				case <-e.context.Done():
					return
				case e.slotChan <- uint64(block.Slot):
					e.Infof("send failed slot:%d", block.Slot)
					startSlot = block.Slot
				case <-time.After(time.Second):
					e.Infof("send failed slot timeout:%d", block.Slot)
					continue
				}
			}

			if len(blocks) < 50 {
				startSlot = int64(e.ctx.Config.Sol.StartBlock)
				if startSlot > 0 {
					startSlot--
				}
			}
		}
	}

}

func (e *ErrSlotService) End() {
	e.Info("errSlotService service close")
	e.cancle(errors.New("errSlotService service stop"))
}

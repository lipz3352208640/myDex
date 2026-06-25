package slot

import "myDex/myConsumer/internal/svc"

type SlotGroupService struct {
	slotService    *SlotService
	errSlotService *ErrSlotService
}

func NewSlotGroupService(svc *svc.ServiceContext,
	slotChannel chan uint64,
	errSlotChannel chan uint64) *SlotGroupService {
	return &SlotGroupService{
		slotService:    NewSlotService(svc, slotChannel, "slotService"),
		errSlotService: NewErrSlotService(svc, errSlotChannel, "errSlotService"),
	}
}

func (group *SlotGroupService) Start() {
	group.errSlotService.Start()
	group.slotService.Start()
}

func (group *SlotGroupService) Stop() {
	group.errSlotService.Stop()
	group.slotService.Stop()
}

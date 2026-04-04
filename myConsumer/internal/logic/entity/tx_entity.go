package entity

import (
	"myDex/model/solmodel"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
)

type TxDecodeEntity struct {
	Price           float64
	TokenAccountMap map[string]*TokenAccount
	TranscationMeta *client.TransactionMeta
	Instruction     *types.CompiledInstruction
	Signature       string
	Block           *solmodel.Block
	Slot            uint64
	PumpEventIndex  int
	AccountKeys     []common.PublicKey
	PumpEvents      []*PumpEvent
}

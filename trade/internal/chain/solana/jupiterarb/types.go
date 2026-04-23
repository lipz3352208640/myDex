package jupiterarb

import (
	"context"

	"myDex/trade/internal/chain/solana/arbitrage"

	"github.com/gagliardetto/solana-go"
)

type SwapInstructionAccount struct {
	Pubkey     string `json:"pubkey"`
	IsSigner   bool   `json:"isSigner"`
	IsWritable bool   `json:"isWritable"`
}

type EncodedInstruction struct {
	ProgramID string                   `json:"programId"`
	Accounts  []SwapInstructionAccount `json:"accounts"`
	Data      string                   `json:"data"`
}

type SwapInstructionsResponse struct {
	TokenLedgerInstruction      *EncodedInstruction  `json:"tokenLedgerInstruction"`
	ComputeBudgetInstructions   []EncodedInstruction `json:"computeBudgetInstructions"`
	SetupInstructions           []EncodedInstruction `json:"setupInstructions"`
	SwapInstruction             EncodedInstruction   `json:"swapInstruction"`
	CleanupInstruction          *EncodedInstruction  `json:"cleanupInstruction"`
	AddressLookupTableAddresses []string             `json:"addressLookupTableAddresses"`
	ComputeUnitLimit            uint32               `json:"computeUnitLimit"`
	PrioritizationFeeLamports   uint64               `json:"prioritizationFeeLamports"`
}

type SwapRequest struct {
	UserPublicKey                 solana.PublicKey
	WrapAndUnwrapSol              bool
	UseSharedAccounts             bool
	ComputeUnitPriceMicroLamports uint64
	DynamicComputeUnitLimit       bool
	SkipUserAccountsRPCCalls      bool
	QuoteResponse                 *arbitrage.QuoteResponse
}

type SwapInstructionProvider interface {
	GetSwapInstructions(ctx context.Context, req *SwapRequest) (*SwapInstructionsResponse, error)
}

type BuildRequest struct {
	Payer            solana.PublicKey
	Opportunity      *arbitrage.Opportunity
	JitoTipRecipient *solana.PublicKey
}

type AtomicSwapBuildResult struct {
	Instructions      []solana.Instruction
	LookupTableKeys   []solana.PublicKey
	ComputeUnitLimit  uint32
	ExpectedProfit    int64
	JitoTipLamports   uint64
	MergedQuote       *arbitrage.QuoteResponse
	InstructionBundle *SwapInstructionsResponse
}

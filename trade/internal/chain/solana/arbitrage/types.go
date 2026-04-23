package arbitrage

import "github.com/gagliardetto/solana-go"

type QuoteRequest struct {
	InputMint            solana.PublicKey //WSOL
	OutputMint           solana.PublicKey //WSPL
	Amount               uint64           //输入的WSOL数量
	OnlyDirectRoutes     bool
	RestrictIntermediate bool
	SlippageBps          uint32           //滑点
	MaxAccounts          uint32
}

type RoutePlanStep struct {
	Raw map[string]any
}

type QuoteResponse struct {
	InputMint            solana.PublicKey
	InAmount             uint64
	OutputMint           solana.PublicKey
	OutAmount            uint64
	OtherAmountThreshold uint64
	PriceImpactPct       string
	ContextSlot          uint64
	RoutePlan            []RoutePlanStep
	Raw                  map[string]any
}

type Opportunity struct {
	FirstQuote          *QuoteResponse
	SecondQuote         *QuoteResponse
	InputAmount         uint64
	FinalOutputAmount   uint64
	GrossProfitLamports int64
	SuggestedJitoTip    uint64
	TargetOutAmount     uint64
	ThresholdLamports   int64
	ShouldExecute       bool
}

type QuoteProvider interface {
	GetQuote(req *QuoteRequest) (*QuoteResponse, error)
}

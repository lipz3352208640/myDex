package entity


type BuyInstruction struct {
	ProgranId string
	Accounts  PumpBuyAccount
	Data      PumpBuyData
}
type SellInstruction struct {
	ProgranId string
	Accounts  PumpSellAccount
	Data      PumpSellData
}

type PumpBuyAccount struct {
	Pool Account
	User Account
	GlobalConfig Account
	BaseMint Account
	QuoteMint Account
	UserBaseTokenAccount Account
	UserQuoteTokenAccount Account
	PoolBaseTokenAccount Account
	PoolQuoteTokenAccount Account
	ProtocolFeeRecipient Account
	ProtocolFeeRecipientTokenAccount Account
	BaseTokenProgram Account
	QuoteTokenProgram Account
	SystemProgram Account
	AssociatedTokenProgram Account
	EventAuthority Account
	Program Account
	CoinCreatorVaultAta Account
	CoinCreatorVaultAuthority Account
	GlobalVolumeAccumulator Account
	UserVolumeAccumulator Account
	FeeConfig Account
	FeeProgram Account
}

type PumpSellAccount struct {
	Pool Account
	User Account
	GlobalConfig Account
	BaseMint Account
	QuoteMint Account
	UserBaseTokenAccount Account
	UserQuoteTokenAccount Account
	PoolBaseTokenAccount Account
	PoolQuoteTokenAccount Account
	ProtocolFeeRecipient Account
	ProtocolFeeRecipientTokenAccount Account
	BaseTokenProgram Account
	QuoteTokenProgram Account
	SystemProgram Account
	AssociatedTokenProgram Account
	EventAuthority Account
	Program Account
	CoinCreatorVaultAta Account
	CoinCreatorVaultAuthority Account
	FeeConfig Account
	FeeProgram Account
}

type Account struct {
	Address string
	IsWritable bool
	IsSigner bool
}

type PumpBuyData struct {
	BaseAmountOut uint64
	MaxQuoteAmountIn uint64
	TrackVolume bool
}
type PumpSellData struct {
	BaseAmountOut uint64
	MinQuoteAmountOut uint64
}
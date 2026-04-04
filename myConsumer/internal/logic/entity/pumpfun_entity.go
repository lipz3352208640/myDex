package entity

import "github.com/blocto/solana-go-sdk/common"

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
	Pool                             Account
	User                             Account
	GlobalConfig                     Account
	BaseMint                         Account
	QuoteMint                        Account
	UserBaseTokenAccount             Account
	UserQuoteTokenAccount            Account
	PoolBaseTokenAccount             Account
	PoolQuoteTokenAccount            Account
	ProtocolFeeRecipient             Account
	ProtocolFeeRecipientTokenAccount Account
	BaseTokenProgram                 Account
	QuoteTokenProgram                Account
	SystemProgram                    Account
	AssociatedTokenProgram           Account
	EventAuthority                   Account
	Program                          Account
	CoinCreatorVaultAta              Account
	CoinCreatorVaultAuthority        Account
	GlobalVolumeAccumulator          Account
	UserVolumeAccumulator            Account
	FeeConfig                        Account
	FeeProgram                       Account
}

type PumpSellAccount struct {
	Pool                             Account
	User                             Account
	GlobalConfig                     Account
	BaseMint                         Account
	QuoteMint                        Account
	UserBaseTokenAccount             Account
	UserQuoteTokenAccount            Account
	PoolBaseTokenAccount             Account
	PoolQuoteTokenAccount            Account
	ProtocolFeeRecipient             Account
	ProtocolFeeRecipientTokenAccount Account
	BaseTokenProgram                 Account
	QuoteTokenProgram                Account
	SystemProgram                    Account
	AssociatedTokenProgram           Account
	EventAuthority                   Account
	Program                          Account
	CoinCreatorVaultAta              Account
	CoinCreatorVaultAuthority        Account
	FeeConfig                        Account
	FeeProgram                       Account
}

type Account struct {
	Address    string
	IsWritable bool
	IsSigner   bool
}

type PumpBuyData struct {
	BaseAmountOut    uint64
	MaxQuoteAmountIn uint64
	TrackVolume      bool
}
type PumpSellData struct {
	BaseAmountOut     uint64
	MinQuoteAmountOut uint64
}

type PumpEvent struct {
	Sign                 uint64           //签名地址
	Mint                 common.PublicKey //mint 账户地址
	SolAmount            uint64           //交易的SOL 数量
	TokenAccount         uint64           //交易的token 数量
	IsBuy                bool             //是否买入，卖出
	User                 common.PublicKey //参与交易的用户地址
	VirtualSolReserves   uint64           //虚拟sol储备量
	VirtualTokenReserves uint64           //虚拟Token储备量
}

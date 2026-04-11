package entity

import (
	aSDK "github.com/gagliardetto/solana-go"
)

type MarketTx struct {
	UserId            uint64
	ChainId           uint64
	UserWalletId      uint32
	UserWalletAddress string
	AmountIn          string
	IsAntiMev         bool
	IsAutoSlippage    bool
	Slippage          uint32
	SwapType          int32
	TradePoolName     string
	InDecimal         uint8
	OutDecimal        uint8
	InTokenCa         string
	OutTokenCa        string
	PairAddr          string
	Price             string
	InTokenProgram    string
	OutTokenProgram   string
}

type MarketTxExt struct {
	UserId            uint64
	ChainId           uint64
	UserWalletId      uint32
	UserWalletAddress aSDK.PublicKey
	AmountIn          string
	IsAntiMev         bool
	IsAutoSlippage    bool
	Slippage          uint32
	SwapType          int32
	TradePoolName     string
	InDecimal         uint8
	OutDecimal        uint8
	InMint            aSDK.PublicKey
	OutMint           aSDK.PublicKey
	PairAddr          string
	Price             string
	InTokenProgram    aSDK.PublicKey
	OutTokenProgram   aSDK.PublicKey
}

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
	PairAddr          aSDK.PublicKey
	Price             string
	InTokenProgram    aSDK.PublicKey
	OutTokenProgram   aSDK.PublicKey
}

type TokenSwapPoolConfig struct {
	RPCURL          string `json:"rpcUrl"`
	Payer           string `json:"payer"`
	UsdcMint        string `json:"usdcMint"`
	UsdcSource      string `json:"usdcSource"`
	SwapAccount     string `json:"swapAccount"`
	SwapAuthority   string `json:"swapAuthority"`
	WsolVault       string `json:"wsolVault"`
	UsdcVault       string `json:"usdcVault"`
	PoolMint        string `json:"poolMint"`
	PoolFeeAccount  string `json:"poolFeeAccount"`
	PoolDestination string `json:"poolDestination"`
	SolLamports     uint64 `json:"solLamports"`
	UsdcAmount      uint64 `json:"usdcAmount"`
	UsdcDecimals    uint8  `json:"usdcDecimals"`
	PoolDecimals    uint8  `json:"poolDecimals"`
}

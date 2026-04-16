package constant

import "github.com/shopspring/decimal"

const (
	FeeReceiver = "77r1L6TyggUhwFkk3wFrMMkYS7xK6xJu78wuzMjr2PHZ"

	RaydiumV4SwapCU   = 150_000
	PumpFunSwapCU     = 300_000
	RaydiumClmmSwapCu = 250_000
	RaydiumCpmmSwapCu = 150_000
	PumpSwapCU        = 150_000

	GasPerSignature    = 5_000
	GasPerSignatureSol = 0.000005

	TransferSolCU                 = 500   //refer to https://solscan.io/tx/fiAwZ5G3tgDjuKZGztzn8NY8tU34biVjuTvHidnX9S8AWTGfmCabaKeP1ioARZkc13NoDjmR7pcdEpDrShGRkt4
	TransferTokenCU               = 36118 //refer to https://solscan.io/tx/3Jy3X2bguDXKF6vBNKd5rBA5d3ncbzQXXDgiG6tb7cFVvoNbh274Qz6ZUnZc8fyTKNr67ARpTGgNRW7Z4Zs7X7LC
	PriorityGasPriceTransferSol   = 10000000
	PriorityGasPriceTransferToken = 500000

	Microlamports = 1e9 * 1e6
	MinDecimal    = 0
	MaxDecimal    = 18

	JitoMaxFee = 0.05 // 0.05 sol

	// TipFee Deprecated
	TipFee               = 0.001
	TipFeeLamport uint64 = 1000000
)

var ServericeFeePercent = decimal.NewFromFloat(0.01)

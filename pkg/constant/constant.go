package constant

const (
	//pump_address：AMM地址
	PumpAddress = "pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA"
	// pumpfun：buy 指令
	PumpBuyInstruction uint64 = 0xeaebda01123d0666
	// pumpfun：sell 指令
	PumpSellInstruction uint64 = 0xad837f01a485e633

	//区块处理状态
	BlockProcessed = 1
	BlockFailed    = 2
	BlockSkipped    = 3
)

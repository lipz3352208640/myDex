package constant

const (
	//pump_address：
	PumpAddress = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"
	// pumpfun：buy 指令
	PumpBuyInstruction uint64 = 0xeaebda01123d0666
	// pumpfun：sell 指令
	PumpSellInstruction uint64 = 0xad837f01a485e633

	//区块处理状态
	BlockProcessed = 1
	BlockFailed    = 2
	BlockSkipped   = 3
)

const (
	Token2022ProgramID = "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"
	TokenProgramID     = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"

	//token mint
	Wsol       = "So11111111111111111111111111111111111111112"
	Usdc       = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	SolDecimal = 9
	//token 虚拟流动量和真实流动量的差值
	TokenReservesDiff = 279900000000000
	//sol 虚拟流动量和真实流动量的差值
	SolReservesDiff = 30000000000
)

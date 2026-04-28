package constant

const (
	//pump_address：
	PumpAddress = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"

	PumpAmmAddress = "pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA"
	// pumpfun：buy 指令
	PumpBuyInstruction uint64 = 0xeaebda01123d0666
	// pumpfun：sell 指令
	PumpSellInstruction uint64 = 0xad837f01a485e633

	// pumpfun：sell 指令
	PumpBuyInstructionTest uint64 = 0x5fcddf9e0874fc38

	//区块处理状态
	BlockProcessed = 1
	BlockFailed    = 2
	BlockSkipped   = 3
)

const (
	Token2022ProgramID = "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"
	TokenProgramID     = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"

	//token mint
	Wsol = "So11111111111111111111111111111111111111112"
	Usdc = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	Usdt = "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB"

	//Usdc       = "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU"
	SolDecimal = 9
	//token 虚拟流动量和真实流动量的差值
	TokenReservesDiff = 279900000000000
	//sol 虚拟流动量和真实流动量的差值
	SolReservesDiff = 30000000000

	//pumpfun 池子初始化的Token数量
	VirtualInitPumpTokenAmount = 1073000191
	//pumpfun 池子初始化的Sol数量
	InitSolTokenAmount = 0.015

	//
	PumpFunName    = "PumpFun"
	PumpFunAmmName = "PumpFun.Amm"
)

const (
	SolChainId            = "100000"
	SolChainIdInt         = 100000
)


type GasType int32

const (
	GasType_GasTypeSpeedInvalid GasType = 0
	GasType_NormalSpeed         GasType = 1
	GasType_FastSpeed           GasType = 2
	GasType_SuperFastSpeed      GasType = 3
)

var GasMODE = map[GasType]uint64{
	0:                      5_000,    // use for test
	GasType_NormalSpeed:    150000,   //0.00015 sol
	GasType_FastSpeed:      4500000,  //0.0045 sol
	GasType_SuperFastSpeed: 15000000, //0.015 sol
}

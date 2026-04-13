package solana

import (
	"context"
	"fmt"
	"myDex/pkg/constant"
	pumpfun "myDex/pkg/pump"
	"myDex/pkg/xcode"
	"time"

	"myDex/trade/internal/chain/solana/entity"
	"myDex/trade/trade"

	aSDK "github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
)

var Decimals2Value = map[uint8]int64{
	0:  1,
	1:  1e1,
	2:  1e2,
	3:  1e3,
	4:  1e4,
	5:  1e5,
	6:  1e6,
	7:  1e7,
	8:  1e8,
	9:  1e9,
	10: 1e10,
	11: 1e11,
	12: 1e12,
	13: 1e13,
	14: 1e14,
	15: 1e15,
	16: 1e16,
	17: 1e17,
	18: 1e18,
}

var (
	AllBpDecimal = decimal.NewFromInt(10000)
	//fee 分母
	FeeRateDenominatorValue = decimal.NewFromInt(1000000)
	RaydiumV4Fee            = uint64(2500)
	//PumpFee                 = uint64(50000)
	PumpFee     = uint64(10000)
	PumpSwapFee = uint64(2500)
)

type PumpFunInstruction interface {
	CreateMarketOrderPumpfun(ctx context.Context, marketTx entity.MarketTxExt) ([]aSDK.Instruction, error)
	CreateGasByGasFee(ctx context.Context, isAntiMev bool, walletAccount aSDK.PublicKey, cuLimit uint32, gasFeeInLamport uint64) ([]aSDK.Instruction, uint64, error)
}

func (tm *TxManager) CreateGasByGasFee(ctx context.Context, isAntiMev bool, walletAccount aSDK.PublicKey, cuLimit uint32, gasFeeInLamport uint64) ([]aSDK.Instruction, uint64, error) {
	var instructionNew aSDK.Instruction
	var instructions []aSDK.Instruction

	//gasFeeInLamport：总的gas费用 GasPerSignature：签名费用  cuLimit：耗费的计算单元
	//gasPriceMicroLamports：每个计算单元实际花费的gas费用
	gasPriceMicroLamports := (gasFeeInLamport - constant.GasPerSignature) * 1e6 / uint64(cuLimit)
	var err error
	if gasPriceMicroLamports != 0 {
		//#1 - Compute Budget: SetComputeUnitPrice
		//添加计算单元价格指令。表示：每消耗一个计算单元，愿意付出的priority fee，出价越高越容易被优先打包

		instructionNew, err = computebudget.NewSetComputeUnitPriceInstruction(gasPriceMicroLamports).ValidateAndBuild()
		if nil != err {
			return nil, 0, err
		}
		instructions = append(instructions, instructionNew)

		// #2 - Compute Budget: SetComputeUnitLimit
		//添加计算单元数量限制指令。表示这笔交易最多允许消耗多少个计算单元
		instructionNew, err = computebudget.NewSetComputeUnitLimitInstruction(cuLimit).ValidateAndBuild()
		if nil != err {
			return nil, 0, err
		}
		instructions = append(instructions, instructionNew)
	}

	tm.Debugf("CreateGasAndJitoByGasPrice, initiator=%s, gasPrice=%d, cuLimit=%d, isAntiMev=%v",
		walletAccount, gasPriceMicroLamports, cuLimit, isAntiMev)

	feeInLamport := gasFeeInLamport

	return instructions, feeInLamport, nil
}

// 创建pumpfun swap相关指令
func (tx *TxManager) CreateMarketOrderPumpfun(ctx context.Context, marketTx *entity.MarketTxExt) ([]aSDK.Instruction, error) {

	var instructions []aSDK.Instruction
	//设置
	lamportCost := tx.rentFee

	wallet := marketTx.UserWalletAddress

	amountDecimal, err := decimal.NewFromString(marketTx.AmountIn)

	if nil != err {
		return nil, err
	}
	amountDecimal = amountDecimal.Mul(decimal.NewFromInt(Decimals2Value[marketTx.InDecimal]))
	//链上的金额用lamport表示，是整型
	amountUint64 := uint64(amountDecimal.IntPart())
	//step 1: 创建Compute Budget指令，设置计算单元价格以及计算单元数量限制
	budgetInstructions, lamportCostFee, err := tx.CreateGasByGasFee(ctx, marketTx.IsAntiMev, marketTx.UserWalletAddress, constant.PumpFunSwapCU, constant.GasMODE[1])
	lamportCost += lamportCostFee
	if nil != err {
		return nil, err
	}
	instructions = append(instructions, budgetInstructions...)

	//判断支付的费用和钱包余额对比
	debugCtx, cancel := context.WithTimeout(context.Background(), 5000000*time.Second)
	defer cancel()
	out, err := tx.Client.GetBalance(debugCtx, wallet, rpc.CommitmentConfirmed)

	if err != nil {
		return nil, fmt.Errorf("GetBalance failed, wallet=%s, err=%w", wallet.String(), err)
	}

	balance := out.Value

	//buy时：直接拿sol买，服务费现收  sell时；服务费是从卖出token（wspl）得到的sol中获取
	if marketTx.SwapType == int32(trade.SwapType_Buy) {
		//计算服务费
		if balance < amountUint64 {
			return nil, xcode.SolBalanceNotEnough
		}

		// 计算服务费，实际购买的sol数量*1%
		serviceFee := uint64(amountDecimal.Mul(constant.ServericeFeePercent).IntPart())
		lamportCost += serviceFee + amountUint64
	}
	if lamportCost > balance {
		return nil, xcode.SolBalanceNotEnough
	}

	//step 2: Associated Token Program: CreateIdempotent。创建mint 对应的ata账户，判断mint是否是sol
	var swapDirection int
	if marketTx.InMint == aSDK.WrappedSol || marketTx.OutMint == aSDK.WrappedSol {
		swapDirection = int(trade.SwapType_Buy)
		tokenMint := marketTx.OutMint
		tokenProgram := marketTx.OutTokenProgram
		if swapDirection == int(trade.SwapType_Sell) {
			tokenMint = marketTx.InMint
			tokenProgram = marketTx.InTokenProgram
		}
		ataOneInst, err := tx.CreateAtaIdempotent(wallet, wallet, tokenMint, tokenProgram)
		if err != nil {
			tx.Errorf("create inMint ATA account err : %v", err)
			return nil, err
		}
		instructions = append(instructions, ataOneInst)
	} else {
		ataOneInst, err := tx.CreateAtaIdempotent(wallet, wallet, marketTx.InMint, marketTx.InTokenProgram)
		if err != nil {
			tx.Errorf("create inMint ATA account err : %v", err)
			return nil, err
		}
		ataTwoInst, err := tx.CreateAtaIdempotent(wallet, wallet, marketTx.OutMint, marketTx.OutTokenProgram)
		if err != nil {
			tx.Errorf("create outMint ATA account err : %v", err)
			return nil, err
		}
		instructions = append(instructions, ataOneInst, ataTwoInst)
	}

	//构建buy和sell指令
	if marketTx.SwapType == int32(trade.SwapType_Buy) {
		tx.Debugf("Creating pumpfun buy instruction, amount: %s, amountUint64: %d", marketTx.AmountIn, amountUint64)
		userVolumeAccumulator, err := pumpfun.FindUserVolumeAccumulatorAddress(marketTx.UserWalletAddress)
		if err != nil {
			tx.Errorf("find user volume accumulator address err : %v", err)
			return nil, err
		}

		needInitUserVolumeAccumulator, err := tx.shouldInitUserVolumeAccumulator(userVolumeAccumulator)
		if err != nil {
			tx.Errorf("check user volume accumulator err : %v", err)
			return nil, err
		}
		if needInitUserVolumeAccumulator {
			tx.Infof("user volume accumulator not found, initializing: %s", userVolumeAccumulator.String())
			instructions = append(instructions, tx.BuildInitUserVolumeAccumulatorInstruction(marketTx.UserWalletAddress, userVolumeAccumulator))
		}

		buyInstruction, err := tx.CreateBuyInstruction(marketTx)
		if err != nil {
			tx.Errorf("create pumpfun buy instruction err : %v", err)
			return nil, err
		}
		instructions = append(instructions, buyInstruction)
	} else if marketTx.SwapType == int32(trade.SwapType_Sell) {
		tx.Debugf("Creating pumpfun sell instruction, amount: %s, amountUint64: %d", marketTx.AmountIn, amountUint64)
		sellInstruction, err := tx.CreateSellInstruction(marketTx)
		if err != nil {
			tx.Errorf("create pumpfun sell instruction err : %v", err)
			return nil, err
		}
		instructions = append(instructions, sellInstruction)
	} else {
		return nil, fmt.Errorf("invalid swap type: %d", marketTx.SwapType)
	}
	return instructions, nil

}

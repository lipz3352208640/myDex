package solana

import (
	"context"
	"myDex/pkg/constant"
	"myDex/pkg/xcode"
	"myDex/trade/internal/logic/entity"
	"myDex/trade/trade"

	aSDK "github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	ata "github.com/gagliardetto/solana-go/programs/associated-token-account"
	"greet/pkg/sol/associatedtoken2022account"
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


type PumpFunInstruction interface {
	CreateMarketOrderPumpfun(ctx context.Context, marketTx entity.MarketTxExt) ([]aSDK.Instruction, error)
	CreateGasByGasFee(ctx context.Context, isAntiMev bool, walletAccount aSDK.PublicKey, cuLimit uint32, gasFeeInLamport uint64) ([]aSDK.Instruction, uint64, error)
	CreateAtaIdempotent(payer,walletAddress,token)
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
func (tx *TxManager) CreateMarketOrderPumpfun(ctx context.Context, marketTx entity.MarketTxExt) ([]aSDK.Instruction, error) {

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
	instructions, lamportCostFee, err := tx.CreateGasByGasFee(ctx, marketTx.IsAntiMev, marketTx.UserWalletAddress, constant.PumpFunSwapCU, constant.GasMODE[1])
	lamportCost += lamportCostFee
	if nil != err {
		return nil, err
	}

	//判断支付的费用和钱包余额对比
	out, err := tx.Client.GetBalance(ctx, wallet, rpc.CommitmentFinalized)
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

	//step 2: Associated Token Program: CreateIdempotent。创建mint 对应的ata账户
	ata.

}

package solana

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"myDex/pkg/constant"
	pumpfun "myDex/pkg/pump"
	"myDex/trade/internal/chain/solana/entity"
	swap_entity "myDex/trade/internal/chain/solana/entity"

	"github.com/gagliardetto/solana-go"
	aSDK "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
)

type Swap interface {
	FetchBondingCurve(rpcClient *rpc.Client, bondingCurvePubKey solana.PublicKey) (*pumpfun.BondingCurveData, error)
	CalcMinAmountOutByPrice(price, amountIn decimal.Decimal, slippage uint32, indecimal, outdecimal uint8, isBuy bool) (int64, int64)
	CreateBuyInstruction(marketTx entity.MarketTxExt) (aSDK.Instruction, error)
	CreateSellInstruction(marketTx entity.MarketTxExt) (aSDK.Instruction, error)
	BuildBuyInstructionWithCreatorVault(buyInstruction *swap_entity.BuyInstruction) aSDK.Instruction
	BuildSellInstructionWithCreatorVault(sellInstruction *swap_entity.SellInstruction) aSDK.Instruction
}

// 减掉池子交易手续费，抛出滑点部分
func (tx *TxManager) CalcMinAmountOutByPrice(price, amountIn decimal.Decimal,
	slippage uint32,
	indecimal, outdecimal uint8,
	isBuy bool) (int64, int64) {
	//费率真实金额
	feeRateDecimal := decimal.NewFromUint64(PumpFee).Div(FeeRateDenominatorValue)
	subAmtDecimal := amountIn.Sub(amountIn.Mul(feeRateDecimal))
	var outAmtDecaiml decimal.Decimal
	var amtdecimal = outdecimal
	if isBuy {
		outAmtDecaiml = subAmtDecimal.Div(price)
	} else {
		outAmtDecaiml = subAmtDecimal.Mul(price)
	}

	outAmt := outAmtDecaiml.Mul(decimal.NewFromInt(Decimals2Value[amtdecimal]))

	//计算滑点
	minOut := outAmt.Mul(AllBpDecimal.Sub(decimal.NewFromUint64(uint64(slippage))).Div(AllBpDecimal))

	return outAmt.IntPart(), minOut.IntPart()
}

func (tx *TxManager) CalcMinAmountInBondingCurve(amountIn decimal.Decimal,
	bondingCurveData *pumpfun.BondingCurveData, slippage uint32) (int64, int64) {

	solAmountBig := big.NewInt(amountIn.IntPart())

	solTotalBig := new(big.Int).Add(solAmountBig, bondingCurveData.VirtualSolReserves)
	totalBig := new(big.Int).Mul(bondingCurveData.VirtualSolReserves, bondingCurveData.VirtualTokenReserves)
	leftTokenAmountBig := new(big.Int).Div(totalBig, solTotalBig)
	buyTokenAmountBig := new(big.Int).Sub(bondingCurveData.RealTokenReserves, leftTokenAmountBig)

	//计算滑点
	slippageRate := AllBpDecimal.Sub(decimal.NewFromUint64(uint64(slippage))).Div(AllBpDecimal)
	minOut := decimal.NewFromBigInt(buyTokenAmountBig, 0).Mul(slippageRate)

	return buyTokenAmountBig.Int64(), minOut.IntPart()
}

func (tx *TxManager) CreateBuyInstruction(marketTx *entity.MarketTxExt) (aSDK.Instruction, error) {

	//step 1 组装pump bonding curve 曲线状态账户以及关联的ata账户
	bondingCurve, err := pumpfun.GetBondingCurveAndAssociatedBondingCurve(marketTx.OutMint)
	if err != nil {
		tx.Errorf("create Bonding Curve err: %v", err)
		return nil, err
	}

	//step 2 获取amount。计算最小输出金额
	price, _ := decimal.NewFromString(marketTx.Price)
	amountIn, _ := decimal.NewFromString(marketTx.AmountIn)
	slippage := marketTx.Slippage
	indecimal := marketTx.InDecimal
	outdecimal := marketTx.OutDecimal
	var isBuy bool = true
	var minOut int64
	if price.IsZero() {
		outAmt, calcMinOut := tx.CalcMinAmountOutByPrice(price, amountIn, slippage, indecimal, outdecimal, isBuy)
		tx.Infof("CreateBuyInstruction: get amout is :%v, minmount is :%v", outAmt, calcMinOut)
		minOut = calcMinOut
	} else {
		bondingCurve, err := pumpfun.FetchBondingCurve(tx.Client, bondingCurve.BondingCurve)
		if err != nil {
			return nil, fmt.Errorf("can't fetch bonding curve: %w", err)
		}

		minAmountOut, minAmountIn := tx.CalcMinAmountInBondingCurve(amountIn, bondingCurve, slippage)
		tx.Infof("CreateBuyInstruction: get minAmountIn is :%v, minAmountOut is :%v", minAmountIn, minAmountOut)
		minOut = minAmountOut
	}

	//step 3 获取ata账户
	ataAccount, _, err := aSDK.FindAssociatedTokenAddress(marketTx.UserWalletAddress, marketTx.OutMint)
	if err != nil {
		tx.Errorf("find associated token address err: %v", err)
		return nil, err
	}

	//step 4 获取creator vault 相关账户
	creatorVault, err := pumpfun.CreateCreatorVault(tx.Client, bondingCurve.BondingCurve)
	if err != nil {
		tx.Errorf("create creator vault err: %v", err)
		return nil, err
	}

	//step 5 获取fee_recipient账户
	feeRecipient, err := pumpfun.GetGlobalFeeRecipient(tx.Client)
	if err != nil {
		tx.Errorf("get fee recipient err: %v", err)
		return nil, err
	}

	//step 6 global_volume_accumulator
	globalVolumeAccumulator, err := pumpfun.FindGlobalVolumeAccumulatorAddress()
	if err != nil {
		tx.Errorf("find global volume accumulator address err: %v", err)
		return nil, err
	}
	//step 7 user_volume_accumulator
	userVolumeAccumulator, err := pumpfun.FindUserVolumeAccumulatorAddress()
	if err != nil {
		tx.Errorf("find user volume accumulator address err: %v", err)
		return nil, err
	}

	//step 8 fee_config
	feeConfig, err := pumpfun.FindFeeConfigAddress()
	if err != nil {
		tx.Errorf("find fee config address err: %v", err)
		return nil, err
	}

	//step 9 构建buy指令
	return tx.BuildBuyInstructionWithCreatorVault(
		&swap_entity.BuyInstruction{
			Amount:                  uint64(minOut),
			MaxSolCost:              uint64(amountIn.IntPart()),
			Global:                  pumpfun.GlobalPumpFunAddress,
			FeeRecipient:            feeRecipient,
			Mint:                    marketTx.OutMint,
			BondingCurve:            bondingCurve.BondingCurve,
			AssociatedBondingCurve:  bondingCurve.AssociatedBondingCurve,
			AssociatedUser:          ataAccount,
			User:                    marketTx.UserWalletAddress,
			SystemProgram:           aSDK.SystemProgramID,
			TokenProgram:            marketTx.OutTokenProgram,
			EventAuthority:          pumpfun.PumpFunEventAuthority,
			CreatorVault:            creatorVault.CreatorVault,
			Program:                 aSDK.MustPublicKeyFromBase58(constant.PumpAddress),
			GlobalVolumeAccumulator: globalVolumeAccumulator,
			UserVolumeAccumulator:   userVolumeAccumulator,
			FeeConfig:               feeConfig}), nil
}

func (tx *TxManager) BuildBuyInstructionWithCreatorVault(buyInstruction *swap_entity.BuyInstruction) aSDK.Instruction {
	// Create accounts slice matching the correct PumpFun order (12 accounts total)
	accounts := []*aSDK.AccountMeta{
		aSDK.Meta(buyInstruction.Global).WRITE(),                  // #0 - Global (WRITABLE)
		aSDK.Meta(buyInstruction.FeeRecipient).WRITE(),            // #1 - Fee Recipient (WRITABLE)
		aSDK.Meta(buyInstruction.Mint).WRITE(),                    // #2 - Mint (WRITABLE)
		aSDK.Meta(buyInstruction.BondingCurve).WRITE(),            // #3 - Bonding Curve (WRITABLE)
		aSDK.Meta(buyInstruction.AssociatedBondingCurve).WRITE(),  // #4 - Associated Bonding Curve (WRITABLE)
		aSDK.Meta(buyInstruction.AssociatedUser).WRITE(),          // #5 - Associated User (WRITABLE)
		aSDK.Meta(buyInstruction.User).WRITE().SIGNER(),           // #6 - User (WRITABLE, SIGNER)
		aSDK.Meta(buyInstruction.SystemProgram),                   // #7 - System Program
		aSDK.Meta(buyInstruction.TokenProgram).WRITE(),            // #8 - Token Program (WRITABLE)
		aSDK.Meta(buyInstruction.CreatorVault).WRITE(),            // #9 - Creator Vault (WRITABLE)
		aSDK.Meta(buyInstruction.EventAuthority).WRITE(),          // #10 - Event Authority (WRITABLE)
		aSDK.Meta(buyInstruction.Program),                         // #11 - Program
		aSDK.Meta(buyInstruction.GlobalVolumeAccumulator).WRITE(), // #12 - Global Volume Accumulator (WRITABLE)
		aSDK.Meta(buyInstruction.UserVolumeAccumulator).WRITE(),   // #13 - User Volume Accumulator (WRITABLE)
		aSDK.Meta(buyInstruction.FeeConfig),                       // #14 - Fee Config
		aSDK.Meta(pumpfun.PumpFeeProgramAddress),                  // #15 - Fee Program
	}

	// Create instruction data
	data := make([]byte, 16) // 8 bytes discriminator + 8 bytes amount

	// Add buy instruction discriminator [102, 6, 61, 18, 1, 218, 235, 234]
	copy(data[0:8], []byte{102, 6, 61, 18, 1, 218, 235, 234})

	// Add amount parameter (u64, little endian)
	binary.LittleEndian.PutUint64(data[8:16], buyInstruction.Amount)

	// Add maxSolCost parameter (u64, little endian)

	data = append(data, make([]byte, 8)...) // Extend data slice to accommodate maxSolCost
	binary.LittleEndian.PutUint64(data[16:24], buyInstruction.MaxSolCost)
	// Add track_volume (bool)
	data = append(data, byte(1))

	return aSDK.NewInstruction(
		buyInstruction.Program,
		accounts,
		data,
	)
}

func (tx *TxManager) CreateSellInstruction(marketTx *entity.MarketTxExt) (aSDK.Instruction, error) {

	//step 1 组装pump bonding curve 曲线状态账户以及关联的ata账户
	bondingCurve, err := pumpfun.GetBondingCurveAndAssociatedBondingCurve(marketTx.OutMint)
	if err != nil {
		tx.Errorf("create Bonding Curve err: %v", err)
		return nil, err
	}

	//step 2 获取amount。计算最小输出金额
	price, _ := decimal.NewFromString(marketTx.Price)
	amountIn, _ := decimal.NewFromString(marketTx.AmountIn)
	slippage := marketTx.Slippage
	indecimal := marketTx.InDecimal
	outdecimal := marketTx.OutDecimal
	var isBuy bool = false
	var minOut int64
	if price.IsZero() {
		outAmt, calcMinOut := tx.CalcMinAmountOutByPrice(price, amountIn, slippage, indecimal, outdecimal, isBuy)
		tx.Infof("CreateBuyInstruction: get amout is :%v, minmount is :%v", outAmt, calcMinOut)
		minOut = calcMinOut
	} else {
		bondingCurve, err := pumpfun.FetchBondingCurve(tx.Client, bondingCurve.BondingCurve)
		if err != nil {
			return nil, fmt.Errorf("can't fetch bonding curve: %w", err)
		}

		minAmountOut, minAmountIn := tx.CalcMinAmountInBondingCurve(amountIn, bondingCurve, slippage)
		tx.Infof("CreateBuyInstruction: get minAmountIn is :%v, minAmountOut is :%v", minAmountIn, minAmountOut)
		minOut = minAmountOut
	}

	//step 3 获取ata账户
	ataAccount, _, err := aSDK.FindAssociatedTokenAddress(marketTx.UserWalletAddress, marketTx.InMint)
	if err != nil {
		tx.Errorf("find associated token address err: %v", err)
		return nil, err
	}

	//step 4 获取creator vault 相关账户
	creatorVault, err := pumpfun.CreateCreatorVault(tx.Client, bondingCurve.BondingCurve)
	if err != nil {
		tx.Errorf("create creator vault err: %v", err)
		return nil, err
	}

	//step 5 获取fee_recipient账户
	feeRecipient, err := pumpfun.GetGlobalFeeRecipient(tx.Client)
	if err != nil {
		tx.Errorf("get fee recipient err: %v", err)
		return nil, err
	}

	//step 6 global_volume_accumulator
	globalVolumeAccumulator, err := pumpfun.FindGlobalVolumeAccumulatorAddress()
	if err != nil {
		tx.Errorf("find global volume accumulator address err: %v", err)
		return nil, err
	}
	//step 7 user_volume_accumulator
	userVolumeAccumulator, err := pumpfun.FindUserVolumeAccumulatorAddress()
	if err != nil {
		tx.Errorf("find user volume accumulator address err: %v", err)
		return nil, err
	}

	//step 8 fee_config
	feeConfig, err := pumpfun.FindFeeConfigAddress()
	if err != nil {
		tx.Errorf("find fee config address err: %v", err)
		return nil, err
	}

	//step 9 构建buy指令
	return tx.BuildBuyInstructionWithCreatorVault(
		&swap_entity.BuyInstruction{
			Amount:                  uint64(minOut),
			MaxSolCost:              uint64(amountIn.IntPart()),
			Global:                  pumpfun.GlobalPumpFunAddress,
			FeeRecipient:            feeRecipient,
			Mint:                    marketTx.OutMint,
			BondingCurve:            bondingCurve.BondingCurve,
			AssociatedBondingCurve:  bondingCurve.AssociatedBondingCurve,
			AssociatedUser:          ataAccount,
			User:                    marketTx.UserWalletAddress,
			SystemProgram:           aSDK.SystemProgramID,
			TokenProgram:            marketTx.OutTokenProgram,
			EventAuthority:          pumpfun.PumpFunEventAuthority,
			CreatorVault:            creatorVault.CreatorVault,
			Program:                 aSDK.MustPublicKeyFromBase58(constant.PumpAddress),
			GlobalVolumeAccumulator: globalVolumeAccumulator,
			UserVolumeAccumulator:   userVolumeAccumulator,
			FeeConfig:               feeConfig}), nil
}

func (tx *TxManager) BuildSellInstructionWithCreatorVault(sellInstruction *swap_entity.SellInstruction) aSDK.Instruction {
	// Create accounts slice matching the correct PumpFun order (12 accounts total)
	accounts := []*aSDK.AccountMeta{
		aSDK.Meta(sellInstruction.Global).WRITE(),                 // #0 - Global (WRITABLE)
		aSDK.Meta(sellInstruction.FeeRecipient).WRITE(),           // #1 - Fee Recipient (WRITABLE)
		aSDK.Meta(sellInstruction.Mint).WRITE(),                   // #2 - Mint (WRITABLE)
		aSDK.Meta(sellInstruction.BondingCurve).WRITE(),           // #3 - Bonding Curve (WRITABLE)
		aSDK.Meta(sellInstruction.AssociatedBondingCurve).WRITE(), // #4 - Associated Bonding Curve (WRITABLE)
		aSDK.Meta(sellInstruction.AssociatedUser).WRITE(),         // #5 - Associated User (WRITABLE)
		aSDK.Meta(sellInstruction.User).WRITE().SIGNER(),          // #6 - User (WRITABLE, SIGNER)
		aSDK.Meta(sellInstruction.SystemProgram),                  // #7 - System Program
		aSDK.Meta(sellInstruction.TokenProgram).WRITE(),           // #8 - Token Program (WRITABLE)
		aSDK.Meta(sellInstruction.CreatorVault).WRITE(),           // #9 - Creator Vault (WRITABLE)
		aSDK.Meta(sellInstruction.EventAuthority).WRITE(),         // #10 - Event Authority (WRITABLE)
		aSDK.Meta(sellInstruction.Program),                        // #11 - Program
		aSDK.Meta(sellInstruction.FeeConfig),                      // #14 - Fee Config
		aSDK.Meta(pumpfun.PumpFeeProgramAddress),                  // #15 - Fee Program
	}

	// Create instruction data
	data := make([]byte, 16) // 8 bytes discriminator + 8 bytes amount

	// Add sell instruction discriminator [51, 230, 133, 164, 1, 127, 131, 173]
	copy(data[0:8], []byte{51, 230, 133, 164, 1, 127, 131, 173})

	// Add amount parameter (u64, little endian)
	binary.LittleEndian.PutUint64(data[8:16], *sellInstruction.Amount)

	// Add MinSolOutput parameter (u64, little endian)

	data = append(data, make([]byte, 8)...) // Extend data slice to accommodate maxSolCost
	binary.LittleEndian.PutUint64(data[16:24], *sellInstruction.MinSolOutput)

	return aSDK.NewInstruction(
		sellInstruction.Program,
		accounts,
		data,
	)
}

package solana

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"myDex/pkg/constant"
	pumpfun "myDex/pkg/pump"
	token2022 "myDex/pkg/token2022"
	"myDex/pkg/xcode"
	"myDex/trade/internal/chain/solana/entity"
	swap_entity "myDex/trade/internal/chain/solana/entity"
	"strings"

	"github.com/gagliardetto/solana-go"
	aSDK "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
)

const pumpBuySafetyBps uint64 = 9950

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

	amountIn = amountIn.Div(decimal.NewFromInt(Decimals2Value[indecimal]))
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
	bondingCurveData *pumpfun.BondingCurveData, slippage uint32) (int64, int64, error) {

	// feeRateDecimal := decimal.NewFromUint64(PumpFee).Div(FeeRateDenominatorValue)
	// subAmtDecimal := amountIn.Sub(amountIn.Mul(feeRateDecimal))

	solAmountBig := big.NewInt(amountIn.IntPart())

	solTotalBig := new(big.Int).Add(solAmountBig, bondingCurveData.VirtualSolReserves)
	totalBig := new(big.Int).Mul(bondingCurveData.VirtualSolReserves, bondingCurveData.VirtualTokenReserves)
	leftTokenAmountBig := new(big.Int).Div(totalBig, solTotalBig)
	buyTokenAmountBig := new(big.Int).Sub(bondingCurveData.VirtualTokenReserves, leftTokenAmountBig)
	if buyTokenAmountBig.Cmp(bondingCurveData.RealTokenReserves) > 0 {
		return 0, 0, xcode.PoolLiquidityNotEnough
	}

	tx.Infof("amountIn=%s", amountIn.String())
	tx.Infof("virtualSol=%s", bondingCurveData.VirtualSolReserves.String())
	tx.Infof("virtualToken=%s", bondingCurveData.VirtualTokenReserves.String())
	tx.Infof("realToken=%s", bondingCurveData.RealTokenReserves.String())
	tx.Infof("buyTokenAmount=%s", buyTokenAmountBig.String())

	//计算滑点
	slippageRate := AllBpDecimal.Sub(decimal.NewFromUint64(uint64(slippage))).Div(AllBpDecimal)
	minOut := decimal.NewFromBigInt(buyTokenAmountBig, 0).Mul(slippageRate)

	return buyTokenAmountBig.Int64(), minOut.IntPart(), nil
}

func ceilDivBigInt(numerator, denominator *big.Int) *big.Int {
	if denominator.Sign() <= 0 {
		return big.NewInt(0)
	}

	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func quoteBuyTokenAmountFromSol(maxSolCost uint64, bondingCurveData *pumpfun.BondingCurveData) (*big.Int, error) {
	if maxSolCost == 0 {
		return nil, fmt.Errorf("maxSolCost is zero")
	}
	if bondingCurveData == nil {
		return nil, fmt.Errorf("bonding curve data is nil")
	}

	grossSol := big.NewInt(0).SetUint64(maxSolCost)
	feeDenominator := big.NewInt(FeeRateDenominatorValue.IntPart())
	feeNumerator := big.NewInt(int64(PumpFee))
	netDenominator := new(big.Int).Sub(feeDenominator, feeNumerator)
	if netDenominator.Sign() <= 0 {
		return nil, fmt.Errorf("invalid net denominator")
	}

	effectiveSol := new(big.Int).Mul(grossSol, netDenominator)
	effectiveSol.Div(effectiveSol, feeDenominator)
	if effectiveSol.Sign() <= 0 {
		return nil, fmt.Errorf("effectiveSol is zero")
	}

	newVirtualSol := new(big.Int).Add(new(big.Int).Set(bondingCurveData.VirtualSolReserves), effectiveSol)
	k := new(big.Int).Mul(new(big.Int).Set(bondingCurveData.VirtualSolReserves), new(big.Int).Set(bondingCurveData.VirtualTokenReserves))
	newVirtualToken := new(big.Int).Div(k, newVirtualSol)
	tokenAmount := new(big.Int).Sub(new(big.Int).Set(bondingCurveData.VirtualTokenReserves), newVirtualToken)
	if tokenAmount.Sign() <= 0 {
		return nil, fmt.Errorf("quoted token amount is zero")
	}
	if tokenAmount.Cmp(bondingCurveData.RealTokenReserves) > 0 {
		return nil, xcode.PoolLiquidityNotEnough
	}

	return tokenAmount, nil
}

func quoteBuySolAmountFromToken(tokenAmount *big.Int, bondingCurveData *pumpfun.BondingCurveData) (uint64, error) {
	if tokenAmount == nil || tokenAmount.Sign() <= 0 {
		return 0, fmt.Errorf("tokenAmount is zero")
	}
	if bondingCurveData == nil {
		return 0, fmt.Errorf("bonding curve data is nil")
	}
	if tokenAmount.Cmp(bondingCurveData.RealTokenReserves) > 0 {
		return 0, xcode.PoolLiquidityNotEnough
	}
	if tokenAmount.Cmp(bondingCurveData.VirtualTokenReserves) >= 0 {
		return 0, fmt.Errorf("tokenAmount exceeds virtual reserves")
	}

	remainingVirtualToken := new(big.Int).Sub(new(big.Int).Set(bondingCurveData.VirtualTokenReserves), tokenAmount)
	k := new(big.Int).Mul(new(big.Int).Set(bondingCurveData.VirtualSolReserves), new(big.Int).Set(bondingCurveData.VirtualTokenReserves))
	requiredVirtualSol := ceilDivBigInt(k, remainingVirtualToken)
	netSol := new(big.Int).Sub(requiredVirtualSol, new(big.Int).Set(bondingCurveData.VirtualSolReserves))
	if netSol.Sign() <= 0 {
		return 0, fmt.Errorf("required net sol is zero")
	}

	feeDenominator := big.NewInt(FeeRateDenominatorValue.IntPart())
	feeNumerator := big.NewInt(int64(PumpFee))
	netDenominator := new(big.Int).Sub(feeDenominator, feeNumerator)
	if netDenominator.Sign() <= 0 {
		return 0, fmt.Errorf("invalid net denominator")
	}

	grossSolNumerator := new(big.Int).Mul(netSol, feeDenominator)
	grossSol := ceilDivBigInt(grossSolNumerator, netDenominator)
	if !grossSol.IsUint64() {
		return 0, fmt.Errorf("required sol exceeds uint64")
	}

	return grossSol.Uint64(), nil
}

func fitBuyTokenAmountByMaxSol(maxSolCost uint64, bondingCurveData *pumpfun.BondingCurveData) (uint64, uint64, error) {
	quotedTokenAmount, err := quoteBuyTokenAmountFromSol(maxSolCost, bondingCurveData)
	if err != nil {
		return 0, 0, err
	}
	if !quotedTokenAmount.IsUint64() {
		return 0, 0, fmt.Errorf("quoted token amount exceeds uint64")
	}

	low := uint64(1)
	high := quotedTokenAmount.Uint64()
	var bestAmount uint64
	var bestRequiredSol uint64

	for low <= high {
		mid := low + (high-low)/2
		requiredSol, err := quoteBuySolAmountFromToken(new(big.Int).SetUint64(mid), bondingCurveData)
		if err != nil {
			return 0, 0, err
		}

		if requiredSol <= maxSolCost {
			bestAmount = mid
			bestRequiredSol = requiredSol
			low = mid + 1
			continue
		}

		if mid == 0 {
			break
		}
		high = mid - 1
	}

	if bestAmount == 0 {
		return 0, 0, fmt.Errorf("no token amount fits maxSolCost")
	}

	return bestAmount, bestRequiredSol, nil
}

func applySafetyBps(amount uint64, safetyBps uint64) uint64 {
	if amount == 0 {
		return 0
	}
	safeAmount := amount * safetyBps / 10000
	if safeAmount == 0 {
		return 1
	}
	return safeAmount
}

func (tx *TxManager) CreateBuyInstruction(marketTx *entity.MarketTxExt) (aSDK.Instruction, error) {

	//step 1 组装pump bonding curve 曲线状态账户以及关联的ata账户

	bondingCurve, err := pumpfun.GetBondingCurveAndAssociatedBondingCurve(marketTx.OutMint, marketTx.OutTokenProgram)
	if err != nil {
		tx.Errorf("create Bonding Curve err: %v", err)
		return nil, err
	}
	fmt.Println("tokenProgram:", marketTx.OutTokenProgram.String())
	fmt.Println("mint:", marketTx.OutMint.String())
	fmt.Println("bondingCurvePDA:", bondingCurve.BondingCurve.String())

	//step 2 获取amount。计算最小输出金额
	price, _ := decimal.NewFromString(marketTx.Price)
	amountIn, _ := decimal.NewFromString(marketTx.AmountIn)
	slippage := marketTx.Slippage
	indecimal := marketTx.InDecimal
	outdecimal := marketTx.OutDecimal

	amountIn = amountIn.Mul(decimal.NewFromInt(Decimals2Value[indecimal]))

	var isBuy bool = true
	var minOut int64
	var bondingCurveData *pumpfun.BondingCurveData
	feeConfig, err := pumpfun.FindFeeConfigAddress()
	if err != nil {
		tx.Errorf("find fee config address err: %v", err)
		return nil, err
	}
	if !price.IsZero() {
		outAmt, calcMinOut := tx.CalcMinAmountOutByPrice(price, amountIn, slippage, indecimal, outdecimal, isBuy)
		tx.Infof("CreateBuyInstruction: get amout is :%v, minmount is :%v", outAmt, calcMinOut)
		minOut = calcMinOut
	} else {
	
		bondingCurveData, err = pumpfun.FetchBondingCurve(tx.Client, bondingCurve.BondingCurve)

		if err != nil {
			return nil, fmt.Errorf("can't fetch bonding curve: %w", err)
		}

		if bondingCurveData.Complete {
			return nil, fmt.Errorf("bonding curve is complete, can't buy")
		}

		tokenAmount, requiredSol, err := tx.fitBuyTokenAmountByMaxSolWithFee(uint64(amountIn.IntPart()), feeConfig, bondingCurveData)
		if err != nil {
			return nil, fmt.Errorf("can't fit token amount by max sol: %w", err)
		}
		slippageRate := AllBpDecimal.Sub(decimal.NewFromUint64(uint64(slippage))).Div(AllBpDecimal)
		minAmount := decimal.NewFromUint64(tokenAmount).Mul(slippageRate).IntPart()
		tx.Infof("buy quote: amountIn=%s virtualSol=%s virtualToken=%s realSol=%s realToken=%s tokenAmount=%d requiredSol=%d minAmount=%d",
			amountIn.String(),
			bondingCurveData.VirtualSolReserves.String(),
			bondingCurveData.VirtualTokenReserves.String(),
			bondingCurveData.RealSolReserves.String(),
			bondingCurveData.RealTokenReserves.String(),
			tokenAmount,
			requiredSol,
			minAmount,
		)

		// maxSolCost := uint64(amountIn.IntPart())
		// tokenAmount, requiredSol, err := fitBuyTokenAmountByMaxSol(maxSolCost, bondingCurveData)
		// if err != nil {
		// 	return nil, err
		// }

		// safeTokenAmount := applySafetyBps(tokenAmount, pumpBuySafetyBps)
		// slippageRate := AllBpDecimal.Sub(decimal.NewFromUint64(uint64(slippage))).Div(AllBpDecimal)
		// minAmount := decimal.NewFromUint64(safeTokenAmount).Mul(slippageRate).IntPart()

		//tx.Infof("CreateBuyInstruction: fit tokenAmount=%d requiredSol=%d safeTokenAmount=%d minAmount=%d", tokenAmount, requiredSol, safeTokenAmount, minAmount)
		minOut = minAmount
	}

	//step 3 获取ata账户
	var ataAccount aSDK.PublicKey
	switch marketTx.OutTokenProgram {
	case aSDK.TokenProgramID:
		ataAccount, _, err = aSDK.FindAssociatedTokenAddress(marketTx.UserWalletAddress, marketTx.OutMint)
		if err != nil {
			tx.Errorf("find associated token address err: %v", err)
			return nil, err
		}
	case aSDK.Token2022ProgramID:
		ataAccount, _, err = token2022.FindAssociatedToken2022Address(marketTx.UserWalletAddress, marketTx.OutMint)
		if err != nil {
			tx.Errorf("find associated token address err: %v", err)
			return nil, err
		}
	}
	//step 4 获取creator vault 相关账户
	creatorVault, err := pumpfun.CreateCreatorVault(tx.MainClient, bondingCurve.BondingCurve)
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
	globalVolumeInfo, err := tx.Client.GetAccountInfoWithOpts(context.TODO(), globalVolumeAccumulator, &rpc.GetAccountInfoOpts{
		Encoding:   solana.EncodingBase64,
		Commitment: rpc.CommitmentProcessed,
	})
	if err != nil {
		tx.Errorf("get global volume accumulator account info err: %v", err)
		return nil, err
	}
	if globalVolumeInfo.Value == nil {
		return nil, fmt.Errorf("global volume accumulator not initialized: %s", globalVolumeAccumulator.String())
	}
	//step 7 user_volume_accumulator
	userVolumeAccumulator, err := pumpfun.FindUserVolumeAccumulatorAddress(marketTx.UserWalletAddress)
	if err != nil {
		tx.Errorf("find user volume accumulator address err: %v", err)
		return nil, err
	}

	tx.Infof("minOut is %v and MaxSolCost is %v", minOut, amountIn.IntPart())

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

	// Legacy buy instruction layout: discriminator + amount + max_sol_cost + track_volume (bool)
	data := make([]byte, 16)
	copy(data[0:8], []byte{102, 6, 61, 18, 1, 218, 235, 234})
	binary.LittleEndian.PutUint64(data[8:16], buyInstruction.Amount)
	data = append(data, make([]byte, 8)...)
	binary.LittleEndian.PutUint64(data[16:24], buyInstruction.MaxSolCost)
	data = append(data, byte(1))

	return aSDK.NewInstruction(
		buyInstruction.Program,
		accounts,
		data,
	)
}

func (tx *TxManager) BuildInitUserVolumeAccumulatorInstruction(user, userVolumeAccumulator solana.PublicKey) aSDK.Instruction {
	accounts := []*aSDK.AccountMeta{
		aSDK.Meta(user).WRITE().SIGNER(),
		aSDK.Meta(user),
		aSDK.Meta(userVolumeAccumulator).WRITE(),
		aSDK.Meta(aSDK.SystemProgramID),
		aSDK.Meta(pumpfun.PumpFunEventAuthority),
		aSDK.Meta(solana.MustPublicKeyFromBase58(constant.PumpAddress)),
	}

	data := []byte{94, 6, 202, 115, 255, 96, 232, 183}

	return aSDK.NewInstruction(
		solana.MustPublicKeyFromBase58(constant.PumpAddress),
		accounts,
		data,
	)
}

func (tx *TxManager) shouldInitUserVolumeAccumulator(userVolumeAccumulator solana.PublicKey) (bool, error) {
	accountInfo, err := tx.Client.GetAccountInfoWithOpts(context.TODO(), userVolumeAccumulator, &rpc.GetAccountInfoOpts{
		Encoding:   solana.EncodingBase64,
		Commitment: rpc.CommitmentProcessed,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return true, nil
		}
		return false, fmt.Errorf("get user volume accumulator account info: %w", err)
	}

	if accountInfo.Value != nil {
		if accountInfo.Value.Owner != solana.MustPublicKeyFromBase58(constant.PumpAddress) {
			return false, fmt.Errorf("user volume accumulator owner mismatch: %s", accountInfo.Value.Owner.String())
		}
		dataLen := len(accountInfo.Value.Data.GetBinary())
		const expectedLen = 8 + 32 + 1 + 8 + 8 + 8 + 8 + 1
		if dataLen < expectedLen {
			return false, fmt.Errorf("user volume accumulator data too short: %d", dataLen)
		}
	}

	return accountInfo.Value == nil, nil
}

func (tx *TxManager) CreateSellInstruction(marketTx *entity.MarketTxExt) (aSDK.Instruction, error) {

	//step 1 组装pump bonding curve 曲线状态账户以及关联的ata账户
	bondingCurve, err := pumpfun.GetBondingCurveAndAssociatedBondingCurve(marketTx.InMint, marketTx.InTokenProgram)
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

		_, minAmountOut, err := tx.CalcMinAmountInBondingCurve(amountIn, bondingCurve, slippage)
		if err != nil {
			return nil, err
		}
		tx.Infof("CreateBuyInstruction: get minAmountIn is :%v", minAmountOut)
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
	userVolumeAccumulator, err := pumpfun.FindUserVolumeAccumulatorAddress(marketTx.UserWalletAddress)
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

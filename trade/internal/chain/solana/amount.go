package solana

import (
	"context"
	"fmt"
	"math/big"
	pumpfun "myDex/pkg/pump"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
)

type feeConfigAccount struct {
	Bump     uint8
	Admin    solana.PublicKey
	FlatFees fees
}

type fees struct {
	LpFeeBps       uint64
	ProtocolFeeBps uint64
	CreatorFeeBps  uint64
}

var feeConfigAccountDiscriminator = [8]byte{143, 52, 146, 187, 219, 123, 76, 155}

func (obj *fees) UnmarshalWithDecoder(decoder *bin.Decoder) (err error) {
	if err = decoder.Decode(&obj.LpFeeBps); err != nil {
		return err
	}
	if err = decoder.Decode(&obj.ProtocolFeeBps); err != nil {
		return err
	}
	return decoder.Decode(&obj.CreatorFeeBps)
}

func (obj *feeConfigAccount) UnmarshalWithDecoder(decoder *bin.Decoder) (err error) {
	discriminator, err := decoder.ReadTypeID()
	if err != nil {
		return err
	}
	if !discriminator.Equal(feeConfigAccountDiscriminator[:]) {
		return fmt.Errorf("wrong discriminator: wanted %v, got %v", feeConfigAccountDiscriminator, discriminator)
	}
	if err = decoder.Decode(&obj.Bump); err != nil {
		return err
	}
	if err = decoder.Decode(&obj.Admin); err != nil {
		return err
	}
	return obj.FlatFees.UnmarshalWithDecoder(decoder)
}

//计算buy 最小输出
// "- total_fee_bps = protocol_fee_bps + creator_fee_bps (creator_fee_bps is 0 if no creator)",
//         "- floor(a/b) = a / b (integer division)",
//         "- ceil(a/b) = (a + b - 1) / b",

func (tx *TxManager) calcAmountOutBybuy(amountIn decimal.Decimal, feeConfig solana.PublicKey, boddingcurve *pumpfun.BondingCurveData) (uint64, error) {
	if boddingcurve == nil {
		return 0, fmt.Errorf("bonding curve data is nil")
	}

	spendableSolIn := amountIn.BigInt()
	if spendableSolIn == nil || spendableSolIn.Sign() <= 0 {
		return 0, fmt.Errorf("spendable_sol_in must be greater than zero")
	}

	protocolFeeBps, creatorFeeBps, err := tx.fetchFlatFees(feeConfig)
	if err != nil {
		return 0, err
	}

	//step 1: 计算fee totalFeeBps = feeConfig.FlatFees.ProtocolFeeBps + feeConfig.FlatFees.CreatorFeeBps
	totalFeeBps := new(big.Int).Add(
		new(big.Int).SetUint64(protocolFeeBps),
		new(big.Int).SetUint64(creatorFeeBps),
	)

	//step 2: 计算用于交易的sol net_sol = floor(spendable_sol_in * 10000 / (10000 + total_fee_bps))
	bpsDenominator := big.NewInt(10000)
	netSolNumerator := new(big.Int).Mul(new(big.Int).Set(spendableSolIn), bpsDenominator)
	netSolDenominator := new(big.Int).Add(new(big.Int).Set(bpsDenominator), totalFeeBps)
	netSol := new(big.Int).Div(netSolNumerator, netSolDenominator)
	if netSol.Sign() <= 0 {
		return 0, fmt.Errorf("net_sol is zero")
	}

	//step 3: 计算服务费 fees = ceil(net_sol * protocol_fee_bps / 10_000) + ceil(net_sol * creator_fee_bps / 10_000) (creator_fee_bps is 0 if no creator)
	protocolFees := ceilDivBigInt(
		new(big.Int).Mul(new(big.Int).Set(netSol), new(big.Int).SetUint64(protocolFeeBps)),
		bpsDenominator,
	)
	creatorFees := ceilDivBigInt(
		new(big.Int).Mul(new(big.Int).Set(netSol), new(big.Int).SetUint64(creatorFeeBps)),
		bpsDenominator,
	)
	fees := new(big.Int).Add(protocolFees, creatorFees)

	//step 4: if net_sol + fees > spendable_sol_in: net_sol = net_sol - (net_sol + fees - spendable_sol_in)",
	if new(big.Int).Add(new(big.Int).Set(netSol), fees).Cmp(spendableSolIn) > 0 {
		overflow := new(big.Int).Sub(new(big.Int).Add(new(big.Int).Set(netSol), fees), spendableSolIn)
		netSol.Sub(netSol, overflow)
	}
	if netSol.Sign() <= 0 {
		return 0, fmt.Errorf("net_sol is zero after fee adjustment")
	}

	//step 5: tokens_out = floor((net_sol - 1) * virtual_token_reserves / (virtual_sol_reserves + net_sol - 1))
	adjustedNetSol := new(big.Int).Sub(new(big.Int).Set(netSol), big.NewInt(1))
	if adjustedNetSol.Sign() <= 0 {
		return 0, fmt.Errorf("net_sol is too small to quote buy output")
	}

	tokensOutNumerator := new(big.Int).Mul(adjustedNetSol, new(big.Int).Set(boddingcurve.VirtualTokenReserves))
	tokensOutDenominator := new(big.Int).Add(new(big.Int).Set(boddingcurve.VirtualSolReserves), adjustedNetSol)
	if tokensOutDenominator.Sign() <= 0 {
		return 0, fmt.Errorf("invalid token output denominator")
	}

	tokensOut := new(big.Int).Div(tokensOutNumerator, tokensOutDenominator)
	if tokensOut.Sign() <= 0 {
		return 0, fmt.Errorf("tokens_out is zero")
	}
	if tokensOut.Cmp(boddingcurve.RealTokenReserves) > 0 {
		return 0, fmt.Errorf("tokens_out exceeds real token reserves")
	}
	if !tokensOut.IsUint64() {
		return 0, fmt.Errorf("tokens_out exceeds uint64")
	}

	return tokensOut.Uint64(), nil
}

// calcSpendableSolInByTokens calculates spendable_sol_in for a desired number of tokens using feeConfig.
// Formula (from IDL):
// 1. net_sol = ceil(tokens * virtual_sol_reserves / (virtual_token_reserves - tokens)) + 1
// 2. spendable_sol_in = ceil(net_sol * (10_000 + total_fee_bps) / 10_000)
func (tx *TxManager) calcSpendableSolInByTokens(tokens uint64, feeConfig solana.PublicKey, boddingcurve *pumpfun.BondingCurveData) (uint64, error) {
	if boddingcurve == nil {
		return 0, fmt.Errorf("bonding curve data is nil")
	}
	if tokens == 0 {
		return 0, fmt.Errorf("tokens is zero")
	}
	if new(big.Int).SetUint64(tokens).Cmp(boddingcurve.RealTokenReserves) > 0 {
		return 0, fmt.Errorf("tokens exceeds real token reserves")
	}
	if new(big.Int).SetUint64(tokens).Cmp(boddingcurve.VirtualTokenReserves) >= 0 {
		return 0, fmt.Errorf("tokens exceeds virtual token reserves")
	}

	protocolFeeBps, creatorFeeBps, err := tx.fetchFlatFees(feeConfig)
	if err != nil {
		return 0, err
	}
	totalFeeBps := new(big.Int).Add(
		new(big.Int).SetUint64(protocolFeeBps),
		new(big.Int).SetUint64(creatorFeeBps),
	)
	bpsDenominator := big.NewInt(10000)

	tokensBig := new(big.Int).SetUint64(tokens)
	denominator := new(big.Int).Sub(new(big.Int).Set(boddingcurve.VirtualTokenReserves), tokensBig)
	if denominator.Sign() <= 0 {
		return 0, fmt.Errorf("invalid token denominator")
	}

	netSolNumerator := new(big.Int).Mul(tokensBig, new(big.Int).Set(boddingcurve.VirtualSolReserves))
	netSol := ceilDivBigInt(netSolNumerator, denominator)
	netSol.Add(netSol, big.NewInt(1))
	if netSol.Sign() <= 0 {
		return 0, fmt.Errorf("net_sol is zero")
	}

	spendableNumerator := new(big.Int).Mul(netSol, new(big.Int).Add(new(big.Int).Set(bpsDenominator), totalFeeBps))
	spendableSol := ceilDivBigInt(spendableNumerator, bpsDenominator)
	if !spendableSol.IsUint64() {
		return 0, fmt.Errorf("spendable_sol_in exceeds uint64")
	}

	return spendableSol.Uint64(), nil
}

// fitBuyTokenAmountByMaxSolWithFee finds the max token amount that fits maxSolCost using feeConfig.
func (tx *TxManager) fitBuyTokenAmountByMaxSolWithFee(maxSolCost uint64, feeConfig solana.PublicKey, boddingcurve *pumpfun.BondingCurveData) (uint64, uint64, error) {
	if maxSolCost == 0 {
		return 0, 0, fmt.Errorf("maxSolCost is zero")
	}
	if boddingcurve == nil {
		return 0, 0, fmt.Errorf("bonding curve data is nil")
	}

	// upper bound: real token reserves or virtual-1
	maxTokens := new(big.Int).Set(boddingcurve.RealTokenReserves)
	vtMinusOne := new(big.Int).Sub(new(big.Int).Set(boddingcurve.VirtualTokenReserves), big.NewInt(1))
	if maxTokens.Cmp(vtMinusOne) > 0 {
		maxTokens = vtMinusOne
	}
	if maxTokens.Sign() <= 0 || !maxTokens.IsUint64() {
		return 0, 0, fmt.Errorf("invalid max token bound")
	}

	low := uint64(1)
	high := maxTokens.Uint64()
	var bestAmount uint64
	var bestRequiredSol uint64

	for low <= high {
		mid := low + (high-low)/2
		requiredSol, err := tx.calcSpendableSolInByTokens(mid, feeConfig, boddingcurve)
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

func (tx *TxManager) fetchFlatFees(feeConfig solana.PublicKey) (uint64, uint64, error) {
	accountInfo, err := tx.Client.GetAccountInfoWithOpts(context.TODO(), feeConfig, &rpc.GetAccountInfoOpts{
		Encoding:   solana.EncodingBase64,
		Commitment: rpc.CommitmentProcessed,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("fetch fee config: %w", err)
	}
	if accountInfo.Value == nil || accountInfo.Value.Data == nil {
		return 0, 0, fmt.Errorf("fee config account not found")
	}

	data := accountInfo.Value.Data.GetBinary()
	decoder := bin.NewBorshDecoder(data)
	var cfg feeConfigAccount
	if err := cfg.UnmarshalWithDecoder(decoder); err != nil {
		return 0, 0, fmt.Errorf("decode fee config: %w", err)
	}

	return cfg.FlatFees.ProtocolFeeBps, cfg.FlatFees.CreatorFeeBps, nil
}

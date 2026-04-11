package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"myDex/pkg/constant"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
)

var (
	// Global account address for pump.fun
	GlobalPumpFunAddress  solana.PublicKey
	PumpFunEventAuthority solana.PublicKey
	PumpFeeProgramAddress solana.PublicKey
)

func init() {
	// Pump.fun global account address
	GlobalPumpFunAddress = solana.MustPublicKeyFromBase58("4wTV1YmiEkRvAtNtsSGPtUrqRYQMe5SKy2uB4Jjaxnjf")
	// Pump.fun event authority
	PumpFunEventAuthority = solana.MustPublicKeyFromBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1")

	// Fee Program (constant)
	PumpFeeProgramAddress = solana.MustPublicKeyFromBase58("pfeeUxB6jkeY1Hxd7CsFCAjcbHA9rWtchMGdZ6VojVZ")
}

type BondingCurvePublicKeys struct {
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
}

type CreatorVaultResult struct {
	Creator      solana.PublicKey
	CreatorVault solana.PublicKey
}

type BondingCurveData struct {
	RealTokenReserves    *big.Int
	VirtualTokenReserves *big.Int
	VirtualSolReserves   *big.Int
}

type PumpFun struct {
	context   context.Context
	rpcClient *rpc.Client
}

func FetchBondingCurve(rpcClient *rpc.Client, bondingCurvePubKey solana.PublicKey) (*BondingCurveData, error) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp, err := rpcClient.GetAccountInfoWithOpts(ctx, bondingCurvePubKey,
		&rpc.GetAccountInfoOpts{Encoding: solana.EncodingBase64,
			Commitment: rpc.CommitmentProcessed})
	if err != nil {
		return nil, fmt.Errorf("FBCD: failed to get account info: %w", err)
	}

	data := resp.Value.Data.GetBinary()
	if len(data) < 32 {
		return nil, fmt.Errorf("FBCD: insufficient data length")
	}

	discriminator := data[0:8]
	virtualTokenReserves := data[8:16]
	virtualSolReserves := data[16:24]
	RealTokenReserves := data[24:32]

	logx.Infof("Fetched bonding curve data: {discriminator: %v, virtualTokenReserves: %v, virtualSolReserves: %v, realTokenReserves: %v}", discriminator, virtualTokenReserves, virtualSolReserves, RealTokenReserves)

	return &BondingCurveData{
		RealTokenReserves:    new(big.Int).SetUint64(binary.LittleEndian.Uint64(RealTokenReserves)),
		VirtualTokenReserves: new(big.Int).SetUint64(binary.LittleEndian.Uint64(virtualTokenReserves)),
		VirtualSolReserves:   new(big.Int).SetUint64(binary.LittleEndian.Uint64(virtualSolReserves)),
	}, nil
}

// GetBondingCurveAndAssociatedBondingCurve returns the bonding curve and associated bonding curve, in a structured format.
func GetBondingCurveAndAssociatedBondingCurve(mint solana.PublicKey) (*BondingCurvePublicKeys, error) {
	// Derive bonding curve address.
	// define the seeds used to derive the PDA
	// getProgramDerivedAddress equivalent.
	seeds := [][]byte{
		[]byte("bonding-curve"),
		mint.Bytes(),
	}
	bondingCurve, _, err := solana.FindProgramAddress(seeds, solana.MustPublicKeyFromBase58(constant.PumpAddress))
	if err != nil {
		return nil, fmt.Errorf("failed to derive bonding curve address: %w", err)
	}
	// Derive associated bonding curve address.
	associatedBondingCurve, _, err := solana.FindAssociatedTokenAddress(
		bondingCurve,
		mint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to derive associated bonding curve address: %w", err)
	}
	return &BondingCurvePublicKeys{
		BondingCurve:           bondingCurve,
		AssociatedBondingCurve: associatedBondingCurve,
	}, nil
}

// bondingCurve 账户
// VirtualTokenReserves uint64
// VirtualSolReserves   uint64
// RealTokenReserves    uint64
// RealSolReserves      uint64
// TokenTotalSupply     uint64
// Complete             bool
// Creator              ag_solanago.PublicKey
func CreateCreatorVault(rpcClient *rpc.Client, bondingCurve solana.PublicKey) (*CreatorVaultResult, error) {
	// Implementation for creating creator vault

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp, err := rpcClient.GetAccountInfoWithOpts(ctx, bondingCurve,
		&rpc.GetAccountInfoOpts{Encoding: solana.EncodingBase64,
			Commitment: rpc.CommitmentProcessed})
	if err != nil {
		return nil, fmt.Errorf("FBCD: failed to get account info: %w", err)
	}

	data := resp.Value.Data.GetBinary()
	if len(data) < 32 {
		return nil, fmt.Errorf("FBCD: insufficient data length")
	}

	if len(data) < 81 {
		return nil, fmt.Errorf("FBCD: insufficient data length for creator vault")
	}

	//获取create vault地址
	creator := data[49:81]
	creatorPubKey := solana.PublicKeyFromBytes(creator)

	seeds := [][]byte{
		[]byte("creator-vault"),
		creator,
	}

	pda, _, err := solana.FindProgramAddress(seeds, solana.MustPublicKeyFromBase58(constant.PumpAddress))
	if err != nil {
		return nil, fmt.Errorf("failed to find creator vault PDA: %w", err)
	}

	return &CreatorVaultResult{
		Creator:      creatorPubKey,
		CreatorVault: pda,
	}, nil

}

func GetGlobalFeeRecipient(rpcClient *rpc.Client) (solana.PublicKey, error) {
	// Derive Global PDA using seed "global"
	globalPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("global")}, solana.MustPublicKeyFromBase58(constant.PumpAddress))
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("derive global PDA: %w", err)
	}
	acct, err := rpcClient.GetAccountInfoWithOpts(context.TODO(), globalPDA, &rpc.GetAccountInfoOpts{
		Encoding:   solana.EncodingBase64,
		Commitment: rpc.CommitmentProcessed,
	})
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("fetch global account: %w", err)
	}
	if acct.Value == nil || acct.Value.Data == nil {
		return solana.PublicKey{}, fmt.Errorf("global account not found")
	}
	data := acct.Value.Data.GetBinary()
	if len(data) < 73 {
		return solana.PublicKey{}, fmt.Errorf("invalid global account data length")
	}

	feeRecipient := data[41:73]
	feeRecipientPubKey := solana.PublicKeyFromBytes(feeRecipient)

	return solana.MustPublicKeyFromBase58(feeRecipientPubKey.String()), nil
}

func FindGlobalVolumeAccumulatorAddress() (pda solana.PublicKey, err error) {
	pda, _, err = solana.FindProgramAddress([][]byte{[]byte("global_volume_accumulator")}, solana.MustPublicKeyFromBase58(constant.PumpAddress))
	if err != nil {

		return solana.PublicKey{}, fmt.Errorf("derive global_volume_accumulator PDA: %w", err)
	}
	return pda, nil
}

func FindUserVolumeAccumulatorAddress() (pda solana.PublicKey, err error) {
	pda, _, err = solana.FindProgramAddress([][]byte{[]byte("user_volume_accumulator")}, solana.MustPublicKeyFromBase58(constant.PumpAddress))
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("derive user_volume_accumulator PDA: %w", err)
	}
	return pda, nil
}

func FindFeeConfigAddress() (pda solana.PublicKey, err error) {

	feeConfigSeedTag := []byte("fee_config")
	feeConfigSeedKey := []byte{1, 86, 224, 246, 147, 102, 90, 207, 68, 219, 21, 104, 191, 23, 91, 170, 81, 137, 203, 151, 245, 210, 255, 59, 101, 93, 43, 182, 253, 109, 24, 176}

	var seeds [][]byte
	// const: fee_config
	seeds = append(seeds, feeConfigSeedTag)
	// const (raw): [1 86 224 246 147 102 90 207 68 219 21 104 191 23 91 170 81 137 203 151 245 210 255 59 101 93 43 182 253 109 24 176]
	seeds = append(seeds, feeConfigSeedKey)

	pda, _, err = solana.FindProgramAddress(seeds, PumpFeeProgramAddress)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("derive fee config PDA: %w", err)
	}

	return pda, nil
}

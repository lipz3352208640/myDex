package pumpfun

import (
	"context"
	"fmt"
	"math/big"
	"myDex/pkg/constant"
	token2022 "myDex/pkg/token2022"
	"time"

	bin "github.com/gagliardetto/binary"
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
	RealSolReserves      *big.Int
	VirtualTokenReserves *big.Int
	VirtualSolReserves   *big.Int
	TokenTotalSupply     *big.Int
	Complete             bool
	Creator              solana.PublicKey
}

type PumpFun struct {
	context   context.Context
	rpcClient *rpc.Client
}

type globalAccountMinimal struct {
	Initialized  bool
	Authority    solana.PublicKey
	FeeRecipient solana.PublicKey
}

var globalAccountDiscriminator = [8]byte{167, 232, 232, 177, 200, 108, 114, 127}

func (obj *globalAccountMinimal) UnmarshalWithDecoder(decoder *bin.Decoder) (err error) {
	discriminator, err := decoder.ReadTypeID()
	if err != nil {
		return err
	}
	if !discriminator.Equal(globalAccountDiscriminator[:]) {
		return fmt.Errorf("wrong discriminator: wanted %v, got %v", globalAccountDiscriminator, discriminator)
	}
	if err = decoder.Decode(&obj.Initialized); err != nil {
		return err
	}
	if err = decoder.Decode(&obj.Authority); err != nil {
		return err
	}
	return decoder.Decode(&obj.FeeRecipient)
}

type bondingCurveAccount struct {
	VirtualTokenReserves uint64
	VirtualSolReserves   uint64
	RealTokenReserves    uint64
	RealSolReserves      uint64
	TokenTotalSupply     uint64
	Complete             bool
	Creator              solana.PublicKey
}

var bondingCurveAccountDiscriminator = [8]byte{23, 183, 248, 55, 96, 216, 172, 96}

func (obj *bondingCurveAccount) UnmarshalWithDecoder(decoder *bin.Decoder) (err error) {
	discriminator, err := decoder.ReadTypeID()
	if err != nil {
		return err
	}
	if !discriminator.Equal(bondingCurveAccountDiscriminator[:]) {
		return fmt.Errorf("wrong discriminator: wanted %v, got %v", bondingCurveAccountDiscriminator, discriminator)
	}
	if err = decoder.Decode(&obj.VirtualTokenReserves); err != nil {
		return err
	}
	if err = decoder.Decode(&obj.VirtualSolReserves); err != nil {
		return err
	}
	if err = decoder.Decode(&obj.RealTokenReserves); err != nil {
		return err
	}
	if err = decoder.Decode(&obj.RealSolReserves); err != nil {
		return err
	}
	if err = decoder.Decode(&obj.TokenTotalSupply); err != nil {
		return err
	}
	if err = decoder.Decode(&obj.Complete); err != nil {
		return err
	}
	return decoder.Decode(&obj.Creator)
}

func FetchBondingCurve(rpcClient *rpc.Client, bondingCurvePubKey solana.PublicKey) (*BondingCurveData, error) {

	fmt.Println("bondingCurvePDA:", bondingCurvePubKey.String())
	ctx, cancel := context.WithTimeout(context.Background(), 5000000*time.Second)
	defer cancel()
	resp, err := rpcClient.GetAccountInfoWithOpts(ctx, bondingCurvePubKey,
		&rpc.GetAccountInfoOpts{Encoding: solana.EncodingBase64,
			Commitment: rpc.CommitmentConfirmed})
	if err != nil {
		return nil, fmt.Errorf("FBCD: failed to get account info: %w", err)
	}

	data := resp.Value.Data.GetBinary()
	decoder := bin.NewBorshDecoder(data)
	var account bondingCurveAccount
	if err := account.UnmarshalWithDecoder(decoder); err != nil {
		return nil, fmt.Errorf("FBCD: decode bonding curve failed: %w", err)
	}

	logx.Infof("Fetched bonding curve data: {virtualTokenReserves: %d, virtualSolReserves: %d, realTokenReserves: %d, realSolReserves: %d, complete: %v}",
		account.VirtualTokenReserves, account.VirtualSolReserves, account.RealTokenReserves, account.RealSolReserves, account.Complete)

	return &BondingCurveData{
		RealTokenReserves:    new(big.Int).SetUint64(account.RealTokenReserves),
		RealSolReserves:      new(big.Int).SetUint64(account.RealSolReserves),
		VirtualTokenReserves: new(big.Int).SetUint64(account.VirtualTokenReserves),
		VirtualSolReserves:   new(big.Int).SetUint64(account.VirtualSolReserves),
		TokenTotalSupply:     new(big.Int).SetUint64(account.TokenTotalSupply),
		Complete:             account.Complete,
		Creator:              solana.PublicKey(account.Creator),
	}, nil
}

// GetBondingCurveAndAssociatedBondingCurve returns the bonding curve and associated bonding curve, in a structured format.
func GetBondingCurveAndAssociatedBondingCurve(mint solana.PublicKey, tokenProgram solana.PublicKey) (*BondingCurvePublicKeys, error) {
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

	var associatedBondingCurve solana.PublicKey
	var err1 error
	switch tokenProgram {
	case solana.TokenProgramID:
		associatedBondingCurve, _, err1 = solana.FindAssociatedTokenAddress(bondingCurve, mint)

	case solana.Token2022ProgramID:
		associatedBondingCurve, _, err1 = token2022.FindAssociatedToken2022Address(bondingCurve, mint)
	}
	// associatedBondingCurve, _, err := solana.FindAssociatedTokenAddress(
	// 	bondingCurve,
	// 	mint,
	// )
	if err1 != nil {
		return nil, fmt.Errorf("failed to derive associated bonding curve address: %w", err1)
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
			Commitment: rpc.CommitmentConfirmed})
	if err != nil {
		return nil, fmt.Errorf("FBCD: failed to get account info: %w", err)
	}

	data := resp.Value.Data.GetBinary()
	decoder := bin.NewBorshDecoder(data)
	var account bondingCurveAccount
	if err := account.UnmarshalWithDecoder(decoder); err != nil {
		return nil, fmt.Errorf("FBCD: decode bonding curve for creator vault failed: %w", err)
	}

	//获取create vault地址
	creatorPubKey := solana.PublicKey(account.Creator)

	seeds := [][]byte{
		[]byte("creator-vault"),
		creatorPubKey.Bytes(),
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
	decoder := bin.NewBorshDecoder(data)
	var globalAccount globalAccountMinimal
	if err := globalAccount.UnmarshalWithDecoder(decoder); err != nil {
		return solana.PublicKey{}, fmt.Errorf("decode global account: %w", err)
	}

	return globalAccount.FeeRecipient, nil
}

func FindGlobalVolumeAccumulatorAddress() (pda solana.PublicKey, err error) {
	pda, _, err = solana.FindProgramAddress([][]byte{[]byte("global_volume_accumulator")}, solana.MustPublicKeyFromBase58(constant.PumpAddress))
	if err != nil {

		return solana.PublicKey{}, fmt.Errorf("derive global_volume_accumulator PDA: %w", err)
	}
	return pda, nil
}

func FindUserVolumeAccumulatorAddress(user solana.PublicKey) (pda solana.PublicKey, err error) {
	pda, _, err = solana.FindProgramAddress([][]byte{[]byte("user_volume_accumulator"), user.Bytes()}, solana.MustPublicKeyFromBase58(constant.PumpAddress))
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

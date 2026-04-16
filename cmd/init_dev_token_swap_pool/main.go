package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	system "github.com/gagliardetto/solana-go/programs/system"
	token "github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
)

const (
	swapStateSize    = 324
	tokenMintSize    = 82
	tokenAccountSize = 165
)

const (
	constantProductCurveType = 0
)

type tokenSwapFees struct {
	TradeFeeNumerator           uint64
	TradeFeeDenominator         uint64
	OwnerTradeFeeNumerator      uint64
	OwnerTradeFeeDenominator    uint64
	OwnerWithdrawFeeNumerator   uint64
	OwnerWithdrawFeeDenominator uint64
	HostFeeNumerator            uint64
	HostFeeDenominator          uint64
}

type poolConfig struct {
	RPCURL          string `json:"rpcUrl"`
	Payer           string `json:"payer"`
	UsdcMint        string `json:"usdcMint"`
	UsdcSource      string `json:"usdcSource"`
	SwapAccount     string `json:"swapAccount"`
	SwapAuthority   string `json:"swapAuthority"`
	WsolVault       string `json:"wsolVault"`
	UsdcVault       string `json:"usdcVault"`
	PoolMint        string `json:"poolMint"`
	PoolFeeAccount  string `json:"poolFeeAccount"`
	PoolDestination string `json:"poolDestination"`
	SolLamports     uint64 `json:"solLamports"`
	UsdcAmount      uint64 `json:"usdcAmount"`
	UsdcDecimals    uint8  `json:"usdcDecimals"`
	PoolDecimals    uint8  `json:"poolDecimals"`
}

type tokenSwapInstruction struct {
	programID solana.PublicKey
	accounts  solana.AccountMetaSlice
	data      []byte
}

func (i *tokenSwapInstruction) ProgramID() solana.PublicKey     { return i.programID }
func (i *tokenSwapInstruction) Accounts() []*solana.AccountMeta { return i.accounts }
func (i *tokenSwapInstruction) Data() ([]byte, error)           { return i.data, nil }

func main() {
	var (
		rpcURL               = flag.String("rpc-url", "https://api.devnet.solana.com", "Solana RPC endpoint")
		usdcMint             = flag.String("usdc-mint", "", "USDC mint address on devnet")
		usdcSource           = flag.String("usdc-source-account", "", "optional USDC source token account; defaults to payer ATA")
		solUI                = flag.String("sol-ui", "0.1", "initial SOL liquidity in UI units")
		usdcUI               = flag.String("usdc-ui", "10", "initial USDC liquidity in UI units")
		poolDecimals         = flag.Uint("pool-decimals", 9, "LP mint decimals")
		legacyInitializeData = flag.Bool("legacy-initialize-data", true, "pack initialize data with legacy nonce byte for older token-swap deployments")
		outPath              = flag.String("out", "", "optional path to write the created pool config JSON")
		dryRun               = flag.Bool("dry-run", false, "build and print derived accounts without sending the transaction")
	)
	flag.Parse()

	if *usdcMint == "" {
		exitf("missing required flag: --usdc-mint")
	}

	privateKeyBase58 := firstNonEmpty(os.Getenv("PRIVATE_KEY"), os.Getenv("private_key"))
	if privateKeyBase58 == "" {
		exitf("missing PRIVATE_KEY/private_key environment variable")
	}

	payer, err := solana.PrivateKeyFromBase58(privateKeyBase58)
	if err != nil {
		exitf("invalid private key: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := rpc.New(*rpcURL)

	usdcMintKey := solana.MustPublicKeyFromBase58(*usdcMint)
	usdcSourceKey, usdcDecimals, err := resolveUSDCSupport(ctx, client, payer.PublicKey(), usdcMintKey, *usdcSource)
	if err != nil {
		exitf("resolve USDC setup: %v", err)
	}

	solLamports, err := parseUIAmount(*solUI, 9)
	if err != nil {
		exitf("invalid --sol-ui: %v", err)
	}
	usdcAmount, err := parseUIAmount(*usdcUI, usdcDecimals)
	if err != nil {
		exitf("invalid --usdc-ui: %v", err)
	}

	if solLamports == 0 || usdcAmount == 0 {
		exitf("initial liquidity must be greater than zero")
	}

	swapAccount := solana.NewWallet()
	wsolVault := solana.NewWallet()
	usdcVault := solana.NewWallet()
	poolMint := solana.NewWallet()
	poolFeeAccount := solana.NewWallet()
	poolDestination := solana.NewWallet()

	swapAuthority, swapAuthorityBump, err := solana.FindProgramAddress(
		[][]byte{swapAccount.PublicKey().Bytes()},
		solana.TokenSwapProgramID,
	)
	if err != nil {
		exitf("derive swap authority: %v", err)
	}

	swapRent, err := client.GetMinimumBalanceForRentExemption(ctx, swapStateSize, rpc.CommitmentConfirmed)
	if err != nil {
		exitf("rent exemption for swap state: %v", err)
	}
	tokenAccountRent, err := client.GetMinimumBalanceForRentExemption(ctx, tokenAccountSize, rpc.CommitmentConfirmed)
	if err != nil {
		exitf("rent exemption for token account: %v", err)
	}
	mintRent, err := client.GetMinimumBalanceForRentExemption(ctx, tokenMintSize, rpc.CommitmentConfirmed)
	if err != nil {
		exitf("rent exemption for mint: %v", err)
	}

	cfg := poolConfig{
		RPCURL:          *rpcURL,
		Payer:           payer.PublicKey().String(),
		UsdcMint:        usdcMintKey.String(),
		UsdcSource:      usdcSourceKey.String(),
		SwapAccount:     swapAccount.PublicKey().String(),
		SwapAuthority:   swapAuthority.String(),
		WsolVault:       wsolVault.PublicKey().String(),
		UsdcVault:       usdcVault.PublicKey().String(),
		PoolMint:        poolMint.PublicKey().String(),
		PoolFeeAccount:  poolFeeAccount.PublicKey().String(),
		PoolDestination: poolDestination.PublicKey().String(),
		SolLamports:     solLamports,
		UsdcAmount:      usdcAmount,
		UsdcDecimals:    usdcDecimals,
		PoolDecimals:    uint8(*poolDecimals),
	}
	printJSON("pool setup", cfg)

	outputPath := *outPath
	if strings.TrimSpace(outputPath) == "" {
		outputPath = fmt.Sprintf("token-swap-pool-%s.json", swapAccount.PublicKey().String())
	}

	vaultSetupInstructions, poolSetupInstructions, initializeInstructions, err := buildPoolInstructions(
		payer.PublicKey(),
		usdcMintKey,
		usdcSourceKey,
		swapAccount.PublicKey(),
		swapAuthority,
		wsolVault.PublicKey(),
		usdcVault.PublicKey(),
		poolMint.PublicKey(),
		poolFeeAccount.PublicKey(),
		poolDestination.PublicKey(),
		solLamports,
		usdcAmount,
		uint8(*poolDecimals),
		*legacyInitializeData,
		swapAuthorityBump,
		tokenAccountRent,
		mintRent,
		swapRent,
	)
	if err != nil {
		exitf("build pool instructions: %v", err)
	}

	if *dryRun {
		if err := writeJSONFile(outputPath, cfg); err != nil {
			exitf("write pool config: %v", err)
		}
		fmt.Printf(
			"dry run enabled; vault setup instructions=%d pool setup instructions=%d initialize instructions=%d config=%s\n",
			len(vaultSetupInstructions),
			len(poolSetupInstructions),
			len(initializeInstructions),
			outputPath,
		)
		return
	}

	vaultSetupSigners := []solana.PrivateKey{
		payer,
		wsolVault.PrivateKey,
		usdcVault.PrivateKey,
	}
	vaultSetupSig, err := sendSignedTransaction(ctx, client, payer.PublicKey(), vaultSetupInstructions, vaultSetupSigners)
	if err != nil {
		exitf("send vault setup transaction: %v", err)
	}
	fmt.Printf("submitted vault setup transaction: %s\n", vaultSetupSig.String())
	if err := waitForConfirmation(ctx, client, vaultSetupSig); err != nil {
		exitf("vault setup transaction %s not confirmed: %v", vaultSetupSig.String(), err)
	}

	poolSetupSigners := []solana.PrivateKey{
		payer,
		poolMint.PrivateKey,
		poolFeeAccount.PrivateKey,
		poolDestination.PrivateKey,
	}
	poolSetupSig, err := sendSignedTransaction(ctx, client, payer.PublicKey(), poolSetupInstructions, poolSetupSigners)
	if err != nil {
		exitf("send pool setup transaction: %v", err)
	}
	fmt.Printf("submitted pool setup transaction: %s\n", poolSetupSig.String())
	if err := waitForConfirmation(ctx, client, poolSetupSig); err != nil {
		exitf("pool setup transaction %s not confirmed: %v", poolSetupSig.String(), err)
	}

	if err := validatePoolAccounts(
		ctx,
		client,
		swapAuthority,
		usdcMintKey,
		wsolVault.PublicKey(),
		usdcVault.PublicKey(),
		poolMint.PublicKey(),
		poolFeeAccount.PublicKey(),
		poolDestination.PublicKey(),
	); err != nil {
		exitf("validate pool accounts before initialize: %v", err)
	}

	initializeSigners := []solana.PrivateKey{
		payer,
		swapAccount.PrivateKey,
	}
	initializeSig, err := sendSignedTransaction(ctx, client, payer.PublicKey(), initializeInstructions, initializeSigners)
	if err != nil {
		exitf("send initialize transaction: %v", err)
	}
	fmt.Printf("submitted initialize transaction: %s\n", initializeSig.String())
	if err := waitForConfirmation(ctx, client, initializeSig); err != nil {
		exitf("initialize transaction %s not confirmed: %v", initializeSig.String(), err)
	}

	fmt.Printf("pool initialized successfully: %s\n", initializeSig.String())
	if err := writeJSONFile(outputPath, cfg); err != nil {
		exitf("write pool config: %v", err)
	}
	fmt.Printf("pool config written to: %s\n", outputPath)
}

func resolveUSDCSupport(
	ctx context.Context,
	client *rpc.Client,
	payer solana.PublicKey,
	usdcMint solana.PublicKey,
	overrideSource string,
) (solana.PublicKey, uint8, error) {
	mintInfo, err := client.GetAccountInfo(ctx, usdcMint)
	if err != nil {
		return solana.PublicKey{}, 0, err
	}
	if !mintInfo.Value.Owner.Equals(solana.TokenProgramID) {
		return solana.PublicKey{}, 0, fmt.Errorf("USDC mint owner is %s, expected %s", mintInfo.Value.Owner, solana.TokenProgramID)
	}

	supply, err := client.GetTokenSupply(ctx, usdcMint, rpc.CommitmentConfirmed)
	if err != nil {
		return solana.PublicKey{}, 0, err
	}
	if supply == nil || supply.Value == nil {
		return solana.PublicKey{}, 0, errors.New("missing token supply response")
	}

	if overrideSource != "" {
		return solana.MustPublicKeyFromBase58(overrideSource), supply.Value.Decimals, nil
	}

	source, _, err := solana.FindAssociatedTokenAddress(payer, usdcMint)
	if err != nil {
		return solana.PublicKey{}, 0, err
	}
	return source, supply.Value.Decimals, nil
}

func buildPoolInstructions(
	payer solana.PublicKey,
	usdcMint solana.PublicKey,
	usdcSource solana.PublicKey,
	swapAccount solana.PublicKey,
	swapAuthority solana.PublicKey,
	wsolVault solana.PublicKey,
	usdcVault solana.PublicKey,
	poolMint solana.PublicKey,
	poolFeeAccount solana.PublicKey,
	poolDestination solana.PublicKey,
	solLamports uint64,
	usdcAmount uint64,
	poolDecimals uint8,
	legacyInitializeData bool,
	swapAuthorityBump uint8,
	tokenAccountRent uint64,
	mintRent uint64,
	swapRent uint64,
) ([]solana.Instruction, []solana.Instruction, []solana.Instruction, error) {
	var vaultSetupInstructions []solana.Instruction

	createWsolVault, err := system.NewCreateAccountInstruction(
		tokenAccountRent+solLamports,
		tokenAccountSize,
		solana.TokenProgramID,
		payer,
		wsolVault,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	initWsolVault, err := token.NewInitializeAccount3Instruction(
		swapAuthority,
		wsolVault,
		solana.WrappedSol,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	syncWsol, err := token.NewSyncNativeInstruction(wsolVault).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	vaultSetupInstructions = append(vaultSetupInstructions, createWsolVault, initWsolVault, syncWsol)

	createUsdcVault, err := system.NewCreateAccountInstruction(
		tokenAccountRent,
		tokenAccountSize,
		solana.TokenProgramID,
		payer,
		usdcVault,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	initUsdcVault, err := token.NewInitializeAccount3Instruction(
		swapAuthority,
		usdcVault,
		usdcMint,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	transferUsdc, err := token.NewTransferInstruction(
		usdcAmount,
		usdcSource,
		usdcVault,
		payer,
		nil,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	vaultSetupInstructions = append(vaultSetupInstructions, createUsdcVault, initUsdcVault, transferUsdc)

	var poolSetupInstructions []solana.Instruction

	createPoolMint, err := system.NewCreateAccountInstruction(
		mintRent,
		tokenMintSize,
		solana.TokenProgramID,
		payer,
		poolMint,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	initPoolMintBuilder := token.NewInitializeMint2InstructionBuilder().
		SetDecimals(poolDecimals).
		SetMintAuthority(swapAuthority).
		SetMintAccount(poolMint)
	initPoolMint, err := initPoolMintBuilder.ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	poolSetupInstructions = append(poolSetupInstructions, createPoolMint, initPoolMint)

	createPoolFee, err := system.NewCreateAccountInstruction(
		tokenAccountRent,
		tokenAccountSize,
		solana.TokenProgramID,
		payer,
		poolFeeAccount,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	initPoolFee, err := token.NewInitializeAccount3Instruction(
		solana.TokenSwapFeeOwner,
		poolFeeAccount,
		poolMint,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	poolSetupInstructions = append(poolSetupInstructions, createPoolFee, initPoolFee)

	createPoolDestination, err := system.NewCreateAccountInstruction(
		tokenAccountRent,
		tokenAccountSize,
		solana.TokenProgramID,
		payer,
		poolDestination,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	initPoolDestination, err := token.NewInitializeAccount3Instruction(
		payer,
		poolDestination,
		poolMint,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	poolSetupInstructions = append(poolSetupInstructions, createPoolDestination, initPoolDestination)

	createSwapState, err := system.NewCreateAccountInstruction(
		swapRent,
		swapStateSize,
		solana.TokenSwapProgramID,
		payer,
		swapAccount,
	).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, err
	}
	initializeInstructions := []solana.Instruction{createSwapState, newInitializeTokenSwapInstruction(
		swapAccount,
		swapAuthority,
		wsolVault,
		usdcVault,
		poolMint,
		poolFeeAccount,
		poolDestination,
		legacyInitializeData,
		swapAuthorityBump,
	)}

	return vaultSetupInstructions, poolSetupInstructions, initializeInstructions, nil
}

func newInitializeTokenSwapInstruction(
	swapAccount solana.PublicKey,
	swapAuthority solana.PublicKey,
	wsolVault solana.PublicKey,
	usdcVault solana.PublicKey,
	poolMint solana.PublicKey,
	poolFeeAccount solana.PublicKey,
	poolDestination solana.PublicKey,
	legacyInitializeData bool,
	swapAuthorityBump uint8,
) solana.Instruction {
	fees := tokenSwapFees{
		TradeFeeNumerator:           0,
		TradeFeeDenominator:         10000,
		OwnerTradeFeeNumerator:      5,
		OwnerTradeFeeDenominator:    10000,
		OwnerWithdrawFeeNumerator:   0,
		OwnerWithdrawFeeDenominator: 0,
		HostFeeNumerator:            20,
		HostFeeDenominator:          100,
	}

	dataCapacity := 1 + 64 + 33
	if legacyInitializeData {
		dataCapacity++
	}
	data := make([]byte, 0, dataCapacity)
	data = append(data, 0)
	if legacyInitializeData {
		data = append(data, swapAuthorityBump)
	}
	data = append(data, packFees(fees)...)
	data = append(data, packConstantProductCurve()...)

	return &tokenSwapInstruction{
		programID: solana.TokenSwapProgramID,
		accounts: solana.AccountMetaSlice{
			solana.Meta(swapAccount).WRITE().SIGNER(),
			solana.Meta(swapAuthority),
			solana.Meta(wsolVault),
			solana.Meta(usdcVault),
			solana.Meta(poolMint).WRITE(),
			solana.Meta(poolFeeAccount).WRITE(),
			solana.Meta(poolDestination).WRITE(),
			solana.Meta(solana.TokenProgramID),
		},
		data: data,
	}
}

func packFees(fees tokenSwapFees) []byte {
	out := make([]byte, 0, 64)
	values := []uint64{
		fees.TradeFeeNumerator,
		fees.TradeFeeDenominator,
		fees.OwnerTradeFeeNumerator,
		fees.OwnerTradeFeeDenominator,
		fees.OwnerWithdrawFeeNumerator,
		fees.OwnerWithdrawFeeDenominator,
		fees.HostFeeNumerator,
		fees.HostFeeDenominator,
	}
	for _, v := range values {
		out = append(out, byte(v), byte(v>>8), byte(v>>16), byte(v>>24), byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
	}
	return out
}

func packConstantProductCurve() []byte {
	out := make([]byte, 33)
	out[0] = constantProductCurveType
	return out
}

func parseUIAmount(value string, decimals uint8) (uint64, error) {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return 0, err
	}
	if d.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("amount must be positive")
	}

	base := decimal.NewFromInt(10).Pow(decimal.NewFromInt32(int32(decimals)))
	scaled := d.Mul(base)
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, fmt.Errorf("amount %s has more than %d decimals", value, decimals)
	}
	if !scaled.IsPositive() || !scaled.IsInteger() {
		return 0, fmt.Errorf("invalid scaled amount for %s", value)
	}
	return scaled.BigInt().Uint64(), nil
}

func sendSignedTransaction(
	ctx context.Context,
	client *rpc.Client,
	payer solana.PublicKey,
	instructions []solana.Instruction,
	signers []solana.PrivateKey,
) (solana.Signature, error) {
	latestBlockhash, err := client.GetLatestBlockhash(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("get latest blockhash: %w", err)
	}

	tx, err := solana.NewTransaction(
		instructions,
		latestBlockhash.Value.Blockhash,
		solana.TransactionPayer(payer),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("build transaction: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		for _, signer := range signers {
			if signer.PublicKey().Equals(key) {
				return &signer
			}
		}
		return nil
	})
	if err != nil {
		return solana.Signature{}, fmt.Errorf("sign transaction: %w", err)
	}

	sig, err := client.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{
		SkipPreflight:       false,
		PreflightCommitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return solana.Signature{}, err
	}
	return sig, nil
}

func validatePoolAccounts(
	ctx context.Context,
	client *rpc.Client,
	swapAuthority solana.PublicKey,
	usdcMint solana.PublicKey,
	wsolVault solana.PublicKey,
	usdcVault solana.PublicKey,
	poolMint solana.PublicKey,
	poolFeeAccount solana.PublicKey,
	poolDestination solana.PublicKey,
) error {
	wsolVaultState, err := fetchTokenAccount(ctx, client, wsolVault)
	if err != nil {
		return fmt.Errorf("fetch wsol vault: %w", err)
	}
	usdcVaultState, err := fetchTokenAccount(ctx, client, usdcVault)
	if err != nil {
		return fmt.Errorf("fetch usdc vault: %w", err)
	}
	poolFeeState, err := fetchTokenAccount(ctx, client, poolFeeAccount)
	if err != nil {
		return fmt.Errorf("fetch pool fee account: %w", err)
	}
	poolDestinationState, err := fetchTokenAccount(ctx, client, poolDestination)
	if err != nil {
		return fmt.Errorf("fetch pool destination: %w", err)
	}
	poolMintState, err := fetchMint(ctx, client, poolMint)
	if err != nil {
		return fmt.Errorf("fetch pool mint: %w", err)
	}

	printJSON("pre-initialize account state", map[string]interface{}{
		"swapAuthority": swapAuthority.String(),
		"wsolVault": map[string]interface{}{
			"address":        wsolVault.String(),
			"mint":           wsolVaultState.Mint.String(),
			"owner":          wsolVaultState.Owner.String(),
			"amount":         wsolVaultState.Amount,
			"isNative":       wsolVaultState.IsNative != nil,
			"delegate":       publicKeyString(wsolVaultState.Delegate),
			"closeAuthority": publicKeyString(wsolVaultState.CloseAuthority),
		},
		"usdcVault": map[string]interface{}{
			"address":        usdcVault.String(),
			"mint":           usdcVaultState.Mint.String(),
			"owner":          usdcVaultState.Owner.String(),
			"amount":         usdcVaultState.Amount,
			"delegate":       publicKeyString(usdcVaultState.Delegate),
			"closeAuthority": publicKeyString(usdcVaultState.CloseAuthority),
		},
		"poolMint": map[string]interface{}{
			"address":         poolMint.String(),
			"supply":          poolMintState.Supply,
			"decimals":        poolMintState.Decimals,
			"isInitialized":   poolMintState.IsInitialized,
			"mintAuthority":   publicKeyString(poolMintState.MintAuthority),
			"freezeAuthority": publicKeyString(poolMintState.FreezeAuthority),
		},
		"poolFeeAccount": map[string]interface{}{
			"address":        poolFeeAccount.String(),
			"mint":           poolFeeState.Mint.String(),
			"owner":          poolFeeState.Owner.String(),
			"amount":         poolFeeState.Amount,
			"delegate":       publicKeyString(poolFeeState.Delegate),
			"closeAuthority": publicKeyString(poolFeeState.CloseAuthority),
		},
		"poolDestination": map[string]interface{}{
			"address":        poolDestination.String(),
			"mint":           poolDestinationState.Mint.String(),
			"owner":          poolDestinationState.Owner.String(),
			"amount":         poolDestinationState.Amount,
			"delegate":       publicKeyString(poolDestinationState.Delegate),
			"closeAuthority": publicKeyString(poolDestinationState.CloseAuthority),
		},
	})

	if !wsolVaultState.Owner.Equals(swapAuthority) {
		return fmt.Errorf("wsol vault owner mismatch: %s", wsolVaultState.Owner)
	}
	if !wsolVaultState.Mint.Equals(solana.WrappedSol) {
		return fmt.Errorf("wsol vault mint mismatch: %s", wsolVaultState.Mint)
	}
	if wsolVaultState.Amount == 0 {
		return errors.New("wsol vault has zero balance")
	}
	if !usdcVaultState.Owner.Equals(swapAuthority) {
		return fmt.Errorf("usdc vault owner mismatch: %s", usdcVaultState.Owner)
	}
	if !usdcVaultState.Mint.Equals(usdcMint) {
		return fmt.Errorf("usdc vault mint mismatch: %s", usdcVaultState.Mint)
	}
	if usdcVaultState.Amount == 0 {
		return errors.New("usdc vault has zero balance")
	}
	if wsolVaultState.Delegate != nil || usdcVaultState.Delegate != nil {
		return errors.New("vault delegate must be empty")
	}
	if wsolVaultState.CloseAuthority != nil || usdcVaultState.CloseAuthority != nil {
		return errors.New("vault close authority must be empty")
	}
	if !poolMintState.IsInitialized {
		return errors.New("pool mint is not initialized")
	}
	if poolMintState.Supply != 0 {
		return fmt.Errorf("pool mint supply must be zero before initialize, got %d", poolMintState.Supply)
	}
	if poolMintState.MintAuthority == nil || !poolMintState.MintAuthority.Equals(swapAuthority) {
		return fmt.Errorf("pool mint authority mismatch: %s", publicKeyString(poolMintState.MintAuthority))
	}
	if poolMintState.FreezeAuthority != nil {
		return fmt.Errorf("pool mint freeze authority must be empty, got %s", publicKeyString(poolMintState.FreezeAuthority))
	}
	if !poolFeeState.Mint.Equals(poolMint) {
		return fmt.Errorf("pool fee account mint mismatch: %s", poolFeeState.Mint)
	}
	if !poolDestinationState.Mint.Equals(poolMint) {
		return fmt.Errorf("pool destination mint mismatch: %s", poolDestinationState.Mint)
	}
	if poolFeeState.Delegate != nil || poolDestinationState.Delegate != nil {
		return errors.New("pool token delegate must be empty")
	}
	if poolFeeState.CloseAuthority != nil || poolDestinationState.CloseAuthority != nil {
		return errors.New("pool token close authority must be empty")
	}
	return nil
}

func fetchTokenAccount(ctx context.Context, client *rpc.Client, address solana.PublicKey) (*token.Account, error) {
	accountInfo, err := client.GetAccountInfoWithOpts(ctx, address, &rpc.GetAccountInfoOpts{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return nil, err
	}
	var state token.Account
	if err := state.UnmarshalWithDecoder(binDecoder(accountInfo.Value.Data.GetBinary())); err != nil {
		return nil, err
	}
	return &state, nil
}

func fetchMint(ctx context.Context, client *rpc.Client, address solana.PublicKey) (*token.Mint, error) {
	accountInfo, err := client.GetAccountInfoWithOpts(ctx, address, &rpc.GetAccountInfoOpts{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return nil, err
	}
	var state token.Mint
	if err := state.UnmarshalWithDecoder(binDecoder(accountInfo.Value.Data.GetBinary())); err != nil {
		return nil, err
	}
	return &state, nil
}

func binDecoder(data []byte) *bin.Decoder {
	return bin.NewBinDecoder(data)
}

func publicKeyString(key *solana.PublicKey) string {
	if key == nil {
		return ""
	}
	return key.String()
}

func waitForConfirmation(ctx context.Context, client *rpc.Client, sig solana.Signature) error {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for {
		statuses, err := client.GetSignatureStatuses(ctx, true, sig)
		if err != nil {
			return err
		}
		if len(statuses.Value) > 0 && statuses.Value[0] != nil {
			status := statuses.Value[0]
			if status.Err != nil {
				return fmt.Errorf("transaction failed: %v", status.Err)
			}
			switch status.ConfirmationStatus {
			case rpc.ConfirmationStatusConfirmed, rpc.ConfirmationStatusFinalized:
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func printJSON(label string, v interface{}) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("%s: %#v\n", label, v)
		return
	}
	fmt.Printf("%s:\n%s\n", label, string(body))
}

func writeJSONFile(path string, v interface{}) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func exitf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

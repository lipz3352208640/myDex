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

	"github.com/gagliardetto/solana-go"
	ata "github.com/gagliardetto/solana-go/programs/associated-token-account"
	system "github.com/gagliardetto/solana-go/programs/system"
	token "github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
)

const tokenAccountSize = 165

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
		configPath = flag.String("config", "", "path to the token swap pool config JSON")
		rpcURL     = flag.String("rpc-url", "", "optional RPC endpoint override")
		amountUI   = flag.String("amount-ui", "0.01", "SOL amount to swap into USDC")
		minOutUI   = flag.String("min-out-ui", "0", "minimum USDC amount out in UI units")
	)
	flag.Parse()

	if *configPath == "" {
		exitf("missing required flag: --config")
	}

	privateKeyBase58 := firstNonEmpty(os.Getenv("PRIVATE_KEY"), os.Getenv("private_key"))
	if privateKeyBase58 == "" {
		exitf("missing PRIVATE_KEY/private_key environment variable")
	}

	payer, err := solana.PrivateKeyFromBase58(privateKeyBase58)
	if err != nil {
		exitf("invalid private key: %v", err)
	}

	cfg, err := loadPoolConfig(*configPath)
	if err != nil {
		exitf("load pool config: %v", err)
	}

	endpoint := cfg.RPCURL
	if strings.TrimSpace(*rpcURL) != "" {
		endpoint = *rpcURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := rpc.New(endpoint)

	swapAmountLamports, err := parseUIAmount(*amountUI, 9)
	if err != nil {
		exitf("invalid --amount-ui: %v", err)
	}
	minOutAmount, err := parseUIAmount(*minOutUI, cfg.UsdcDecimals)
	if err != nil {
		exitf("invalid --min-out-ui: %v", err)
	}

	usdcMint := solana.MustPublicKeyFromBase58(cfg.UsdcMint)
	swapAccount := solana.MustPublicKeyFromBase58(cfg.SwapAccount)
	swapAuthority := solana.MustPublicKeyFromBase58(cfg.SwapAuthority)
	wsolVault := solana.MustPublicKeyFromBase58(cfg.WsolVault)
	usdcVault := solana.MustPublicKeyFromBase58(cfg.UsdcVault)
	poolMint := solana.MustPublicKeyFromBase58(cfg.PoolMint)
	poolFeeAccount := solana.MustPublicKeyFromBase58(cfg.PoolFeeAccount)

	userUsdcATA, _, err := solana.FindAssociatedTokenAddress(payer.PublicKey(), usdcMint)
	if err != nil {
		exitf("derive user USDC ATA: %v", err)
	}

	tempWsol := solana.NewWallet()
	tokenAccountRent, err := client.GetMinimumBalanceForRentExemption(ctx, tokenAccountSize, rpc.CommitmentConfirmed)
	if err != nil {
		exitf("rent exemption for temp WSOL account: %v", err)
	}

	printJSON("swap request", map[string]interface{}{
		"payer":          payer.PublicKey().String(),
		"swapAccount":    swapAccount.String(),
		"swapAuthority":  swapAuthority.String(),
		"userUsdcATA":    userUsdcATA.String(),
		"tempWsol":       tempWsol.PublicKey().String(),
		"amountLamports": swapAmountLamports,
		"minOutAmount":   minOutAmount,
	})

	instructions, err := buildSwapInstructions(
		payer.PublicKey(),
		tempWsol.PublicKey(),
		userUsdcATA,
		usdcMint,
		swapAccount,
		swapAuthority,
		wsolVault,
		usdcVault,
		poolMint,
		poolFeeAccount,
		swapAmountLamports,
		minOutAmount,
		tokenAccountRent,
	)
	if err != nil {
		exitf("build swap instructions: %v", err)
	}

	sig, err := sendSignedTransaction(ctx, client, payer.PublicKey(), instructions, []solana.PrivateKey{
		payer,
		tempWsol.PrivateKey,
	})
	if err != nil {
		exitf("send swap transaction: %v", err)
	}

	fmt.Printf("submitted swap transaction: %s\n", sig.String())
	if err := waitForConfirmation(ctx, client, sig); err != nil {
		exitf("swap transaction %s not confirmed: %v", sig.String(), err)
	}
	fmt.Printf("swap completed successfully: %s\n", sig.String())
}

func buildSwapInstructions(
	payer solana.PublicKey,
	tempWsol solana.PublicKey,
	userUsdcATA solana.PublicKey,
	usdcMint solana.PublicKey,
	swapAccount solana.PublicKey,
	swapAuthority solana.PublicKey,
	wsolVault solana.PublicKey,
	usdcVault solana.PublicKey,
	poolMint solana.PublicKey,
	poolFeeAccount solana.PublicKey,
	swapAmountLamports uint64,
	minOutAmount uint64,
	tokenAccountRent uint64,
) ([]solana.Instruction, error) {
	createUserATA, err := ata.NewCreateIdempotentInstructionBuilder().
		SetPayer(payer).
		SetWallet(payer).
		SetMint(usdcMint).
		ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	createTempWsol, err := system.NewCreateAccountInstruction(
		tokenAccountRent+swapAmountLamports,
		tokenAccountSize,
		solana.TokenProgramID,
		payer,
		tempWsol,
	).ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	initTempWsol, err := token.NewInitializeAccount3Instruction(
		payer,
		tempWsol,
		solana.WrappedSol,
	).ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	syncWsol, err := token.NewSyncNativeInstruction(tempWsol).ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	swapInstruction := newSwapInstruction(
		swapAccount,
		swapAuthority,
		payer,
		tempWsol,
		wsolVault,
		usdcVault,
		userUsdcATA,
		poolMint,
		poolFeeAccount,
		swapAmountLamports,
		minOutAmount,
	)

	closeTempWsol, err := token.NewCloseAccountInstruction(
		tempWsol,
		payer,
		payer,
		nil,
	).ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	return []solana.Instruction{
		createUserATA,
		createTempWsol,
		initTempWsol,
		syncWsol,
		swapInstruction,
		closeTempWsol,
	}, nil
}

func newSwapInstruction(
	swapAccount solana.PublicKey,
	swapAuthority solana.PublicKey,
	userTransferAuthority solana.PublicKey,
	source solana.PublicKey,
	swapSource solana.PublicKey,
	swapDestination solana.PublicKey,
	destination solana.PublicKey,
	poolMint solana.PublicKey,
	poolFeeAccount solana.PublicKey,
	amountIn uint64,
	minimumAmountOut uint64,
) solana.Instruction {
	data := make([]byte, 0, 17)
	data = append(data, 1)
	data = append(data, le64(amountIn)...)
	data = append(data, le64(minimumAmountOut)...)

	return &tokenSwapInstruction{
		programID: solana.TokenSwapProgramID,
		accounts: solana.AccountMetaSlice{
			solana.Meta(swapAccount),
			solana.Meta(swapAuthority),
			solana.Meta(userTransferAuthority).SIGNER(),
			solana.Meta(source).WRITE(),
			solana.Meta(swapSource).WRITE(),
			solana.Meta(swapDestination).WRITE(),
			solana.Meta(destination).WRITE(),
			solana.Meta(poolMint).WRITE(),
			solana.Meta(poolFeeAccount).WRITE(),
			solana.Meta(solana.TokenProgramID),
		},
		data: data,
	}
}

func le64(v uint64) []byte {
	return []byte{
		byte(v),
		byte(v >> 8),
		byte(v >> 16),
		byte(v >> 24),
		byte(v >> 32),
		byte(v >> 40),
		byte(v >> 48),
		byte(v >> 56),
	}
}

func loadPoolConfig(path string) (*poolConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg poolConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func parseUIAmount(value string, decimals uint8) (uint64, error) {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return 0, err
	}
	if d.LessThanOrEqual(decimal.Zero) {
		if d.Equal(decimal.Zero) {
			return 0, nil
		}
		return 0, errors.New("amount must be positive")
	}
	base := decimal.NewFromInt(10).Pow(decimal.NewFromInt32(int32(decimals)))
	scaled := d.Mul(base)
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, fmt.Errorf("amount %s has more than %d decimals", value, decimals)
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

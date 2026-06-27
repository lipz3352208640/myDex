package solana

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"myDex/pkg/constant"
	"myDex/trade/internal/chain/solana/entity"

	aSDK "github.com/gagliardetto/solana-go"
	system "github.com/gagliardetto/solana-go/programs/system"
	token "github.com/gagliardetto/solana-go/programs/token"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
)

type tokenSwapInstruction struct {
	programID aSDK.PublicKey
	accounts  aSDK.AccountMetaSlice
	data      []byte
}

func (i *tokenSwapInstruction) ProgramID() aSDK.PublicKey     { return i.programID }
func (i *tokenSwapInstruction) Accounts() []*aSDK.AccountMeta { return i.accounts }
func (i *tokenSwapInstruction) Data() ([]byte, error)         { return i.data, nil }

func LoadTokenSwapPoolConfig(path string) (*entity.TokenSwapPoolConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg entity.TokenSwapPoolConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (tm *TxManager) BuildUnsignedTransactionPumpfunWithTokenSwap(
	ctx context.Context,
	createMarketTx *entity.MarketTx,
	poolCfg *entity.TokenSwapPoolConfig,
	swapAmountUI string,
	minOutUI string,
) (string, error) {
	unsignedTx, _, err := tm.BuildUnsignedTransactionPumpfunWithTokenSwapWithResult(ctx, createMarketTx, poolCfg, swapAmountUI, minOutUI)
	return unsignedTx, err
}

func (tm *TxManager) BuildUnsignedTransactionPumpfunWithTokenSwapWithResult(
	ctx context.Context,
	createMarketTx *entity.MarketTx,
	poolCfg *entity.TokenSwapPoolConfig,
	swapAmountUI string,
	minOutUI string,
) (string, uint64, error) {
	in, err := convertMarketTx(createMarketTx)
	if err != nil {
		return "", 0, err
	}

	if in.TradePoolName != constant.PumpFunName {
		return "", 0, fmt.Errorf("unsupported combo trade pool: %s", in.TradePoolName)
	}

	instructions, err := tm.CreateMarketOrderPumpfun(ctx, in)
	if err != nil {
		return "", 0, err
	}

	swapInstructions, err := tm.BuildTokenSwapInstructionsForUser(context.Background(), in.UserWalletAddress, poolCfg, swapAmountUI, minOutUI)
	if err != nil {
		return "", 0, err
	}
	instructions = append(instructions, swapInstructions...)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := tm.Client.GetLatestBlockhash(timeoutCtx, ag_rpc.CommitmentProcessed)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get latest blockhash: %w", err)
	}
	fmt.Println("lllll")
	tx, err := aSDK.NewTransaction(instructions, resp.Value.Blockhash, aSDK.TransactionPayer(in.UserWalletAddress))
	if err != nil {
		return "", 0, err
	}

	numSigners := int(tx.Message.Header.NumRequiredSignatures)
	tx.Signatures = make([]aSDK.Signature, numSigners)

	txData, err := tx.MarshalBinary()
	if err != nil {
		return "", 0, err
	}

	return base64.StdEncoding.EncodeToString(txData), resp.Value.LastValidBlockHeight, nil
}

func (tm *TxManager) BuildTokenSwapInstructionsForUser(
	ctx context.Context,
	payer aSDK.PublicKey,
	poolCfg *entity.TokenSwapPoolConfig,
	swapAmountUI string,
	minOutUI string,
) ([]aSDK.Instruction, error) {
	if poolCfg == nil {
		return nil, fmt.Errorf("token swap pool config is nil")
	}

	swapAmountLamports, err := parseUIAmount(swapAmountUI, 9)
	if err != nil {
		return nil, fmt.Errorf("invalid swap amount: %w", err)
	}
	minOutAmount, err := parseUIAmount(minOutUI, poolCfg.UsdcDecimals)
	if err != nil {
		return nil, fmt.Errorf("invalid min out amount: %w", err)
	}

	usdcMint := aSDK.MustPublicKeyFromBase58(poolCfg.UsdcMint)
	swapAccount := aSDK.MustPublicKeyFromBase58(poolCfg.SwapAccount)
	swapAuthority := aSDK.MustPublicKeyFromBase58(poolCfg.SwapAuthority)
	wsolVault := aSDK.MustPublicKeyFromBase58(poolCfg.WsolVault)
	usdcVault := aSDK.MustPublicKeyFromBase58(poolCfg.UsdcVault)
	poolMint := aSDK.MustPublicKeyFromBase58(poolCfg.PoolMint)
	poolFeeAccount := aSDK.MustPublicKeyFromBase58(poolCfg.PoolFeeAccount)

	userUsdcATA, _, err := aSDK.FindAssociatedTokenAddress(payer, usdcMint)
	if err != nil {
		return nil, fmt.Errorf("derive user USDC ATA: %w", err)
	}
	userWsolATA, _, err := aSDK.FindAssociatedTokenAddress(payer, aSDK.WrappedSol)
	if err != nil {
		return nil, fmt.Errorf("derive user WSOL ATA: %w", err)
	}

	if err := tm.requireAccountExists(ctx, userUsdcATA, "user USDC ATA"); err != nil {
		return nil, err
	}
	if err := tm.requireAccountExists(ctx, userWsolATA, "user WSOL ATA"); err != nil {
		return nil, err
	}

	fundWsolATA, err := system.NewTransferInstruction(
		swapAmountLamports,
		payer,
		userWsolATA,
	).ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	syncWsol, err := token.NewSyncNativeInstruction(userWsolATA).ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	tm.Infof(
		"combo token swap request: payer=%s swapAccount=%s swapAuthority=%s userWsolATA=%s userUsdcATA=%s amountLamports=%d minOutAmount=%d",
		payer.String(),
		swapAccount.String(),
		swapAuthority.String(),
		userWsolATA.String(),
		userUsdcATA.String(),
		swapAmountLamports,
		minOutAmount,
	)

	swapInstruction := newLegacyTokenSwapInstruction(
		swapAccount,
		swapAuthority,
		payer,
		userWsolATA,
		wsolVault,
		usdcVault,
		userUsdcATA,
		poolMint,
		poolFeeAccount,
		swapAmountLamports,
		minOutAmount,
	)

	return []aSDK.Instruction{
		fundWsolATA,
		syncWsol,
		swapInstruction,
	}, nil
}

func (tm *TxManager) requireAccountExists(ctx context.Context, account aSDK.PublicKey, label string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := tm.Client.GetAccountInfoWithOpts(timeoutCtx, account, &ag_rpc.GetAccountInfoOpts{
		Encoding:   aSDK.EncodingBase64,
		Commitment: ag_rpc.CommitmentConfirmed,
	})
	if err != nil {
		return fmt.Errorf("fetch %s %s: %w", label, account.String(), err)
	}
	if out == nil || out.Value == nil {
		return fmt.Errorf("%s %s does not exist; create it before sending the combo transaction", label, account.String())
	}
	return nil
}

func newLegacyTokenSwapInstruction(
	swapAccount aSDK.PublicKey,
	swapAuthority aSDK.PublicKey,
	userTransferAuthority aSDK.PublicKey,
	source aSDK.PublicKey,
	swapSource aSDK.PublicKey,
	swapDestination aSDK.PublicKey,
	destination aSDK.PublicKey,
	poolMint aSDK.PublicKey,
	poolFeeAccount aSDK.PublicKey,
	amountIn uint64,
	minimumAmountOut uint64,
) aSDK.Instruction {
	data := make([]byte, 0, 17)
	data = append(data, 1)
	data = append(data, le64(amountIn)...)
	data = append(data, le64(minimumAmountOut)...)

	return &tokenSwapInstruction{
		programID: aSDK.TokenSwapProgramID,
		accounts: aSDK.AccountMetaSlice{
			aSDK.Meta(swapAccount),
			aSDK.Meta(swapAuthority),
			aSDK.Meta(userTransferAuthority).SIGNER(),
			aSDK.Meta(source).WRITE(),
			aSDK.Meta(swapSource).WRITE(),
			aSDK.Meta(swapDestination).WRITE(),
			aSDK.Meta(destination).WRITE(),
			aSDK.Meta(poolMint).WRITE(),
			aSDK.Meta(poolFeeAccount).WRITE(),
			aSDK.Meta(aSDK.TokenProgramID),
		},
		data: data,
	}
}

func parseUIAmount(value string, decimals uint8) (uint64, error) {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return 0, err
	}
	base := d.Shift(int32(decimals))
	if !base.IsInteger() {
		base = base.Truncate(0)
	}
	if base.IsNegative() {
		return 0, fmt.Errorf("amount must be non-negative")
	}
	return uint64(base.IntPart()), nil
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

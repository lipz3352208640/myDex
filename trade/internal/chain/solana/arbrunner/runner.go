package arbrunner

import (
	"context"
	"fmt"

	"myDex/trade/internal/chain/solana/arbitrage"
	"myDex/trade/internal/chain/solana/jupiterarb"

	"github.com/gagliardetto/solana-go"
)

type TransactionBuilder interface {
	BuildVersionedTransaction(
		ctx context.Context,
		payer solana.PublicKey,
		instructions []solana.Instruction,
		tableKeys []solana.PublicKey,
	) (*solana.Transaction, error)
	SimulateAnyTransaction(ctx context.Context, tx *solana.Transaction) error
	SignTransactionWithServiceKey(tx *solana.Transaction) error
	SendJitoBundle(ctx context.Context, jitoEndpoint string, txs []*solana.Transaction) (string, error)
}

type Runner struct {
	detector  *arbitrage.Detector
	builder   *jupiterarb.Builder
	txBuilder TransactionBuilder
	jitoURL   string
}

type ExecuteRequest struct {
	StartMint        solana.PublicKey
	MidMint          solana.PublicKey
	Amount           uint64
	SlippageBps      uint32
	MaxAccounts      uint32
	Payer            solana.PublicKey
	JitoTipRecipient *solana.PublicKey
	SimulateOnly     bool
}

type ExecuteResult struct {
	Opportunity *arbitrage.Opportunity
	Built       *jupiterarb.AtomicSwapBuildResult
	Tx          *solana.Transaction
	BundleID    string
}

func NewRunner(detector *arbitrage.Detector, builder *jupiterarb.Builder, txBuilder TransactionBuilder, jitoURL string) *Runner {
	return &Runner{
		detector:  detector,
		builder:   builder,
		txBuilder: txBuilder,
		jitoURL:   jitoURL,
	}
}

func (r *Runner) ExecuteOnce(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error) {
	if r.detector == nil || r.builder == nil || r.txBuilder == nil {
		return nil, fmt.Errorf("runner dependencies are incomplete")
	}
	if req == nil {
		return nil, fmt.Errorf("execute request is nil")
	}

	opp, err := r.detector.DetectLoop(ctx, req.StartMint, req.MidMint, req.Amount, req.SlippageBps, req.MaxAccounts)
	if err != nil {
		return nil, err
	}
	if !opp.ShouldExecute {
		return &ExecuteResult{Opportunity: opp}, nil
	}

	built, err := r.builder.BuildAtomicLoopInstructions(ctx, &jupiterarb.BuildRequest{
		Payer:            req.Payer,
		Opportunity:      opp,
		JitoTipRecipient: req.JitoTipRecipient,
	})
	if err != nil {
		return nil, err
	}

	tx, err := r.txBuilder.BuildVersionedTransaction(ctx, req.Payer, built.Instructions, built.LookupTableKeys)
	if err != nil {
		return nil, err
	}

	if err := r.txBuilder.SimulateAnyTransaction(ctx, tx); err != nil {
		return nil, err
	}

	result := &ExecuteResult{
		Opportunity: opp,
		Built:       built,
		Tx:          tx,
	}
	if req.SimulateOnly {
		return result, nil
	}

	if err := r.txBuilder.SignTransactionWithServiceKey(tx); err != nil {
		return nil, err
	}

	//https://frankfurt.mainnet.block-engine.jito.wtf/api/v1/bundles 发送bundle 交易
	bundleID, err := r.txBuilder.SendJitoBundle(ctx, r.jitoURL, []*solana.Transaction{tx})
	if err != nil {
		return nil, err
	}
	result.BundleID = bundleID
	return result, nil
}

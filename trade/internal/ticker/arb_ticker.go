package ticker

import (
	"context"
	"errors"
	"os"
	"time"

	"myDex/trade/internal/chain/solana/arbrunner"
	"myDex/trade/internal/svc"

	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
	"github.com/zeromicro/go-zero/core/logx"
)

//套利Arbitrage 服务
type ArbTicker struct {
	ctx    context.Context
	cancel func(err error)
	logx.Logger
	svcCtx *svc.ServiceContext
}

func NewArbTicker(svcCtx *svc.ServiceContext) *ArbTicker {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &ArbTicker{
		ctx:    ctx,
		cancel: cancel,
		Logger: logx.WithContext(ctx).WithFields(logx.LogField{Key: "service", Value: "arbTicker"}),
		svcCtx: svcCtx,
	}
}

func (t *ArbTicker) Start() {
	cfg := t.svcCtx.Config.Arbitrage
	if !cfg.Enabled {
		t.Infof("arb ticker disabled")
		return
	}

	//支付账户
	payer, err := serviceWalletPublicKey()
	if err != nil {
		t.Errorf("arb ticker disabled, resolve payer failed: %v", err)
		return
	}

	//base mint
	startMint, err := solana.PublicKeyFromBase58(cfg.StartMint)
	if err != nil {
		t.Errorf("arb ticker disabled, invalid start mint: %v", err)
		return
	}
	//token mint
	midMint, err := solana.PublicKeyFromBase58(cfg.MidMint)
	if err != nil {
		t.Errorf("arb ticker disabled, invalid mid mint: %v", err)
		return
	}

	//jito tip 接收账户
	var tipRecipient *solana.PublicKey
	if cfg.JitoTipRecipient != "" {
		pubkey, err := solana.PublicKeyFromBase58(cfg.JitoTipRecipient)
		if err != nil {
			t.Errorf("arb ticker disabled, invalid jito tip recipient: %v", err)
			return
		}
		tipRecipient = &pubkey
	}

	interval := time.Duration(cfg.IntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}

	t.Infof("arb ticker started, payer=%s startMint=%s midMint=%s amount=%d interval=%s simulateOnly=%v",
		payer.String(), startMint.String(), midMint.String(), cfg.AmountLamports, interval.String(), t.svcCtx.Config.SimulateOnly)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			t.Infof("arb ticker context done")
			return
		case <-ticker.C:
			result, err := t.svcCtx.ArbRunner.ExecuteOnce(t.ctx, &arbrunner.ExecuteRequest{
				StartMint:        startMint,
				MidMint:          midMint,
				Amount:           cfg.AmountLamports,
				SlippageBps:      cfg.SlippageBps,
				MaxAccounts:      cfg.MaxAccounts,
				Payer:            payer,
				JitoTipRecipient: tipRecipient,
				SimulateOnly:     t.svcCtx.Config.SimulateOnly,
			})
			if err != nil {
				t.Errorf("arb execute failed: %v", err)
				continue
			}
			if result == nil || result.Opportunity == nil {
				continue
			}
			if !result.Opportunity.ShouldExecute {
				t.Infof("arb skipped, grossProfit=%d threshold=%d", result.Opportunity.GrossProfitLamports, result.Opportunity.ThresholdLamports)
				continue
			}
			if result.BundleID != "" {
				t.Infof("arb bundle sent, bundleID=%s grossProfit=%d tip=%d", result.BundleID, result.Opportunity.GrossProfitLamports, result.Opportunity.SuggestedJitoTip)
				continue
			}
			t.Infof("arb simulated, grossProfit=%d tip=%d", result.Opportunity.GrossProfitLamports, result.Opportunity.SuggestedJitoTip)
		}
	}
}

func (t *ArbTicker) Stop() {
	t.Infof("arb ticker stopped")
	t.cancel(errors.New("arb ticker stopped"))
}

func serviceWalletPublicKey() (solana.PublicKey, error) {
	privateKeyBase58 := os.Getenv("private_key")
	if privateKeyBase58 == "" {
		return solana.PublicKey{}, errors.New("private key not set in environment variable")
	}
	privateKeyBytes, err := base58.Decode(privateKeyBase58)
	if err != nil {
		return solana.PublicKey{}, err
	}
	privateKey := solana.PrivateKey(privateKeyBytes)
	return privateKey.PublicKey(), nil
}

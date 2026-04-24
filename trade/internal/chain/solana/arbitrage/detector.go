package arbitrage

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

type Detector struct {
	quoteProvider QuoteProvider
	threshold     int64
	tipBps        uint64
}

func NewDetector(quoteProvider QuoteProvider, threshold int64, tipBps uint64) *Detector {
	return &Detector{
		quoteProvider: quoteProvider,
		threshold:     threshold,
		tipBps:        tipBps,
	}
}

func (d *Detector) DetectLoop(
	ctx context.Context,
	startMint solana.PublicKey,
	midMint solana.PublicKey,
	amount uint64,
	slippageBps uint32,
	maxAccounts uint32,
) (*Opportunity, error) {
	_ = ctx

	if d.quoteProvider == nil {
		return nil, fmt.Errorf("quote provider is nil")
	}

	//first quote 第一笔报价
	firstQuote, err := d.quoteProvider.GetQuote(&QuoteRequest{
		InputMint:        startMint,
		OutputMint:       midMint,
		Amount:           amount,
		SlippageBps:      slippageBps,
		MaxAccounts:      maxAccounts,
		OnlyDirectRoutes: false,
	})
	if err != nil {
		return nil, fmt.Errorf("get first quote: %w", err)
	}

	//second quote 第二笔报价
	secondQuote, err := d.quoteProvider.GetQuote(&QuoteRequest{
		InputMint:        midMint,
		OutputMint:       startMint,
		Amount:           firstQuote.OutAmount,
		SlippageBps:      slippageBps,
		MaxAccounts:      maxAccounts,
		OnlyDirectRoutes: false,
	})
	if err != nil {
		return nil, fmt.Errorf("get second quote: %w", err)
	}

	profit := int64(secondQuote.OutAmount) - int64(amount)
	tip := calculateTip(profit, d.tipBps)
	targetOut := amount + tip

	return &Opportunity{
		FirstQuote:          firstQuote,
		SecondQuote:         secondQuote,
		InputAmount:         amount,
		FinalOutputAmount:   secondQuote.OutAmount,
		GrossProfitLamports: profit,
		SuggestedJitoTip:    tip,
		TargetOutAmount:     targetOut,
		ThresholdLamports:   d.threshold,
		ShouldExecute:       profit > d.threshold,
	}, nil
}

func calculateTip(profit int64, tipBps uint64) uint64 {
	if profit <= 0 || tipBps == 0 {
		return 0
	}
	return uint64(profit) * tipBps / 10000
}

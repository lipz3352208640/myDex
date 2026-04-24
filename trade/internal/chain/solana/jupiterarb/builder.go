package jupiterarb

import (
	"context"
	"encoding/base64"
	"fmt"

	"myDex/trade/internal/chain/solana/arbitrage"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	system "github.com/gagliardetto/solana-go/programs/system"
)

type Builder struct {
	swapProvider SwapInstructionProvider
}

func NewBuilder(swapProvider SwapInstructionProvider) *Builder {
	return &Builder{swapProvider: swapProvider}
}

func (b *Builder) BuildAtomicLoopInstructions(
	ctx context.Context,
	req *BuildRequest,
) (*AtomicSwapBuildResult, error) {
	if b.swapProvider == nil {
		return nil, fmt.Errorf("swap instruction provider is nil")
	}
	if req == nil || req.Opportunity == nil {
		return nil, fmt.Errorf("build request or opportunity is nil")
	}
	if req.Opportunity.FirstQuote == nil || req.Opportunity.SecondQuote == nil {
		return nil, fmt.Errorf("opportunity quotes are incomplete")
	}

	mergedQuote := MergeQuotes(req.Opportunity)
	swapResp, err := b.swapProvider.GetSwapInstructions(ctx, &SwapRequest{
		UserPublicKey:                 req.Payer,
		WrapAndUnwrapSol:              false,
		UseSharedAccounts:             false,
		ComputeUnitPriceMicroLamports: 1,
		DynamicComputeUnitLimit:       true,
		SkipUserAccountsRPCCalls:      true,
		QuoteResponse:                 mergedQuote,
	})
	if err != nil {
		return nil, fmt.Errorf("get swap instructions: %w", err)
	}

	var instructions []solana.Instruction

	if swapResp.ComputeUnitLimit > 0 {
		cuIx, err := computebudget.NewSetComputeUnitLimitInstruction(swapResp.ComputeUnitLimit).ValidateAndBuild()
		if err != nil {
			return nil, fmt.Errorf("build compute unit limit instruction: %w", err)
		}
		instructions = append(instructions, cuIx)
	}

	encoded := append([]EncodedInstruction{}, swapResp.ComputeBudgetInstructions...)
	encoded = append(encoded, swapResp.SetupInstructions...)
	for _, ix := range encoded {
		decoded, err := decodeInstruction(ix)
		if err != nil {
			return nil, err
		}
		instructions = append(instructions, decoded)
	}

	swapIx, err := decodeInstruction(swapResp.SwapInstruction)
	if err != nil {
		return nil, err
	}
	instructions = append(instructions, swapIx)

	if swapResp.CleanupInstruction != nil {
		cleanupIx, err := decodeInstruction(*swapResp.CleanupInstruction)
		if err != nil {
			return nil, err
		}
		instructions = append(instructions, cleanupIx)
	}

	if req.JitoTipRecipient != nil && req.Opportunity.SuggestedJitoTip > 0 {
		tipIx, err := system.NewTransferInstruction(
			req.Opportunity.SuggestedJitoTip,
			req.Payer,
			*req.JitoTipRecipient,
		).ValidateAndBuild()
		if err != nil {
			return nil, fmt.Errorf("build jito tip instruction: %w", err)
		}
		instructions = append(instructions, tipIx)
	}

	lookupTables, err := decodeLookupTableKeys(swapResp.AddressLookupTableAddresses)
	if err != nil {
		return nil, err
	}

	return &AtomicSwapBuildResult{
		Instructions:      instructions,
		LookupTableKeys:   lookupTables,
		ComputeUnitLimit:  swapResp.ComputeUnitLimit,
		ExpectedProfit:    req.Opportunity.GrossProfitLamports,
		JitoTipLamports:   req.Opportunity.SuggestedJitoTip,
		MergedQuote:       mergedQuote,
		InstructionBundle: swapResp,
	}, nil
}

func MergeQuotes(opp *arbitrage.Opportunity) *arbitrage.QuoteResponse {
	merged := cloneQuote(opp.FirstQuote)
	merged.OutputMint = opp.SecondQuote.OutputMint
	merged.OutAmount = opp.TargetOutAmount
	merged.OtherAmountThreshold = opp.TargetOutAmount
	merged.PriceImpactPct = "0"
	merged.RoutePlan = append(append([]arbitrage.RoutePlanStep{}, opp.FirstQuote.RoutePlan...), opp.SecondQuote.RoutePlan...)
	if merged.Raw != nil {
		merged.Raw["outputMint"] = merged.OutputMint.String()
		merged.Raw["outAmount"] = fmt.Sprintf("%d", merged.OutAmount)
		merged.Raw["otherAmountThreshold"] = fmt.Sprintf("%d", merged.OtherAmountThreshold)
		merged.Raw["priceImpactPct"] = merged.PriceImpactPct
		routePlan := make([]map[string]any, 0, len(merged.RoutePlan))
		for _, step := range merged.RoutePlan {
			routePlan = append(routePlan, step.Raw)
		}
		merged.Raw["routePlan"] = routePlan
	}
	return merged
}

func cloneQuote(in *arbitrage.QuoteResponse) *arbitrage.QuoteResponse {
	if in == nil {
		return nil
	}

	out := *in
	if in.RoutePlan != nil {
		out.RoutePlan = append([]arbitrage.RoutePlanStep{}, in.RoutePlan...)
	}
	if in.Raw != nil {
		out.Raw = make(map[string]any, len(in.Raw))
		for k, v := range in.Raw {
			out.Raw[k] = v
		}
	}
	return &out
}

func decodeInstruction(in EncodedInstruction) (solana.Instruction, error) {
	programID, err := solana.PublicKeyFromBase58(in.ProgramID)
	if err != nil {
		return nil, fmt.Errorf("parse instruction program id %q: %w", in.ProgramID, err)
	}

	data, err := base64.StdEncoding.DecodeString(in.Data)
	if err != nil {
		return nil, fmt.Errorf("decode instruction data: %w", err)
	}

	accounts := make([]*solana.AccountMeta, 0, len(in.Accounts))
	for _, account := range in.Accounts {
		pubkey, err := solana.PublicKeyFromBase58(account.Pubkey)
		if err != nil {
			return nil, fmt.Errorf("parse instruction account %q: %w", account.Pubkey, err)
		}

		meta := solana.Meta(pubkey)
		if account.IsWritable {
			meta.WRITE()
		}
		if account.IsSigner {
			meta.SIGNER()
		}
		accounts = append(accounts, meta)
	}

	return solana.NewInstruction(programID, accounts, data), nil
}

func decodeLookupTableKeys(encoded []string) ([]solana.PublicKey, error) {
	out := make([]solana.PublicKey, 0, len(encoded))
	for _, raw := range encoded {
		key, err := solana.PublicKeyFromBase58(raw)
		if err != nil {
			return nil, fmt.Errorf("parse lookup table address %q: %w", raw, err)
		}
		out = append(out, key)
	}
	return out, nil
}

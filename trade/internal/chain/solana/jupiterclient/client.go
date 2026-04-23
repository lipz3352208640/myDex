package jupiterclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"myDex/trade/internal/chain/solana/arbitrage"
	"myDex/trade/internal/chain/solana/jupiterarb"

	"github.com/gagliardetto/solana-go"
)

type Client struct {
	httpClient          *http.Client
	quoteURL            string
	swapInstructionsURL string
}

func NewClient(httpClient *http.Client, quoteURL, swapInstructionsURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}
	return &Client{
		httpClient:          httpClient,
		quoteURL:            quoteURL,
		swapInstructionsURL: swapInstructionsURL,
	}
}

func (c *Client) GetQuote(req *arbitrage.QuoteRequest) (*arbitrage.QuoteResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("quote request is nil")
	}
	if c.quoteURL == "" {
		return nil, fmt.Errorf("quote url is empty")
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.quoteURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create quote request: %w", err)
	}

	query := httpReq.URL.Query()
	query.Set("inputMint", req.InputMint.String())
	query.Set("outputMint", req.OutputMint.String())
	query.Set("amount", strconv.FormatUint(req.Amount, 10))
	query.Set("slippageBps", strconv.FormatUint(uint64(req.SlippageBps), 10))
	if req.OnlyDirectRoutes {
		query.Set("onlyDirectRoutes", "true")
	}
	if req.RestrictIntermediate {
		query.Set("restrictIntermediateTokens", "true")
	}
	if req.MaxAccounts > 0 {
		query.Set("maxAccounts", strconv.FormatUint(uint64(req.MaxAccounts), 10))
	}
	httpReq.URL.RawQuery = query.Encode()

	var raw map[string]any
	if err := c.doJSON(httpReq, &raw); err != nil {
		return nil, fmt.Errorf("request quote: %w", err)
	}

	return parseQuoteResponse(raw)
}

func (c *Client) GetSwapInstructions(ctx context.Context, req *jupiterarb.SwapRequest) (*jupiterarb.SwapInstructionsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("swap request is nil")
	}
	if req.QuoteResponse == nil {
		return nil, fmt.Errorf("quote response is nil")
	}
	if c.swapInstructionsURL == "" {
		return nil, fmt.Errorf("swap instructions url is empty")
	}

	payload := map[string]any{
		"userPublicKey":                 req.UserPublicKey.String(),
		"wrapAndUnwrapSol":              req.WrapAndUnwrapSol,
		"useSharedAccounts":             req.UseSharedAccounts,
		"computeUnitPriceMicroLamports": req.ComputeUnitPriceMicroLamports,
		"dynamicComputeUnitLimit":       req.DynamicComputeUnitLimit,
		"skipUserAccountsRpcCalls":      req.SkipUserAccountsRPCCalls,
		"quoteResponse":                 buildQuotePayload(req.QuoteResponse),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal swap instructions payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.swapInstructionsURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create swap instructions request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	var out jupiterarb.SwapInstructionsResponse
	if err := c.doJSON(httpReq, &out); err != nil {
		return nil, fmt.Errorf("request swap instructions: %w", err)
	}
	return &out, nil
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func parseQuoteResponse(raw map[string]any) (*arbitrage.QuoteResponse, error) {
	inputMint, err := getPublicKey(raw, "inputMint")
	if err != nil {
		return nil, err
	}
	outputMint, err := getPublicKey(raw, "outputMint")
	if err != nil {
		return nil, err
	}
	inAmount, err := getUint64(raw, "inAmount")
	if err != nil {
		return nil, err
	}
	outAmount, err := getUint64(raw, "outAmount")
	if err != nil {
		return nil, err
	}
	otherAmountThreshold, _ := getUint64(raw, "otherAmountThreshold")
	contextSlot, _ := getUint64(raw, "contextSlot")

	routePlan := make([]arbitrage.RoutePlanStep, 0)
	if items, ok := raw["routePlan"].([]any); ok {
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				routePlan = append(routePlan, arbitrage.RoutePlanStep{Raw: m})
			}
		}
	}

	priceImpactPct, _ := raw["priceImpactPct"].(string)

	return &arbitrage.QuoteResponse{
		InputMint:            inputMint,
		InAmount:             inAmount,
		OutputMint:           outputMint,
		OutAmount:            outAmount,
		OtherAmountThreshold: otherAmountThreshold,
		PriceImpactPct:       priceImpactPct,
		ContextSlot:          contextSlot,
		RoutePlan:            routePlan,
		Raw:                  raw,
	}, nil
}

func buildQuotePayload(quote *arbitrage.QuoteResponse) map[string]any {
	if quote == nil {
		return nil
	}
	if quote.Raw != nil {
		out := make(map[string]any, len(quote.Raw))
		for k, v := range quote.Raw {
			out[k] = v
		}
		return out
	}

	routePlan := make([]map[string]any, 0, len(quote.RoutePlan))
	for _, step := range quote.RoutePlan {
		routePlan = append(routePlan, step.Raw)
	}

	return map[string]any{
		"inputMint":            quote.InputMint.String(),
		"inAmount":             strconv.FormatUint(quote.InAmount, 10),
		"outputMint":           quote.OutputMint.String(),
		"outAmount":            strconv.FormatUint(quote.OutAmount, 10),
		"otherAmountThreshold": strconv.FormatUint(quote.OtherAmountThreshold, 10),
		"priceImpactPct":       quote.PriceImpactPct,
		"contextSlot":          quote.ContextSlot,
		"routePlan":            routePlan,
	}
}

func getPublicKey(raw map[string]any, key string) (solana.PublicKey, error) {
	value, ok := raw[key].(string)
	if !ok || value == "" {
		return solana.PublicKey{}, fmt.Errorf("quote missing %s", key)
	}
	return solana.PublicKeyFromBase58(value)
}

func getUint64(raw map[string]any, key string) (uint64, error) {
	value, ok := raw[key]
	if !ok {
		return 0, fmt.Errorf("quote missing %s", key)
	}

	switch v := value.(type) {
	case string:
		return strconv.ParseUint(v, 10, 64)
	case float64:
		return uint64(v), nil
	case json.Number:
		return strconv.ParseUint(v.String(), 10, 64)
	default:
		return 0, fmt.Errorf("quote field %s has unsupported type %T", key, value)
	}
}

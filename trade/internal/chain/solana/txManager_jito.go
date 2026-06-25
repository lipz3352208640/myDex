package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"myDex/pkg/constant"
	"net/http"
	//"os"
	"strings"
	"time"

	"myDex/pkg/kmsenvelope"

	"github.com/mr-tron/base58"
	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpc"

	solana "github.com/gagliardetto/solana-go"
	alt "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
)

type JitoBundleRequest struct {
	JSONRPC string     `json:"jsonrpc"`
	ID      int        `json:"id"`
	Method  string     `json:"method"`
	Params  [][]string `json:"params"`
}

type JitoBundleResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Result  string        `json:"result"`
	Error   *JitoRPCError `json:"error,omitempty"`
}

type JitoRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type JitoTipFloor struct {
	Time                        time.Time `json:"time"`
	LandedTips25ThPercentile    float64   `json:"landed_tips_25th_percentile"` //25%的成功交易给的jito费用
	LandedTips50ThPercentile    float64   `json:"landed_tips_50th_percentile"` //50%的成功交易给的jito费用
	LandedTips75ThPercentile    float64   `json:"landed_tips_75th_percentile"` //75%的成功交易给的jito费用
	LandedTips95ThPercentile    float64   `json:"landed_tips_95th_percentile"` //95%的成功交易给的jito费用
	LandedTips99ThPercentile    float64   `json:"landed_tips_99th_percentile"` //99%的成功交易给的jito费用
	EmaLandedTips50ThPercentile float64   `json:"ema_landed_tips_50th_percentile"`
}

const JitoRateLimitCode = "32097"

func (tm *TxManager) updateJitoFloorFee() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := tm.queryJitoTipRpc(ctx)
	if err != nil {
		logx.Error(err)
		return
	}
	tm.RWLock.Lock()
	tm.jitoTipFloor = res
	tm.RWLock.Unlock()
}

func (tm *TxManager) CheckJitoFloorFee() {
	tm.updateJitoFloorFee()

	ticker := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-tm.context.Done():
			return
		case <-ticker.C:
			tm.updateJitoFloorFee()
		}
	}
}

func (tm *TxManager) ListJitoFloorFee() float64 {
	tm.RWLock.RLock()
	defer tm.RWLock.RUnlock()
	if tm.jitoTipFloor == nil {
		return 0
	}
	return tm.jitoTipFloor.LandedTips50ThPercentile
}

func (tm *TxManager) queryJitoTipRpc(ctx context.Context) (*JitoTipFloor, error) {
	resp, err := httpc.Do(ctx, http.MethodGet, "https://bundles.jito.wtf/api/v1/bundles/tip_floor", nil)
	if err != nil {
		return nil, err
	}
	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var jitoTipFloor []JitoTipFloor
	err = json.Unmarshal(res, &jitoTipFloor)
	if nil != err {
		return nil, err
	}
	if len(jitoTipFloor) == 0 {
		return nil, nil
	}
	return &jitoTipFloor[0], nil
}

func (tm *TxManager) FetchAddressLookupTables(
	ctx context.Context,
	tableKeys []solana.PublicKey,
) (map[solana.PublicKey]solana.PublicKeySlice, error) {
	tables := make(map[solana.PublicKey]solana.PublicKeySlice, len(tableKeys))
	for _, tableKey := range tableKeys {
		state, err := alt.GetAddressLookupTable(ctx, tm.Client, tableKey)
		if err != nil {
			return nil, fmt.Errorf("fetch address lookup table %s: %w", tableKey.String(), err)
		}
		tables[tableKey] = state.Addresses
	}
	return tables, nil
}

func (tm *TxManager) BuildVersionedTransaction(
	ctx context.Context,
	payer solana.PublicKey,
	instructions []solana.Instruction,
	tableKeys []solana.PublicKey,
) (*solana.Transaction, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	//拿到最新区块
	resp, err := tm.Client.GetLatestBlockhash(timeoutCtx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("get latest blockhash: %w", err)
	}

	opts := []solana.TransactionOption{solana.TransactionPayer(payer)}
	if len(tableKeys) > 0 {
		//抓取alt账户
		tables, err := tm.FetchAddressLookupTables(ctx, tableKeys)
		if err != nil {
			return nil, err
		}
		opts = append(opts, solana.TransactionAddressTables(tables))
	}

	tx, err := solana.NewTransaction(instructions, resp.Value.Blockhash, opts...)
	if err != nil {
		return nil, fmt.Errorf("build versioned transaction: %w", err)
	}
	return tx, nil
}

func (tm *TxManager) SimulateAnyTransaction(ctx context.Context, tx *solana.Transaction) error {
	simOut, err := tm.MainClient.SimulateTransactionWithOpts(ctx, tx, &ag_rpc.SimulateTransactionOpts{
		Commitment: ag_rpc.CommitmentProcessed,
	})
	if err != nil {
		return err
	}
	if simOut != nil && simOut.Value != nil && simOut.Value.Err != nil {
		return fmt.Errorf("simulate transaction failed: %v", simOut.Value.Err)
	}
	return nil
}

// 将bundle交易发送到jito
func (tm *TxManager) SendJitoBundle(
	ctx context.Context,
	jitoEndpoint string,
	txs []*solana.Transaction,
) (string, error) {
	if jitoEndpoint == "" {
		return "", fmt.Errorf("jito endpoint is empty")
	}
	if len(txs) == 0 {
		return "", fmt.Errorf("bundle is empty")
	}

	encoded := make([]string, 0, len(txs))
	for _, tx := range txs {
		if tx == nil {
			return "", fmt.Errorf("bundle contains nil transaction")
		}
		raw, err := tx.MarshalBinary()
		if err != nil {
			return "", fmt.Errorf("serialize transaction: %w", err)
		}
		encoded = append(encoded, base58.Encode(raw))
	}

	reqBody := JitoBundleRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "sendBundle",
		Params:  [][]string{encoded},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal jito bundle request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, jitoEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create jito request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("post jito bundle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("jito bundle request failed with status %s", resp.Status)
	}

	var bundleResp JitoBundleResponse
	if err := json.NewDecoder(resp.Body).Decode(&bundleResp); err != nil {
		return "", fmt.Errorf("decode jito bundle response: %w", err)
	}
	if bundleResp.Error != nil {
		return "", fmt.Errorf("jito bundle rpc error %d: %s", bundleResp.Error.Code, bundleResp.Error.Message)
	}

	return bundleResp.Result, nil
}

func (tm *TxManager) SendViaJitoRetry(ctx context.Context, tx *solana.Transaction) (string, error) {
	if tm.JitoClient == nil {
		return "", fmt.Errorf("jito client is not configured")
	}
	if tx == nil {
		return "", fmt.Errorf("transaction is nil")
	}

	if err := tm.simulate(ctx, tx); err != nil {
		return "", err
	}

	var lastErr error
	for i := 0; i < 5; i++ {
		sig, err := tm.JitoClient.SendTransaction(ctx, tx)
		if err == nil {
			if sig.IsZero() {
				return "", fmt.Errorf("jito send transaction returned empty signature")
			}
			return sig.String(), nil
		}

		lastErr = err
		if !strings.Contains(err.Error(), JitoRateLimitCode) {
			logc.Error(ctx, err)
			return "", err
		}

		logc.Info(ctx, err)
		if err := sleepWithContext(ctx, jitoRetryBackoff(i)); err != nil {
			return "", err
		}
	}

	return "", lastErr
}

func (tm *TxManager) HasJitoTip(tx *solana.Transaction) bool {
	if tx == nil {
		return false
	}

	tipAddress := solana.MustPublicKeyFromBase58(constant.TipAddress)
	for _, instruction := range tx.Message.Instructions {
		programID, err := tx.Message.Program(instruction.ProgramIDIndex)
		if err != nil || !programID.Equals(solana.SystemProgramID) {
			continue
		}
		if len(instruction.Accounts) < 2 {
			continue
		}

		to, err := tx.Message.Account(instruction.Accounts[1])
		if err != nil {
			continue
		}
		if to.Equals(tipAddress) {
			return true
		}
	}

	return false
}

func jitoRetryBackoff(attempt int) time.Duration {
	delay := 200 * time.Millisecond
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	if delay > 3*time.Second {
		return 3 * time.Second
	}
	return delay
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (tm *TxManager) SignTransactionWithServiceKey(tx *solana.Transaction) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}

	privateKey, err := loadServicePrivateKey(context.Background())
	if err != nil {
		return err
	}
	defer kmsenvelope.Zero(privateKey)

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if privateKey.PublicKey().Equals(key) {
			pk := privateKey
			return &pk
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("sign transaction with service key: %w", err)
	}
	return nil
}

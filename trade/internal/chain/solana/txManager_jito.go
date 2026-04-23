package solana

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/mr-tron/base58"

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

	resp, err := tm.Client.GetLatestBlockhash(timeoutCtx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("get latest blockhash: %w", err)
	}

	opts := []solana.TransactionOption{solana.TransactionPayer(payer)}
	if len(tableKeys) > 0 {
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

func (tm *TxManager) SignTransactionWithServiceKey(tx *solana.Transaction) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}

	privateKeyBase58 := os.Getenv("private_key")
	if privateKeyBase58 == "" {
		return fmt.Errorf("private key not set in environment variable")
	}

	privateKeyBytes, err := base58.Decode(privateKeyBase58)
	if err != nil {
		return fmt.Errorf("decode base58 private key: %w", err)
	}
	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privateKeyBytes))
	}

	privateKey := solana.PrivateKey(privateKeyBytes)
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

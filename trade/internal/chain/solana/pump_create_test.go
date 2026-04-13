package solana

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"
)

var (
	pumpProgramID             = solana.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")
	metaplexMetadataProgramID = solana.MustPublicKeyFromBase58("metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s")
	pumpEventAuthority        = mustFindProgramAddress([][]byte{[]byte("__event_authority")}, pumpProgramID)
)

type pumpCreateAccounts struct {
	Payer                  solana.PublicKey
	Mint                   solana.PublicKey
	MintAuthority          solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
	Global                 solana.PublicKey
	Metadata               solana.PublicKey
	EventAuthority         solana.PublicKey
	TokenProgram           solana.PublicKey
	AssociatedTokenProgram solana.PublicKey
	SystemProgram          solana.PublicKey
	Rent                   solana.PublicKey
}

func derivePumpCreateAccounts(payer, mint, tokenProgram solana.PublicKey) (*pumpCreateAccounts, error) {
	mintAuthority, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("mint-authority")},
		pumpProgramID,
	)
	if err != nil {
		return nil, err
	}

	bondingCurve, _, err := solana.FindProgramAddress(
		[][]byte{
			[]byte("bonding-curve"),
			mint.Bytes(),
		},
		pumpProgramID,
	)
	if err != nil {
		return nil, err
	}

	associatedBondingCurve, _, err := solana.FindProgramAddress(
		[][]byte{
			bondingCurve.Bytes(),
			tokenProgram.Bytes(),
			mint.Bytes(),
		},
		solana.SPLAssociatedTokenAccountProgramID,
	)
	if err != nil {
		return nil, err
	}

	global, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("global")},
		pumpProgramID,
	)
	if err != nil {
		return nil, err
	}

	metadata, _, err := solana.FindProgramAddress(
		[][]byte{
			[]byte("metadata"),
			metaplexMetadataProgramID.Bytes(),
			mint.Bytes(),
		},
		metaplexMetadataProgramID,
	)
	if err != nil {
		return nil, err
	}

	eventAuthority, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("__event_authority")},
		pumpProgramID,
	)
	if err != nil {
		return nil, err
	}

	return &pumpCreateAccounts{
		Payer:                  payer,
		Mint:                   mint,
		MintAuthority:          mintAuthority,
		BondingCurve:           bondingCurve,
		AssociatedBondingCurve: associatedBondingCurve,
		Global:                 global,
		Metadata:               metadata,
		EventAuthority:         eventAuthority,
		TokenProgram:           tokenProgram,
		AssociatedTokenProgram: solana.SPLAssociatedTokenAccountProgramID,
		SystemProgram:          solana.SystemProgramID,
		Rent:                   solana.SysVarRentPubkey,
	}, nil
}

func mustFindProgramAddress(seeds [][]byte, programID solana.PublicKey) solana.PublicKey {
	pda, _, err := solana.FindProgramAddress(seeds, programID)
	if err != nil {
		panic(err)
	}
	return pda
}

func mustPrivateKeyFromEnv(t *testing.T, envKey string) solana.PrivateKey {
	t.Helper()

	privateKeyBase58 := os.Getenv(envKey)
	if privateKeyBase58 == "" {
		t.Skipf("%s is not set", envKey)
	}

	privateKeyBytes, err := base58.Decode(privateKeyBase58)
	require.NoError(t, err)
	require.Len(t, privateKeyBytes, 64)

	return solana.PrivateKey(privateKeyBytes)
}

func encodePumpCreateData(name, symbol, uri string, creator solana.PublicKey) ([]byte, error) {
	var buf bytes.Buffer
	encoder := bin.NewBorshEncoder(&buf)

	discriminator := []byte{24, 30, 200, 40, 5, 28, 7, 119}
	if err := encoder.WriteBytes(discriminator, false); err != nil {
		return nil, err
	}
	if err := encoder.Encode(name); err != nil {
		return nil, err
	}
	if err := encoder.Encode(symbol); err != nil {
		return nil, err
	}
	if err := encoder.Encode(uri); err != nil {
		return nil, err
	}
	if err := encoder.Encode(creator); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func buildPumpCreateInstruction(
	name, symbol, uri string,
	creator solana.PublicKey,
	accounts *pumpCreateAccounts,
) (solana.Instruction, error) {
	data, err := encodePumpCreateData(name, symbol, uri, creator)
	if err != nil {
		return nil, err
	}

	accountMetas := solana.AccountMetaSlice{
		solana.Meta(accounts.Mint).WRITE().SIGNER(),
		solana.Meta(accounts.MintAuthority),
		solana.Meta(accounts.BondingCurve).WRITE(),
		solana.Meta(accounts.AssociatedBondingCurve).WRITE(),
		solana.Meta(accounts.Global),
		solana.Meta(metaplexMetadataProgramID),
		solana.Meta(accounts.Metadata).WRITE(),
		solana.Meta(accounts.Payer).WRITE().SIGNER(),
		solana.Meta(accounts.SystemProgram),
		solana.Meta(accounts.TokenProgram),
		solana.Meta(accounts.AssociatedTokenProgram),
		solana.Meta(accounts.Rent),
		solana.Meta(accounts.EventAuthority),
		solana.Meta(pumpProgramID),
	}

	return solana.NewInstruction(
		pumpProgramID,
		accountMetas,
		data,
	), nil
}

func TestDerivePumpCreateAccountsForToken(t *testing.T) {
	payer := solana.NewWallet().PublicKey()
	mint := solana.NewWallet().PublicKey()

	accounts, err := derivePumpCreateAccounts(payer, mint, solana.TokenProgramID)
	require.NoError(t, err)
	require.Equal(t, payer, accounts.Payer)
	require.Equal(t, mint, accounts.Mint)
	require.False(t, accounts.MintAuthority.IsZero())
	require.False(t, accounts.BondingCurve.IsZero())
	require.False(t, accounts.AssociatedBondingCurve.IsZero())
	require.False(t, accounts.Global.IsZero())
	require.False(t, accounts.Metadata.IsZero())
	require.False(t, accounts.EventAuthority.IsZero())

	t.Logf("payer=%s", accounts.Payer)
	t.Logf("mint=%s", accounts.Mint)
	t.Logf("mintAuthority=%s", accounts.MintAuthority)
	t.Logf("bondingCurve=%s", accounts.BondingCurve)
	t.Logf("associatedBondingCurve=%s", accounts.AssociatedBondingCurve)
	t.Logf("global=%s", accounts.Global)
	t.Logf("metadata=%s", accounts.Metadata)
	t.Logf("eventAuthority=%s", accounts.EventAuthority)
}

func TestDerivePumpCreateAccountsForToken2022(t *testing.T) {
	payer := solana.NewWallet().PublicKey()
	mint := solana.NewWallet().PublicKey()

	accounts, err := derivePumpCreateAccounts(payer, mint, solana.Token2022ProgramID)
	require.NoError(t, err)
	require.Equal(t, solana.Token2022ProgramID, accounts.TokenProgram)
	require.False(t, accounts.AssociatedBondingCurve.IsZero())

	t.Logf("payer=%s", accounts.Payer)
	t.Logf("mint=%s", accounts.Mint)
	t.Logf("mintAuthority=%s", accounts.MintAuthority)
	t.Logf("bondingCurve=%s", accounts.BondingCurve)
	t.Logf("associatedBondingCurve=%s", accounts.AssociatedBondingCurve)
	t.Logf("global=%s", accounts.Global)
	t.Logf("metadata=%s", accounts.Metadata)
	t.Logf("eventAuthority=%s", accounts.EventAuthority)
}

func TestPumpCreateOnDevnet(t *testing.T) {
	privateKey := mustPrivateKeyFromEnv(t, "private_key")
	mint := solana.NewWallet()

	payerPublicKey := privateKey.PublicKey()
	accounts, err := derivePumpCreateAccounts(payerPublicKey, mint.PublicKey(), solana.TokenProgramID)
	require.NoError(t, err)
	require.Equal(t, pumpEventAuthority, accounts.EventAuthority)

	instruction, err := buildPumpCreateInstruction(
		"Test Pump Coin",
		"TPC",
		"https://example.com/devnet-test.json",
		payerPublicKey,
		accounts,
	)
	require.NoError(t, err)

	client := rpc.New(rpc.DevNet_RPC)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// airdropSig, err := client.RequestAirdrop(ctx, payerPublicKey, 2*solana.LAMPORTS_PER_SOL, rpc.CommitmentFinalized)
	// if err != nil {
	// 	t.Logf("airdrop skipped or failed: %v", err)
	// } else {
	// 	t.Logf("airdrop signature=%s", airdropSig.String())
	// }

	latestBlockhash, err := client.GetLatestBlockhash(ctx, rpc.CommitmentConfirmed)
	require.NoError(t, err)

	tx, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		latestBlockhash.Value.Blockhash,
		solana.TransactionPayer(payerPublicKey),
	)
	require.NoError(t, err)

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		switch key {
		case payerPublicKey:
			pk := privateKey
			return &pk
		case mint.PublicKey():
			pk := mint.PrivateKey
			return &pk
		default:
			return nil
		}
	})
	require.NoError(t, err)

	simOut, err := client.SimulateTransactionWithOpts(ctx, tx, &rpc.SimulateTransactionOpts{
		Commitment: rpc.CommitmentProcessed,
	})
	require.NoError(t, err)
	if simOut != nil && simOut.Value != nil && simOut.Value.Err != nil {
		t.Fatalf("simulate failed: err=%v logs=%v", simOut.Value.Err, simOut.Value.Logs)
	}

	sig, err := client.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{
		SkipPreflight:       false,
		PreflightCommitment: rpc.CommitmentProcessed,
	})
	require.NoError(t, err)

	t.Logf("payer=%s", payerPublicKey)
	t.Logf("mint=%s", mint.PublicKey())
	t.Logf("bondingCurve=%s", accounts.BondingCurve)
	t.Logf("associatedBondingCurve=%s", accounts.AssociatedBondingCurve)
	t.Logf("metadata=%s", accounts.Metadata)
	t.Logf("signature=%s", sig.String())
	t.Logf("next step: wait for confirmation, then use this mint in CreateMarketOrder")
}

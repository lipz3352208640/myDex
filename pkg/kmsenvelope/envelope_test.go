package kmsenvelope

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"testing"

	"github.com/mr-tron/base58"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	ctx := context.Background()
	kms := newTestKMS(t)
	aad := []byte("uid=1:wallet=abc")
	plaintext := []byte("secret private key bytes")

	envelopeJSON, err := EncryptJSON(ctx, kms, plaintext, aad)
	if err != nil {
		t.Fatalf("EncryptJSON() error = %v", err)
	}
	if bytes.Contains([]byte(envelopeJSON), plaintext) {
		t.Fatalf("envelope leaked plaintext")
	}

	got, err := DecryptJSON(ctx, kms, envelopeJSON, aad)
	if err != nil {
		t.Fatalf("DecryptJSON() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("DecryptJSON() = %q, want %q", got, plaintext)
	}
}

func TestEnvelopeRejectsWrongAAD(t *testing.T) {
	ctx := context.Background()
	kms := newTestKMS(t)

	envelopeJSON, err := EncryptJSON(ctx, kms, []byte("secret"), []byte("wallet-a"))
	if err != nil {
		t.Fatalf("EncryptJSON() error = %v", err)
	}

	if _, err := DecryptJSON(ctx, kms, envelopeJSON, []byte("wallet-b")); err == nil {
		t.Fatalf("DecryptJSON() with wrong aad succeeded")
	}
}

func TestSolanaPrivateKeyRoundTrip(t *testing.T) {
	ctx := context.Background()
	kms := newTestKMS(t)
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	privateKeyBase58 := base58.Encode(privateKey)
	aad := []byte("uid=7:wallet=sol")

	envelopeJSON, err := EncryptSolanaPrivateKeyBase58(ctx, kms, privateKeyBase58, aad)
	if err != nil {
		t.Fatalf("EncryptSolanaPrivateKeyBase58() error = %v", err)
	}

	got, err := DecryptSolanaPrivateKeyBase58(ctx, kms, envelopeJSON, aad)
	if err != nil {
		t.Fatalf("DecryptSolanaPrivateKeyBase58() error = %v", err)
	}
	if got != privateKeyBase58 {
		t.Fatalf("private key round trip mismatch")
	}
}

func TestNewLocalKMSFromEnv(t *testing.T) {
	masterKey := bytes.Repeat([]byte{3}, dataKeySize)
	t.Setenv(EnvLocalMasterKeyB64, base64.StdEncoding.EncodeToString(masterKey))

	if _, err := NewLocalKMSFromEnv("local-test"); err != nil {
		t.Fatalf("NewLocalKMSFromEnv() error = %v", err)
	}
}

func newTestKMS(t *testing.T) *LocalKMS {
	t.Helper()

	masterKey := bytes.Repeat([]byte{1}, dataKeySize)
	kms, err := NewLocalKMS("local-test", masterKey)
	if err != nil {
		t.Fatalf("NewLocalKMS() error = %v", err)
	}
	return kms
}

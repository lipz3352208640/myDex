package kmsenvelope

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

func EncryptSolanaPrivateKeyBase58(ctx context.Context, kms KMS, privateKeyBase58 string, aad []byte) (string, error) {
	if privateKeyBase58 == "" {
		return "", fmt.Errorf("private key is empty")
	}

	//base58加密
	privateKey, err := base58.Decode(privateKeyBase58)
	if err != nil {
		return "", fmt.Errorf("decode solana private key: %w", err)
	}
	defer Zero(privateKey)

	if len(privateKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("invalid solana private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privateKey))
	}

	return EncryptJSON(ctx, kms, privateKey, aad)
}

func DecryptSolanaPrivateKey(ctx context.Context, kms KMS, envelopeJSON string, aad []byte) (ed25519.PrivateKey, error) {
	privateKey, err := DecryptJSON(ctx, kms, envelopeJSON, aad)
	if err != nil {
		return nil, err
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		Zero(privateKey)
		return nil, fmt.Errorf("invalid solana private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privateKey))
	}

	return ed25519.PrivateKey(privateKey), nil
}

func DecryptSolanaPrivateKeyBase58(ctx context.Context, kms KMS, envelopeJSON string, aad []byte) (string, error) {
	privateKey, err := DecryptSolanaPrivateKey(ctx, kms, envelopeJSON, aad)
	if err != nil {
		return "", err
	}
	defer Zero(privateKey)

	return base58.Encode(privateKey), nil
}

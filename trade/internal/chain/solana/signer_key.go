package solana

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"

	"myDex/pkg/kmsenvelope"

	solana "github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
)

const (
	EnvPlainPrivateKey           = "private_key"
	EnvEncryptedPrivateKeyBundle = "ENCRYPTED_PRIVATE_KEY_BUNDLE"
	EnvKMSLocalKeyID             = "KMS_LOCAL_KEY_ID"
	EnvPrivateKeyAAD             = "PRIVATE_KEY_AAD"

	defaultKMSLocalKeyID = "local-mvp"
	defaultPrivateKeyAAD = "myDex:trade:service-private-key:v1"
)

func loadServicePrivateKey(ctx context.Context) (solana.PrivateKey, error) {
	if encryptedBundle := os.Getenv(EnvEncryptedPrivateKeyBundle); encryptedBundle != "" {
		keyID := os.Getenv(EnvKMSLocalKeyID)
		if keyID == "" {
			keyID = defaultKMSLocalKeyID
		}

		localKMS, err := kmsenvelope.NewLocalKMSFromEnv(keyID)
		if err != nil {
			return nil, err
		}

		privateKey, err := kmsenvelope.DecryptSolanaPrivateKey(ctx, localKMS, encryptedBundle, privateKeyAAD())
		if err != nil {
			return nil, err
		}
		return solana.PrivateKey(privateKey), nil
	}

	privateKeyBase58 := os.Getenv(EnvPlainPrivateKey)
	if privateKeyBase58 == "" {
		return nil, fmt.Errorf("%s or %s must be set", EnvEncryptedPrivateKeyBundle, EnvPlainPrivateKey)
	}

	privateKeyBytes, err := base58.Decode(privateKeyBase58)
	if err != nil {
		return nil, fmt.Errorf("decode base58 private key: %w", err)
	}
	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		kmsenvelope.Zero(privateKeyBytes)
		return nil, fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privateKeyBytes))
	}

	return solana.PrivateKey(privateKeyBytes), nil
}

func privateKeyAAD() []byte {
	aad := os.Getenv(EnvPrivateKeyAAD)
	if aad == "" {
		aad = defaultPrivateKeyAAD
	}
	return []byte(aad)
}

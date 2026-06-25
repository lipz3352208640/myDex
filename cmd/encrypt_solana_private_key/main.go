package main

import (
	"context"
	"fmt"
	"os"

	"myDex/pkg/kmsenvelope"
)

func main() {
	privateKey := os.Getenv("private_key")
	if privateKey == "" {
		exitf("private_key is empty")
	}

	keyID := os.Getenv("KMS_LOCAL_KEY_ID")
	if keyID == "" {
		keyID = "local-mvp"
	}

	aad := os.Getenv("PRIVATE_KEY_AAD")
	if aad == "" {
		aad = "myDex:trade:service-private-key:v1"
	}

	kms, err := kmsenvelope.NewLocalKMSFromEnv(keyID)
	if err != nil {
		exitf("init local kms: %v", err)
	}

	envelopeJSON, err := kmsenvelope.EncryptSolanaPrivateKeyBase58(context.Background(), kms, privateKey, []byte(aad))
	if err != nil {
		exitf("encrypt private key: %v", err)
	}

	fmt.Println(envelopeJSON)
}

func exitf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

package kmsenvelope

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

const (
	currentVersion = 1
	algorithm      = "AES-256-GCM+KMS-ENVELOPE"
	dataKeySize    = 32
	nonceSize      = 12
)

type KMS interface {
	EncryptDataKey(ctx context.Context, plaintextDataKey []byte, aad []byte) (EncryptedDataKey, error)
	DecryptDataKey(ctx context.Context, encryptedDataKey EncryptedDataKey, aad []byte) ([]byte, error)
}

type EncryptedDataKey struct {
	KeyID      string `json:"key_id"`
	Ciphertext string `json:"ciphertext"`
}

type Envelope struct {
	Version          int              `json:"version"`
	Algorithm        string           `json:"algorithm"`
	EncryptedDataKey EncryptedDataKey `json:"encrypted_data_key"`
	Nonce            string           `json:"nonce"`
	Ciphertext       string           `json:"ciphertext"`
	CreatedAt        int64            `json:"created_at"`
}

func Encrypt(ctx context.Context, kms KMS, plaintext []byte, aad []byte) (*Envelope, error) {
	if kms == nil {
		return nil, fmt.Errorf("kms is nil")
	}
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("plaintext is empty")
	}

	dataKey, err := randomBytes(dataKeySize)
	if err != nil {
		return nil, fmt.Errorf("generate data key: %w", err)
	}
	defer Zero(dataKey)

	encryptedDataKey, err := kms.EncryptDataKey(ctx, dataKey, aad)
	if err != nil {
		return nil, fmt.Errorf("encrypt data key: %w", err)
	}

	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce, err := randomBytes(nonceSize)
	if err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)
	return &Envelope{
		Version:          currentVersion,
		Algorithm:        algorithm,
		EncryptedDataKey: encryptedDataKey,
		Nonce:            base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:       base64.StdEncoding.EncodeToString(ciphertext),
		CreatedAt:        time.Now().Unix(),
	}, nil
}

func Decrypt(ctx context.Context, kms KMS, envelope *Envelope, aad []byte) ([]byte, error) {
	if kms == nil {
		return nil, fmt.Errorf("kms is nil")
	}
	if envelope == nil {
		return nil, fmt.Errorf("envelope is nil")
	}
	if envelope.Version != currentVersion {
		return nil, fmt.Errorf("unsupported envelope version: %d", envelope.Version)
	}
	if envelope.Algorithm != algorithm {
		return nil, fmt.Errorf("unsupported envelope algorithm: %s", envelope.Algorithm)
	}

	dataKey, err := kms.DecryptDataKey(ctx, envelope.EncryptedDataKey, aad)
	if err != nil {
		return nil, fmt.Errorf("decrypt data key: %w", err)
	}
	defer Zero(dataKey)
	if len(dataKey) != dataKeySize {
		return nil, fmt.Errorf("invalid data key length: %d", len(dataKey))
	}

	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce length: %d", len(nonce))
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("open ciphertext: %w", err)
	}
	return plaintext, nil
}

func EncryptJSON(ctx context.Context, kms KMS, plaintext []byte, aad []byte) (string, error) {
	envelope, err := Encrypt(ctx, kms, plaintext, aad)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("marshal envelope: %w", err)
	}
	return string(data), nil
}

func DecryptJSON(ctx context.Context, kms KMS, envelopeJSON string, aad []byte) ([]byte, error) {
	var envelope Envelope
	if err := json.Unmarshal([]byte(envelopeJSON), &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	return Decrypt(ctx, kms, &envelope, aad)
}

func Zero(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

func randomBytes(size int) ([]byte, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return nil, err
	}
	return data, nil
}

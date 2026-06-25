package kmsenvelope

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"os"
)

const EnvLocalMasterKeyB64 = "KMS_LOCAL_MASTER_KEY_B64"

type LocalKMS struct {
	keyID     string
	masterKey []byte
}

func NewLocalKMS(keyID string, masterKey []byte) (*LocalKMS, error) {
	if keyID == "" {
		return nil, fmt.Errorf("key id is empty")
	}
	if len(masterKey) != dataKeySize {
		return nil, fmt.Errorf("invalid master key length: expected %d, got %d", dataKeySize, len(masterKey))
	}

	keyCopy := append([]byte(nil), masterKey...)
	return &LocalKMS{
		keyID:     keyID,
		masterKey: keyCopy,
	}, nil
}

// kms构造函数
func NewLocalKMSFromEnv(keyID string) (*LocalKMS, error) {
	masterKeyB64 := os.Getenv(EnvLocalMasterKeyB64)
	if masterKeyB64 == "" {
		return nil, fmt.Errorf("%s is empty", EnvLocalMasterKeyB64)
	}

	masterKey, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", EnvLocalMasterKeyB64, err)
	}
	defer Zero(masterKey)

	return NewLocalKMS(keyID, masterKey)
}

func (k *LocalKMS) EncryptDataKey(_ context.Context, plaintextDataKey []byte, aad []byte) (EncryptedDataKey, error) {
	if k == nil {
		return EncryptedDataKey{}, fmt.Errorf("local kms is nil")
	}
	if len(plaintextDataKey) != dataKeySize {
		return EncryptedDataKey{}, fmt.Errorf("invalid data key length: %d", len(plaintextDataKey))
	}

	gcm, err := k.gcm()
	if err != nil {
		return EncryptedDataKey{}, err
	}
	nonce, err := randomBytes(gcm.NonceSize())
	if err != nil {
		return EncryptedDataKey{}, fmt.Errorf("generate kms nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintextDataKey, aad)
	return EncryptedDataKey{
		KeyID:      k.keyID,
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func (k *LocalKMS) DecryptDataKey(_ context.Context, encryptedDataKey EncryptedDataKey, aad []byte) ([]byte, error) {
	if k == nil {
		return nil, fmt.Errorf("local kms is nil")
	}
	if encryptedDataKey.KeyID != k.keyID {
		return nil, fmt.Errorf("encrypted data key belongs to %s, local kms key is %s", encryptedDataKey.KeyID, k.keyID)
	}

	blob, err := base64.StdEncoding.DecodeString(encryptedDataKey.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted data key: %w", err)
	}

	gcm, err := k.gcm()
	if err != nil {
		return nil, err
	}
	if len(blob) <= gcm.NonceSize() {
		return nil, fmt.Errorf("encrypted data key is too short")
	}

	nonce := blob[:gcm.NonceSize()]
	ciphertext := blob[gcm.NonceSize():]
	plaintextDataKey, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("open encrypted data key: %w", err)
	}
	return plaintextDataKey, nil
}

func (k *LocalKMS) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(k.masterKey)
	if err != nil {
		return nil, fmt.Errorf("create local kms cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create local kms gcm: %w", err)
	}
	return gcm, nil
}

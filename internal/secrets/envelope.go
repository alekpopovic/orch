package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

type Ciphertext struct {
	KeyID string
	Data  []byte
}

type Envelope interface {
	Encrypt(ctx context.Context, plaintext []byte) (Ciphertext, error)
	Decrypt(ctx context.Context, ciphertext Ciphertext) ([]byte, error)
}

type LocalEnvelope struct {
	key   []byte
	keyID string
}

func NewLocalEnvelope(keyMaterial string) (*LocalEnvelope, error) {
	keyMaterial = strings.TrimSpace(keyMaterial)
	if keyMaterial == "" {
		return nil, fmt.Errorf("secret encryption key is required")
	}
	key, err := decodeKeyMaterial(keyMaterial)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(key)
	return &LocalEnvelope{
		key:   key,
		keyID: "local:" + hex.EncodeToString(digest[:8]),
	}, nil
}

func (e *LocalEnvelope) Encrypt(ctx context.Context, plaintext []byte) (Ciphertext, error) {
	if err := ctx.Err(); err != nil {
		return Ciphertext{}, err
	}
	gcm, err := e.gcm()
	if err != nil {
		return Ciphertext{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Ciphertext{}, fmt.Errorf("generate secret nonce: %w", err)
	}
	data := append([]byte(nil), nonce...)
	data = gcm.Seal(data, nonce, plaintext, []byte(e.keyID))
	return Ciphertext{KeyID: e.keyID, Data: data}, nil
}

func (e *LocalEnvelope) Decrypt(ctx context.Context, ciphertext Ciphertext) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ciphertext.KeyID != "" && ciphertext.KeyID != e.keyID {
		return nil, fmt.Errorf("secret key id mismatch")
	}
	gcm, err := e.gcm()
	if err != nil {
		return nil, err
	}
	if len(ciphertext.Data) < gcm.NonceSize() {
		return nil, fmt.Errorf("secret ciphertext is invalid")
	}
	nonce := ciphertext.Data[:gcm.NonceSize()]
	data := ciphertext.Data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, data, []byte(e.keyID))
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}

func (e *LocalEnvelope) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("create secret cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create secret gcm: %w", err)
	}
	return gcm, nil
}

func decodeKeyMaterial(value string) ([]byte, error) {
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		switch len(decoded) {
		case 16, 24, 32:
			return append([]byte(nil), decoded...), nil
		}
	}
	digest := sha256.Sum256([]byte(value))
	return digest[:], nil
}

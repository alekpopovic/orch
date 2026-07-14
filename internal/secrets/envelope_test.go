package secrets

import (
	"context"
	"testing"
)

func TestLocalEnvelopeEncryptDecrypt(t *testing.T) {
	envelope, err := NewLocalEnvelope("test-secret-key")
	if err != nil {
		t.Fatalf("create envelope: %v", err)
	}
	ciphertext, err := envelope.Encrypt(context.Background(), []byte("super-secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if string(ciphertext.Data) == "super-secret" {
		t.Fatalf("expected ciphertext not to store plaintext")
	}
	plaintext, err := envelope.Decrypt(context.Background(), ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(plaintext) != "super-secret" {
		t.Fatalf("unexpected plaintext %q", plaintext)
	}
}

func TestLocalEnvelopeWrongKeyFails(t *testing.T) {
	first, err := NewLocalEnvelope("first-key")
	if err != nil {
		t.Fatalf("create first envelope: %v", err)
	}
	second, err := NewLocalEnvelope("second-key")
	if err != nil {
		t.Fatalf("create second envelope: %v", err)
	}
	ciphertext, err := first.Encrypt(context.Background(), []byte("super-secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := second.Decrypt(context.Background(), ciphertext); err == nil {
		t.Fatalf("expected wrong key to fail")
	}
}

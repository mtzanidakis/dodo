package crypto_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/mtzanidakis/dodo/internal/crypto"
)

func newT(t *testing.T) *crypto.Crypto {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	c, err := crypto.New(key)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return c
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()
	c := newT(t)
	plain := "super-secret-bot-token:123456:ABC"
	ct, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("got %q want %q", got, plain)
	}
}

func TestNonceRandomness(t *testing.T) {
	t.Parallel()
	c := newT(t)
	a, _ := c.Encrypt("x")
	b, _ := c.Encrypt("x")
	if a == b {
		t.Fatalf("two encryptions produced identical ciphertext")
	}
}

func TestTamperDetection(t *testing.T) {
	t.Parallel()
	c := newT(t)
	ct, _ := c.Encrypt("hello")
	raw, _ := base64.StdEncoding.DecodeString(ct)
	raw[len(raw)-1] ^= 0xff
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatalf("expected tamper error, got nil")
	}
}

func TestStoredValueIsNotPlaintext(t *testing.T) {
	t.Parallel()
	c := newT(t)
	plain := "0123456789:botsecret"
	ct, _ := c.Encrypt(plain)
	if strings.Contains(ct, plain) {
		t.Fatalf("ciphertext contains plaintext")
	}
}

func TestInvalidKey(t *testing.T) {
	t.Parallel()
	if _, err := crypto.New(make([]byte, 16)); err == nil {
		t.Fatalf("expected error for short key")
	}
}

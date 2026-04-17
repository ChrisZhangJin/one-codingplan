package models

import (
	"testing"

	"one-codingplan/internal/crypto"
)

func TestUpstream_DecryptAPIKey_Empty(t *testing.T) {
	u := &Upstream{APIKeyEnc: nil}
	key := []byte("0123456789abcdef")
	got, err := u.DecryptAPIKey(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("want empty string, got %q", got)
	}
}

func TestUpstream_DecryptAPIKey_EmptySlice(t *testing.T) {
	u := &Upstream{APIKeyEnc: []byte{}}
	key := []byte("0123456789abcdef")
	got, err := u.DecryptAPIKey(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("want empty string, got %q", got)
	}
}

func TestUpstream_DecryptAPIKey_Valid(t *testing.T) {
	encKey := []byte("0123456789abcdef")
	apiKey := "sk-test-1234567890"

	enc, err := crypto.Encrypt(encKey, apiKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	u := &Upstream{APIKeyEnc: enc}
	got, err := u.DecryptAPIKey(encKey)
	if err != nil {
		t.Fatalf("DecryptAPIKey: %v", err)
	}
	if got != apiKey {
		t.Errorf("want %q, got %q", apiKey, got)
	}
}

func TestUpstream_DecryptAPIKey_WrongKey(t *testing.T) {
	encKey := []byte("0123456789abcdef")
	wrongKey := []byte("fedcba9876543210")

	enc, err := crypto.Encrypt(encKey, "sk-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	u := &Upstream{APIKeyEnc: enc}
	_, err = u.DecryptAPIKey(wrongKey)
	if err == nil {
		t.Error("expected error when decrypting with wrong key, got nil")
	}
}

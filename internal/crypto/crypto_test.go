package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	key := []byte("0123456789abcdef") // 16 bytes (AES-128)
	plaintext := "hello, world"

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plaintext {
		t.Errorf("want %q, got %q", plaintext, got)
	}
}

func TestEncryptDecrypt_AllKeySizes(t *testing.T) {
	sizes := []int{16, 24, 32}
	for _, sz := range sizes {
		key := bytes.Repeat([]byte("k"), sz)
		ct, err := Encrypt(key, "test")
		if err != nil {
			t.Errorf("Encrypt key=%d: %v", sz, err)
			continue
		}
		pt, err := Decrypt(key, ct)
		if err != nil {
			t.Errorf("Decrypt key=%d: %v", sz, err)
			continue
		}
		if pt != "test" {
			t.Errorf("key=%d: want %q, got %q", sz, "test", pt)
		}
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	key := []byte("0123456789abcdef")
	ct, err := Encrypt(key, "")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	got, err := Decrypt(key, ct)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if got != "" {
		t.Errorf("want empty string, got %q", got)
	}
}

func TestEncrypt_InvalidKeySize(t *testing.T) {
	key := []byte("tooshort") // 8 bytes — invalid for AES
	_, err := Encrypt(key, "test")
	if err == nil {
		t.Error("expected error for invalid key size, got nil")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1 := []byte("0123456789abcdef")
	key2 := []byte("fedcba9876543210")

	ct, err := Encrypt(key1, "secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = Decrypt(key2, ct)
	if err == nil {
		t.Error("expected error when decrypting with wrong key, got nil")
	}
}

func TestDecrypt_CiphertextTooShort(t *testing.T) {
	key := []byte("0123456789abcdef")
	_, err := Decrypt(key, []byte{1, 2, 3}) // shorter than nonce size (12 bytes)
	if err == nil {
		t.Error("expected error for short ciphertext, got nil")
	}
}

func TestDecrypt_EmptyCiphertext(t *testing.T) {
	key := []byte("0123456789abcdef")
	_, err := Decrypt(key, []byte{})
	if err == nil {
		t.Error("expected error for empty ciphertext, got nil")
	}
}

func TestEncrypt_NondeterministicOutput(t *testing.T) {
	// Each Encrypt call should produce a different ciphertext (random nonce).
	key := []byte("0123456789abcdef")
	ct1, _ := Encrypt(key, "same plaintext")
	ct2, _ := Encrypt(key, "same plaintext")
	if bytes.Equal(ct1, ct2) {
		t.Error("expected different ciphertexts from repeated Encrypt (nonce must be random)")
	}
}

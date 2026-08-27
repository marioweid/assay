package crypto

import (
	"bytes"
	"testing"
)

func TestNewRejectsNonAES256Key(t *testing.T) {
	t.Parallel()

	if _, err := New(make([]byte, 31)); err == nil {
		t.Fatal("New() error = nil, want invalid key length error")
	}
}

func TestCipherRoundTrip(t *testing.T) {
	t.Parallel()

	cipher := newTestCipher(t)
	want := []byte{0, 1, 2, 127, 128, 255}
	envelope, err := cipher.Encrypt(want)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := cipher.Decrypt(envelope)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("plaintext = %v, want %v", got, want)
	}
}

func TestCipherEncryptsEmptyPlaintext(t *testing.T) {
	t.Parallel()

	cipher := newTestCipher(t)
	envelope, err := cipher.Encrypt(nil)
	if err != nil {
		t.Fatalf("encrypt empty plaintext: %v", err)
	}
	plaintext, err := cipher.Decrypt(envelope)
	if err != nil {
		t.Fatalf("decrypt empty plaintext: %v", err)
	}
	if len(plaintext) != 0 {
		t.Fatalf("plaintext length = %d, want 0", len(plaintext))
	}
}

func TestCipherUsesFreshNonce(t *testing.T) {
	t.Parallel()

	cipher := newTestCipher(t)
	first, err := cipher.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("first encrypt: %v", err)
	}
	second, err := cipher.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("second encrypt: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("two envelopes are equal, want fresh nonce")
	}
}

func TestCipherRejectsInvalidEnvelope(t *testing.T) {
	t.Parallel()

	cipher := newTestCipher(t)
	valid, err := cipher.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	tampered := bytes.Clone(valid)
	tampered[len(tampered)-1] ^= 0xff

	tests := []struct {
		name     string
		envelope []byte
	}{
		{name: "empty", envelope: nil},
		{name: "truncated", envelope: []byte{1}},
		{name: "unsupported version", envelope: append([]byte{2}, valid[1:]...)},
		{name: "tampered", envelope: tampered},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := cipher.Decrypt(test.envelope); err == nil {
				t.Fatal("Decrypt() error = nil, want invalid envelope error")
			}
		})
	}
}

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	cipher, err := New(make([]byte, 32))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	return cipher
}

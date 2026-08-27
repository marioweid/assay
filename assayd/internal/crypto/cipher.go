// Package crypto encrypts Assay's stored secrets.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

const (
	aes256KeyBytes  = 32
	envelopeVersion = byte(1)
)

// Cipher encrypts and decrypts versioned AES-256-GCM envelopes.
type Cipher struct {
	aead cipher.AEAD
}

// New constructs an AES-256-GCM cipher from a 32-byte key.
func New(key []byte) (*Cipher, error) {
	if len(key) != aes256KeyBytes {
		return nil, fmt.Errorf("create secret cipher: key must be exactly %d bytes", aes256KeyBytes)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM cipher: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt seals plaintext with a fresh nonce and returns a versioned envelope.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate secret nonce: %w", err)
	}

	envelope := make([]byte, 1, 1+len(nonce)+len(plaintext)+c.aead.Overhead())
	envelope[0] = envelopeVersion
	envelope = append(envelope, nonce...)
	envelope = c.aead.Seal(envelope, nonce, plaintext, nil)
	return envelope, nil
}

// Decrypt authenticates and opens a versioned envelope.
func (c *Cipher) Decrypt(envelope []byte) ([]byte, error) {
	minimumLength := 1 + c.aead.NonceSize() + c.aead.Overhead()
	if len(envelope) < minimumLength {
		return nil, fmt.Errorf("decrypt secret: envelope is truncated")
	}
	if envelope[0] != envelopeVersion {
		return nil, fmt.Errorf("decrypt secret: unsupported envelope version")
	}

	nonceEnd := 1 + c.aead.NonceSize()
	nonce := envelope[1:nonceEnd]
	plaintext, err := c.aead.Open(nil, nonce, envelope[nonceEnd:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: authenticate envelope: %w", err)
	}
	return plaintext, nil
}

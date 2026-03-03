// This file implements authenticated encryption for app environment-variable
// values stored in the database. It defines one concrete AES-256-GCM codec used
// by app create workflows and startup wiring.
//
// Security intent:
// - Encrypt all env var values before persistence (not only secret-marked keys).
// - Use AEAD (GCM) so tampering is detected during decrypt.
// - Embed a versioned header in ciphertext so future key rotation and format
//   upgrades can be introduced without rewriting service call sites.

package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	envCipherVersionV1  = "v1"     // current storage payload format version.
	envCipherAlgoAESGCM = "aesgcm" // authenticated cipher identifier in payload header.
)

var (
	ErrInvalidEnvValueCiphertext = errors.New("invalid env value ciphertext") // ciphertext failed structural or cryptographic validation.
)

// EnvValueCipher encrypts/decrypts env values for DB persistence.
//
// Storage format:
//
//	v1:aesgcm:<key-id>:<nonce-base64url>:<ciphertext-base64url>
//
// The version/algo/key-id header is authenticated as AAD, so metadata tampering
// fails decryption.
type EnvValueCipher struct {
	keyID      string      // logical key identifier embedded in ciphertext header.
	aead       cipher.AEAD // AES-GCM primitive used for encrypt/decrypt.
	randReader io.Reader   // entropy source for nonce generation.
}

// NewEnvValueCipher constructs one AES-256-GCM env value cipher.
// key must contain exactly 32 raw bytes.
func NewEnvValueCipher(keyID string, key []byte) (*EnvValueCipher, error) {
	trimmedKeyID := strings.TrimSpace(keyID)
	if trimmedKeyID == "" {
		return nil, errors.New("env value cipher requires non-empty key id")
	}
	if len(key) != 32 {
		return nil, errors.New("env value cipher requires 32-byte key")
	}

	keyCopy := append([]byte(nil), key...)
	block, err := aes.NewCipher(keyCopy)
	if err != nil {
		return nil, fmt.Errorf("build aes block: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("build aes-gcm: %w", err)
	}

	return &EnvValueCipher{
		keyID:      trimmedKeyID,
		aead:       aead,
		randReader: rand.Reader,
	}, nil
}

// EncryptForStorage encrypts plaintext and returns a versioned encoded payload.
func (c *EnvValueCipher) EncryptForStorage(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(c.randReader, nonce); err != nil {
		return "", fmt.Errorf("generate env nonce: %w", err)
	}

	header := c.header()
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), []byte(header))

	return header +
		":" + base64.RawURLEncoding.EncodeToString(nonce) +
		":" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// DecryptFromStorage decrypts one encoded ciphertext payload.
func (c *EnvValueCipher) DecryptFromStorage(encoded string) (string, error) {
	parts := strings.Split(encoded, ":")
	if len(parts) != 5 {
		return "", ErrInvalidEnvValueCiphertext
	}

	version, algo, keyID := parts[0], parts[1], parts[2]
	if version != envCipherVersionV1 || algo != envCipherAlgoAESGCM || keyID != c.keyID {
		return "", ErrInvalidEnvValueCiphertext
	}

	nonce, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(nonce) != c.aead.NonceSize() {
		return "", ErrInvalidEnvValueCiphertext
	}

	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil {
		return "", ErrInvalidEnvValueCiphertext
	}

	header := strings.Join(parts[:3], ":")
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(header))
	if err != nil {
		return "", ErrInvalidEnvValueCiphertext
	}

	return string(plaintext), nil
}

func (c *EnvValueCipher) header() string {
	return envCipherVersionV1 + ":" + envCipherAlgoAESGCM + ":" + c.keyID
}

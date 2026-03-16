// This file implements env-value encryption.

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

	"github.com/google/uuid"
)

const (
	envCipherVersionV1  = "v1"     // ciphertext payload format version.
	envCipherAlgoAESGCM = "aesgcm" // authenticated encryption algorithm identifier.
)

var (
	// ErrInvalidEnvValueCiphertext indicates malformed payload, unknown key-id,
	// invalid nonce/ciphertext encoding, or failed AEAD authentication.
	ErrInvalidEnvValueCiphertext = errors.New("invalid env value ciphertext")
)

// EnvValueCipher encrypts/decrypts env var values for one key id.
//
// Stored payload format:
//
//	v1:aesgcm:<key-id>:<nonce-base64url>:<ciphertext-base64url>
type EnvValueCipher struct {
	keyID      string      // logical key id embedded in payload header.
	aead       cipher.AEAD // AES-256-GCM AEAD primitive.
	randReader io.Reader   // nonce entropy source.
}

// EnvValueRowContext identifies the row-bound context used in AAD for new
// encryption's. Values must be stable across reads/writes for the same row.
type EnvValueRowContext struct {
	AppID uuid.UUID
	Key   string
}

// NewEnvValueCipher constructs one AES-256-GCM cipher for keyID.
func NewEnvValueCipher(keyID string, key []byte) (*EnvValueCipher, error) {
	trimmedKeyID := strings.TrimSpace(keyID)
	if trimmedKeyID == "" {
		return nil, errors.New("env value cipher requires non-empty key id")
	}
	if !isValidEnvValueKeyID(trimmedKeyID) {
		return nil, errors.New("env value cipher key id is invalid")
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

	return &EnvValueCipher{keyID: trimmedKeyID, aead: aead, randReader: rand.Reader}, nil
}

// EncryptForStorage encrypts plaintext with row-bound AAD.
func (c *EnvValueCipher) EncryptForStorage(plaintext string, rowCtx EnvValueRowContext) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(c.randReader, nonce); err != nil {
		return "", fmt.Errorf("generate env nonce: %w", err)
	}

	header := c.header()
	aad, err := rowBoundAAD(header, rowCtx)
	if err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), aad)

	return header +
		":" + base64.RawURLEncoding.EncodeToString(nonce) +
		":" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// DecryptFromStorage decrypts payload using row-bound AAD only.
func (c *EnvValueCipher) DecryptFromStorage(encoded string, rowCtx EnvValueRowContext) (string, error) {
	parts, err := parseEnvValueCiphertextParts(encoded)
	if err != nil {
		return "", ErrInvalidEnvValueCiphertext
	}
	return c.decryptFromParts(parts, rowCtx)
}

func parseEnvValueCiphertextParts(encoded string) ([]string, error) {
	parts := strings.Split(encoded, ":")
	if len(parts) != 5 {
		return nil, ErrInvalidEnvValueCiphertext
	}

	version, algo, keyID := parts[0], parts[1], parts[2]
	if version != envCipherVersionV1 || algo != envCipherAlgoAESGCM || !isValidEnvValueKeyID(keyID) {
		return nil, ErrInvalidEnvValueCiphertext
	}

	return parts, nil
}

func (c *EnvValueCipher) decryptFromParts(parts []string, rowCtx EnvValueRowContext) (string, error) {
	if len(parts) != 5 {
		return "", ErrInvalidEnvValueCiphertext
	}
	if parts[2] != c.keyID {
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
	aad, err := rowBoundAAD(header, rowCtx)
	if err != nil {
		return "", ErrInvalidEnvValueCiphertext
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", ErrInvalidEnvValueCiphertext
	}
	return string(plaintext), nil
}

func (c *EnvValueCipher) header() string {
	return envCipherVersionV1 + ":" + envCipherAlgoAESGCM + ":" + c.keyID
}

func isValidEnvValueKeyID(keyID string) bool {
	if keyID == "" || strings.Contains(keyID, ":") {
		return false
	}
	if strings.ContainsAny(keyID, " \t\r\n") {
		return false
	}
	return true
}

func rowBoundAAD(header string, rowCtx EnvValueRowContext) ([]byte, error) {
	if rowCtx.AppID == uuid.Nil {
		return nil, ErrInvalidEnvValueCiphertext
	}
	key := strings.TrimSpace(rowCtx.Key)
	if key == "" {
		return nil, ErrInvalidEnvValueCiphertext
	}
	return []byte(header + "|app_id=" + rowCtx.AppID.String() + "|key=" + key), nil
}

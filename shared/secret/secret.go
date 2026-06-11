// Package secret provides a tiny helper for encrypting stored secrets.
//
// It uses XChaCha20-Poly1305 and stores ciphertext as text in the form:
//
//	v1:xchacha20poly1305:<key-id>:<nonce-base64url>:<ciphertext-base64url>
//
// The package is generic, key-ID aware, and deliberately boring.
package secret

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-playground/validator/v10"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	cipherFormatVersionV1        = "v1"
	cipherFormatAlgorithmXChaCha = "xchacha20poly1305"
)

var (
	// ErrInvalidBox means the encoded payload is malformed, uses an
	// unexpected version or algorithm, has the wrong key ID, or fails AEAD
	// authentication.
	ErrInvalidBox = errors.New("invalid secret ciphertext")
	// ErrInvalidBoxConfig means the caller provided an invalid key ID or key.
	ErrInvalidBoxConfig = errors.New("invalid secret box config")

	boxValidator = validator.New(validator.WithRequiredStructEnabled())
)

type boxConfig struct {
	KeyID      string `validate:"required,max=64,printascii,excludesall=:/\\ "`
	EncodedKey string `validate:"required,base64,min=44,max=44"`
}

// Box encrypts and decrypts string secrets for one logical key ID.
type Box struct {
	keyID      string
	aead       cipher.AEAD
	randReader io.Reader
}

// NewBox constructs a Box from a key ID and a base64-encoded 32-byte secret key.
//
// The key ID is trimmed and validated so ciphertext headers stay predictable.
// The key bytes are copied before use so callers can reuse or zero their input
// slice after construction.
func NewBox(keyID, encodedKey string) (*Box, error) {
	cfg := boxConfig{KeyID: strings.TrimSpace(keyID), EncodedKey: strings.TrimSpace(encodedKey)}
	if err := boxValidator.Struct(cfg); err != nil {
		return nil, ErrInvalidBoxConfig
	}

	key, err := base64.StdEncoding.DecodeString(cfg.EncodedKey)
	if err != nil {
		return nil, ErrInvalidBoxConfig
	}
	if len(key) != chacha20poly1305.KeySize {
		return nil, ErrInvalidBoxConfig
	}

	keyCopy := append([]byte(nil), key...)
	aead, err := chacha20poly1305.NewX(keyCopy)
	if err != nil {
		return nil, fmt.Errorf("build xchacha20poly1305: %w", err)
	}

	return &Box{keyID: cfg.KeyID, aead: aead, randReader: rand.Reader}, nil
}

// Encrypt seals plaintext and returns a text payload that can be stored as-is.
//
// The output always contains the cipher format version, algorithm, key ID,
// nonce, and ciphertext.
func (b *Box) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(b.randReader, nonce); err != nil {
		return "", fmt.Errorf("generate secret nonce: %w", err)
	}

	sealed := b.aead.Seal(nil, nonce, []byte(plaintext), nil)
	return b.header() + ":" + base64.RawURLEncoding.EncodeToString(nonce) + ":" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a payload created by Encrypt and returns the original string.
//
// It treats malformed input and failed authentication as an invalid ciphertext
// so callers do not have to distinguish format errors from tampering.
func (b *Box) Decrypt(encoded string) (string, error) {
	parts, err := parseCiphertext(encoded)
	if err != nil {
		return "", ErrInvalidBox
	}
	if parts[2] != b.keyID {
		return "", ErrInvalidBox
	}

	nonce, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(nonce) != b.aead.NonceSize() {
		return "", ErrInvalidBox
	}

	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil {
		return "", ErrInvalidBox
	}

	plaintext, err := b.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrInvalidBox
	}

	return string(plaintext), nil
}

func (b *Box) header() string {
	return cipherFormatVersionV1 + ":" + cipherFormatAlgorithmXChaCha + ":" + b.keyID
}

func parseCiphertext(encoded string) ([]string, error) {
	parts := strings.Split(encoded, ":")
	if len(parts) != 5 {
		return nil, ErrInvalidBox
	}
	if parts[0] != cipherFormatVersionV1 || parts[1] != cipherFormatAlgorithmXChaCha {
		return nil, ErrInvalidBox
	}

	return parts, nil
}

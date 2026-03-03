// This file tests env value encryption/decryption primitives.
//
// These tests intentionally stay in service package and avoid DB access. They
// validate cryptographic behavior guarantees required by app create workflows.

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const testEnvCipherKey32 = "0123456789abcdef0123456789abcdef"

func TestEnvValueCipherEncryptDecryptRoundTrip(t *testing.T) {
	cipher, err := NewEnvValueCipher("test-v1", []byte(testEnvCipherKey32))
	require.NoError(t, err)

	encrypted, err := cipher.EncryptForStorage("postgres://local")
	require.NoError(t, err)
	require.NotEqual(t, "postgres://local", encrypted)
	require.True(t, strings.HasPrefix(encrypted, "v1:aesgcm:test-v1:"))

	decrypted, err := cipher.DecryptFromStorage(encrypted)
	require.NoError(t, err)
	require.Equal(t, "postgres://local", decrypted)
}

func TestEnvValueCipherUsesRandomNonce(t *testing.T) {
	cipher, err := NewEnvValueCipher("test-v1", []byte(testEnvCipherKey32))
	require.NoError(t, err)

	one, err := cipher.EncryptForStorage("same-value")
	require.NoError(t, err)
	two, err := cipher.EncryptForStorage("same-value")
	require.NoError(t, err)

	require.NotEqual(t, one, two)
}

func TestEnvValueCipherRejectsTamperedCiphertext(t *testing.T) {
	cipher, err := NewEnvValueCipher("test-v1", []byte(testEnvCipherKey32))
	require.NoError(t, err)

	encrypted, err := cipher.EncryptForStorage("postgres://local")
	require.NoError(t, err)

	parts := strings.Split(encrypted, ":")
	require.Len(t, parts, 5)
	payload := parts[4]
	if payload[len(payload)-1] == 'A' {
		parts[4] = payload[:len(payload)-1] + "B"
	} else {
		parts[4] = payload[:len(payload)-1] + "A"
	}
	tampered := strings.Join(parts, ":")

	_, err = cipher.DecryptFromStorage(tampered)
	require.ErrorIs(t, err, ErrInvalidEnvValueCiphertext)
}

func TestEnvValueCipherRejectsWrongKeyID(t *testing.T) {
	left, err := NewEnvValueCipher("kid-a", []byte(testEnvCipherKey32))
	require.NoError(t, err)
	right, err := NewEnvValueCipher("kid-b", []byte(testEnvCipherKey32))
	require.NoError(t, err)

	encrypted, err := left.EncryptForStorage("postgres://local")
	require.NoError(t, err)

	_, err = right.DecryptFromStorage(encrypted)
	require.ErrorIs(t, err, ErrInvalidEnvValueCiphertext)
}

func TestNewEnvValueCipherValidatesInputs(t *testing.T) {
	_, err := NewEnvValueCipher("", []byte(testEnvCipherKey32))
	require.Error(t, err)

	_, err = NewEnvValueCipher("kid", []byte("short"))
	require.Error(t, err)
}

// This file tests env value encryption/decryption primitives.
//
// These tests intentionally stay in service package and avoid DB access. They
// validate cryptographic behavior guarantees required by app create workflows.

package service

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

const testEnvCipherKey32 = "0123456789abcdef0123456789abcdef"
const testEnvCipherKeyAlt32 = "fedcba9876543210fedcba9876543210"

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

func TestEnvValueKeyringEncryptUsesActiveKey(t *testing.T) {
	keyring, err := NewEnvValueKeyring("kid-b", map[string][]byte{
		"kid-a": []byte(testEnvCipherKey32),
		"kid-b": []byte(testEnvCipherKeyAlt32),
	})
	require.NoError(t, err)

	encrypted, err := keyring.EncryptForStorage("postgres://local")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(encrypted, "v1:aesgcm:kid-b:"))
}

func TestEnvValueKeyringDecryptsByPayloadKeyID(t *testing.T) {
	legacyCipher, err := NewEnvValueCipher("kid-a", []byte(testEnvCipherKey32))
	require.NoError(t, err)
	activeCipher, err := NewEnvValueCipher("kid-b", []byte(testEnvCipherKeyAlt32))
	require.NoError(t, err)

	legacyEncrypted, err := legacyCipher.EncryptForStorage("legacy-secret")
	require.NoError(t, err)
	activeEncrypted, err := activeCipher.EncryptForStorage("active-secret")
	require.NoError(t, err)

	keyring, err := NewEnvValueKeyring("kid-b", map[string][]byte{
		"kid-a": []byte(testEnvCipherKey32),
		"kid-b": []byte(testEnvCipherKeyAlt32),
	})
	require.NoError(t, err)

	legacyDecrypted, err := keyring.DecryptFromStorage(legacyEncrypted)
	require.NoError(t, err)
	require.Equal(t, "legacy-secret", legacyDecrypted)

	activeDecrypted, err := keyring.DecryptFromStorage(activeEncrypted)
	require.NoError(t, err)
	require.Equal(t, "active-secret", activeDecrypted)
}

func TestEnvValueKeyringRejectsUnknownKeyID(t *testing.T) {
	legacyCipher, err := NewEnvValueCipher("kid-a", []byte(testEnvCipherKey32))
	require.NoError(t, err)

	encrypted, err := legacyCipher.EncryptForStorage("legacy-secret")
	require.NoError(t, err)

	keyring, err := NewEnvValueKeyring("kid-b", map[string][]byte{
		"kid-b": []byte(testEnvCipherKeyAlt32),
	})
	require.NoError(t, err)

	_, err = keyring.DecryptFromStorage(encrypted)
	require.ErrorIs(t, err, ErrInvalidEnvValueCiphertext)
}

func TestNewEnvValueKeyringValidatesInputs(t *testing.T) {
	_, err := NewEnvValueKeyring("", map[string][]byte{"kid-a": []byte(testEnvCipherKey32)})
	require.Error(t, err)

	_, err = NewEnvValueKeyring("kid-a", nil)
	require.Error(t, err)

	_, err = NewEnvValueKeyring("kid-b", map[string][]byte{"kid-a": []byte(testEnvCipherKey32)})
	require.Error(t, err)
}

func TestNewEnvValueReencryptWorkerValidatesKeyAlignment(t *testing.T) {
	pool := &pgxpool.Pool{}
	queries := &pgstore.Queries{}

	keyring, err := NewEnvValueKeyring("kid-a", map[string][]byte{
		"kid-a": []byte(testEnvCipherKey32),
		"kid-b": []byte(testEnvCipherKeyAlt32),
	})
	require.NoError(t, err)

	t.Run("rejects mismatched active key id", func(t *testing.T) {
		_, err := NewEnvValueReencryptWorker(EnvValueReencryptWorkerConfig{
			Pool:         pool,
			Queries:      queries,
			Keyring:      keyring,
			ActiveKeyID:  "kid-b",
			LoadedKeyIDs: []string{"kid-a", "kid-b"},
		})
		require.EqualError(t, err, "env reencrypt worker active key id must match keyring active key id")
	})

	t.Run("rejects missing active key in loaded ids", func(t *testing.T) {
		_, err := NewEnvValueReencryptWorker(EnvValueReencryptWorkerConfig{
			Pool:         pool,
			Queries:      queries,
			Keyring:      keyring,
			ActiveKeyID:  "kid-a",
			LoadedKeyIDs: []string{"kid-b"},
		})
		require.EqualError(t, err, "env reencrypt worker loaded keys must include active key id")
	})

	t.Run("rejects loaded key missing from keyring", func(t *testing.T) {
		_, err := NewEnvValueReencryptWorker(EnvValueReencryptWorkerConfig{
			Pool:         pool,
			Queries:      queries,
			Keyring:      keyring,
			ActiveKeyID:  "kid-a",
			LoadedKeyIDs: []string{"kid-a", "kid-c"},
		})
		require.EqualError(t, err, "env reencrypt worker loaded key id \"kid-c\" is missing in keyring")
	})

	t.Run("accepts aligned configuration", func(t *testing.T) {
		worker, err := NewEnvValueReencryptWorker(EnvValueReencryptWorkerConfig{
			Pool:         pool,
			Queries:      queries,
			Keyring:      keyring,
			ActiveKeyID:  "kid-a",
			LoadedKeyIDs: []string{"kid-a", "kid-b"},
		})
		require.NoError(t, err)
		require.NotNil(t, worker)
		require.True(t, worker.Enabled())
	})
}

func TestNewEnvValueCipherValidatesInputs(t *testing.T) {
	_, err := NewEnvValueCipher("", []byte(testEnvCipherKey32))
	require.Error(t, err)

	_, err = NewEnvValueCipher("kid", []byte("short"))
	require.Error(t, err)
}

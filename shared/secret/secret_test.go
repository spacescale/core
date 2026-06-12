package secret

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, errors.New("boom")
}

func TestNewCipher(t *testing.T) {
	t.Run("trims key id and copies key", func(t *testing.T) {
		key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
		box, err := NewBox("  prod-2026  ", key)
		require.NoError(t, err)

		encoded, err := box.Encrypt("hello")
		require.NoError(t, err)
		require.Contains(t, encoded, "v1:xchacha20poly1305:prod-2026:")
	})

	tests := []struct {
		name    string
		keyID   string
		keyLen  int
		wantErr error
	}{
		{name: "empty key id", keyID: "", keyLen: 32, wantErr: ErrInvalidBoxConfig},
		{name: "key id with colon", keyID: "prod:1", keyLen: 32, wantErr: ErrInvalidBoxConfig},
		{name: "key id with space", keyID: "prod key", keyLen: 32, wantErr: ErrInvalidBoxConfig},
		{name: "key id with slash", keyID: "prod/key", keyLen: 32, wantErr: ErrInvalidBoxConfig},
		{name: "key id too long", keyID: strings.Repeat("a", 65), keyLen: 32, wantErr: ErrInvalidBoxConfig},
		{name: "short key", keyID: "prod", keyLen: 31, wantErr: ErrInvalidBoxConfig},
		{name: "long key", keyID: "prod", keyLen: 33, wantErr: ErrInvalidBoxConfig},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewBox(tc.keyID, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, tc.keyLen)))
			require.Error(t, err)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestCipherEncryptDecryptRoundTrip(t *testing.T) {
	box, err := NewBox("prod-1", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	require.NoError(t, err)
	box.randReader = bytes.NewReader(bytes.Repeat([]byte{3}, box.aead.NonceSize()))

	encoded, err := box.Encrypt("secret-value")
	require.NoError(t, err)
	parts := strings.Split(encoded, ":")
	require.Len(t, parts, 5)
	require.Equal(t, "v1", parts[0])
	require.Equal(t, "xchacha20poly1305", parts[1])
	require.Equal(t, "prod-1", parts[2])
	require.Equal(t, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, box.aead.NonceSize())), parts[3])

	decoded, err := box.Decrypt(encoded)
	require.NoError(t, err)
	require.Equal(t, "secret-value", decoded)
}

func TestCipherEncryptRejectsNonceReadError(t *testing.T) {
	box, err := NewBox("prod-1", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	require.NoError(t, err)
	box.randReader = failingReader{}

	_, err = box.Encrypt("secret-value")
	require.Error(t, err)
	require.Contains(t, err.Error(), "generate secret nonce")
}

func TestCipherDecryptRejectsMalformedPayloads(t *testing.T) {
	box, err := NewBox("prod-1", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	require.NoError(t, err)
	validNonce := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, box.aead.NonceSize()))
	validCiphertext := base64.RawURLEncoding.EncodeToString([]byte("ciphertext"))

	tests := []struct {
		name    string
		encoded string
	}{
		{name: "wrong part count", encoded: "v1:xchacha20poly1305:prod-1"},
		{name: "wrong version", encoded: "v2:xchacha20poly1305:prod-1:" + validNonce + ":" + validCiphertext},
		{name: "wrong algorithm", encoded: "v1:aesgcm:prod-1:" + validNonce + ":" + validCiphertext},
		{name: "wrong key id", encoded: "v1:xchacha20poly1305:prod-2:" + validNonce + ":" + validCiphertext},
		{name: "bad nonce encoding", encoded: "v1:xchacha20poly1305:prod-1:!!:" + validCiphertext},
		{name: "short nonce", encoded: "v1:xchacha20poly1305:prod-1:" + base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3}) + ":" + validCiphertext},
		{name: "bad ciphertext encoding", encoded: "v1:xchacha20poly1305:prod-1:" + validNonce + ":!!"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := box.Decrypt(tc.encoded)
			require.ErrorIs(t, err, ErrInvalidBox)
		})
	}
}

func TestCipherDecryptRejectsTamperedCiphertext(t *testing.T) {
	box, err := NewBox("prod-1", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	require.NoError(t, err)
	box.randReader = bytes.NewReader(bytes.Repeat([]byte{3}, box.aead.NonceSize()))

	encoded, err := box.Encrypt("secret-value")
	require.NoError(t, err)
	parts := bytes.Split([]byte(encoded), []byte(":"))
	require.Len(t, parts, 5)

	ct, err := base64.RawURLEncoding.DecodeString(string(parts[4]))
	require.NoError(t, err)
	ct[len(ct)-1] ^= 0x01
	parts[4] = []byte(base64.RawURLEncoding.EncodeToString(ct))
	tampered := string(bytes.Join(parts, []byte(":")))

	_, err = box.Decrypt(tampered)
	require.ErrorIs(t, err, ErrInvalidBox)
}

func TestCipherEncryptAndDecryptEmptyPlaintext(t *testing.T) {
	box, err := NewBox("prod-1", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	require.NoError(t, err)
	box.randReader = bytes.NewReader(bytes.Repeat([]byte{3}, box.aead.NonceSize()))

	encoded, err := box.Encrypt("")
	require.NoError(t, err)

	decoded, err := box.Decrypt(encoded)
	require.NoError(t, err)
	require.Empty(t, decoded)
}

func TestCipherDecryptUsesSameKeyID(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	first, err := NewBox("prod-1", key)
	require.NoError(t, err)
	first.randReader = bytes.NewReader(bytes.Repeat([]byte{4}, first.aead.NonceSize()))

	encoded, err := first.Encrypt("secret-value")
	require.NoError(t, err)

	second, err := NewBox("prod-2", key)
	require.NoError(t, err)
	_, err = second.Decrypt(encoded)
	require.ErrorIs(t, err, ErrInvalidBox)
}

var _ io.Reader = failingReader{}

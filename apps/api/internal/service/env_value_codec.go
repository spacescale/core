// This file implements env-value encryption and background key-rotation sweeps.
//
// Encryption model:
// - Ciphertext format remains stable: `v1:aesgcm:<key-id>:<nonce>:<ciphertext>`.
// - New writes bind AEAD associated data (AAD) to stable row context
//   (`app_id` + env var key), so ciphertext cannot be replayed across rows.
// - Decrypt requires the same row context used during encryption.
//
// Rotation model:
// - Keyring encrypts with one active key id.
// - Keyring decrypts by ciphertext key id (active + previous keys loaded).
// - Re-encryption worker upgrades previous-key rows to active key in bounded
//   batches, with failure backoff to avoid hot-looping bad rows.

package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
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

const (
	defaultEnvReencryptBatchSize   = 100
	defaultEnvReencryptSweepPeriod = 30 * time.Second
	defaultEnvReencryptMaxFailures = 5
	defaultEnvReencryptFailBackoff = 5 * time.Minute
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
// encryptions. Values must be stable across reads/writes for the same row.
type EnvValueRowContext struct {
	AppID uuid.UUID
	Key   string
}

// EnvValueKeyring encrypts with one active key and decrypts by payload key id.
type EnvValueKeyring struct {
	activeKeyID string
	ciphers     map[string]*EnvValueCipher
}

// EnvValueReencryptWorkerConfig defines dependencies and scheduling behavior for
// background key-rotation migrations.
type EnvValueReencryptWorkerConfig struct {
	Pool         *pgxpool.Pool
	Queries      *pgstore.Queries
	Keyring      *EnvValueKeyring
	ActiveKeyID  string
	LoadedKeyIDs []string
	BatchSize    int
	SweepPeriod  time.Duration
	Logger       *slog.Logger
}

// EnvValueReencryptWorker incrementally migrates env var ciphertext from
// non-active keys to the active key while preserving API availability.
type EnvValueReencryptWorker struct {
	pool           *pgxpool.Pool
	queries        *pgstore.Queries
	keyring        *EnvValueKeyring
	activeKeyID    string
	previousKeyIDs []string
	batchSize      int
	sweepPeriod    time.Duration
	logger         *slog.Logger
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

// NewEnvValueKeyring constructs a keyring for rotation windows.
func NewEnvValueKeyring(activeKeyID string, keys map[string][]byte) (*EnvValueKeyring, error) {
	trimmedActiveKeyID := strings.TrimSpace(activeKeyID)
	if trimmedActiveKeyID == "" {
		return nil, errors.New("env value keyring requires non-empty active key id")
	}
	if len(keys) == 0 {
		return nil, errors.New("env value keyring requires at least one key")
	}

	ciphers := make(map[string]*EnvValueCipher, len(keys))
	for rawKeyID, key := range keys {
		trimmedKeyID := strings.TrimSpace(rawKeyID)
		if _, exists := ciphers[trimmedKeyID]; exists {
			return nil, errors.New("env value keyring has duplicate key id")
		}
		cipher, err := NewEnvValueCipher(trimmedKeyID, key)
		if err != nil {
			return nil, fmt.Errorf("invalid env value keyring key %q: %w", trimmedKeyID, err)
		}
		ciphers[trimmedKeyID] = cipher
	}

	if _, exists := ciphers[trimmedActiveKeyID]; !exists {
		return nil, errors.New("env value keyring missing active key id")
	}

	return &EnvValueKeyring{activeKeyID: trimmedActiveKeyID, ciphers: ciphers}, nil
}

// EncryptForStorage encrypts plaintext with row-bound AAD.
//
// Row-bound AAD ties ciphertext to one env var row identity and prevents
// ciphertext replay between rows that share the same key id.
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

// EncryptForStorage encrypts using the keyring active key id.
func (k *EnvValueKeyring) EncryptForStorage(plaintext string, rowCtx EnvValueRowContext) (string, error) {
	active, exists := k.ciphers[k.activeKeyID]
	if !exists {
		return "", errors.New("env value keyring missing active cipher")
	}
	return active.EncryptForStorage(plaintext, rowCtx)
}

// DecryptFromStorage routes decryption by payload key id.
func (k *EnvValueKeyring) DecryptFromStorage(encoded string, rowCtx EnvValueRowContext) (string, error) {
	parts, err := parseEnvValueCiphertextParts(encoded)
	if err != nil {
		return "", err
	}

	keyID := parts[2]
	cipher, exists := k.ciphers[keyID]
	if !exists {
		return "", ErrInvalidEnvValueCiphertext
	}

	return cipher.decryptFromParts(parts, rowCtx)
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

// NewEnvValueReencryptWorker builds a background worker that migrates
// non-active-key ciphertext to the active key in bounded sweeps.
func NewEnvValueReencryptWorker(cfg EnvValueReencryptWorkerConfig) (*EnvValueReencryptWorker, error) {
	if cfg.Pool == nil {
		return nil, errors.New("env reencrypt worker requires non-nil db pool")
	}
	if cfg.Queries == nil {
		return nil, errors.New("env reencrypt worker requires non-nil queries")
	}
	if cfg.Keyring == nil {
		return nil, errors.New("env reencrypt worker requires non-nil keyring")
	}

	active := strings.TrimSpace(cfg.ActiveKeyID)
	if active == "" {
		return nil, errors.New("env reencrypt worker requires non-empty active key id")
	}
	if cfg.Keyring.activeKeyID != active {
		return nil, errors.New("env reencrypt worker active key id must match keyring active key id")
	}
	if _, exists := cfg.Keyring.ciphers[active]; !exists {
		return nil, errors.New("env reencrypt worker active key id is missing in keyring")
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = defaultEnvReencryptBatchSize
	}
	sweepPeriod := cfg.SweepPeriod
	if sweepPeriod <= 0 {
		sweepPeriod = defaultEnvReencryptSweepPeriod
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	loadedSet := make(map[string]struct{}, len(cfg.LoadedKeyIDs))
	activePresentInLoaded := false
	for _, raw := range cfg.LoadedKeyIDs {
		keyID := strings.TrimSpace(raw)
		if keyID == "" {
			continue
		}
		loadedSet[keyID] = struct{}{}
		if keyID == active {
			activePresentInLoaded = true
		}
	}
	if !activePresentInLoaded {
		return nil, errors.New("env reencrypt worker loaded keys must include active key id")
	}
	for keyID := range loadedSet {
		if _, exists := cfg.Keyring.ciphers[keyID]; !exists {
			return nil, fmt.Errorf("env reencrypt worker loaded key id %q is missing in keyring", keyID)
		}
	}

	previousSet := make(map[string]struct{})
	for keyID := range loadedSet {
		if keyID == active {
			continue
		}
		previousSet[keyID] = struct{}{}
	}
	previous := make([]string, 0, len(previousSet))
	for keyID := range previousSet {
		previous = append(previous, keyID)
	}
	sort.Strings(previous)

	return &EnvValueReencryptWorker{
		pool:           cfg.Pool,
		queries:        cfg.Queries,
		keyring:        cfg.Keyring,
		activeKeyID:    active,
		previousKeyIDs: previous,
		batchSize:      batchSize,
		sweepPeriod:    sweepPeriod,
		logger:         logger,
	}, nil
}

// Enabled reports whether there are non-active keys to migrate.
func (w *EnvValueReencryptWorker) Enabled() bool {
	return len(w.previousKeyIDs) > 0
}

// Run is a blocking sweep loop. Start it in a dedicated goroutine from main.
// The loop blocks in a select waiting on ctx cancellation or ticker ticks.
func (w *EnvValueReencryptWorker) Run(ctx context.Context) {
	if !w.Enabled() {
		w.logger.Info("env re-encryption worker disabled", "reason", "no previous keys configured")
		return
	}

	w.logger.Info("env re-encryption worker started",
		"active_key_id", w.activeKeyID,
		"previous_keys", len(w.previousKeyIDs),
		"batch_size", w.batchSize,
		"sweep_period", w.sweepPeriod.String(),
	)
	w.sweepOnce(ctx)

	ticker := time.NewTicker(w.sweepPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("env re-encryption worker stopped")
			return
		case <-ticker.C:
			w.sweepOnce(ctx)
		}
	}
}

func (w *EnvValueReencryptWorker) sweepOnce(ctx context.Context) {
	for _, previousKeyID := range w.previousKeyIDs {
		for {
			if ctx.Err() != nil {
				return
			}

			n, err := w.reencryptBatchForKey(ctx, previousKeyID)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				w.logger.Error("env re-encryption batch failed", "previous_key_id", previousKeyID, "error", err)
				break
			}
			if n == 0 {
				break
			}
			w.logger.Info("env re-encryption batch complete", "previous_key_id", previousKeyID, "rows_reencrypted", n)
		}
	}
}

func (w *EnvValueReencryptWorker) reencryptBatchForKey(ctx context.Context, previousKeyID string) (int, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	txQueries := w.queries.WithTx(tx)
	rows, err := txQueries.ClaimAppEnvVarsByKeyID(ctx, pgstore.ClaimAppEnvVarsByKeyIDParams{
		KeyID:          &previousKeyID,
		MaxFailCount:   int32(defaultEnvReencryptMaxFailures),
		BackoffSeconds: int32(defaultEnvReencryptFailBackoff / time.Second),
		LimitRows:      int32(w.batchSize),
	})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		return 0, nil
	}

	reencrypted := 0
	for _, row := range rows {
		rowCtx := EnvValueRowContext{AppID: row.AppID, Key: row.Key}
		plaintext, err := w.keyring.DecryptFromStorage(row.ValueEncrypted, rowCtx)
		if err != nil {
			if markErr := txQueries.MarkAppEnvVarReencryptFailure(ctx, row.ID); markErr != nil {
				return reencrypted, markErr
			}
			w.logger.Error("env re-encryption decrypt failed; row deferred",
				"env_var_id", row.ID.String(),
				"previous_key_id", previousKeyID,
				"error", err,
			)
			continue
		}

		newCiphertext, err := w.keyring.EncryptForStorage(plaintext, rowCtx)
		if err != nil {
			return reencrypted, err
		}

		updatedRows, err := txQueries.UpdateAppEnvVarCiphertextCAS(ctx, pgstore.UpdateAppEnvVarCiphertextCASParams{
			EnvVarID:           row.ID,
			NewCiphertext:      newCiphertext,
			PreviousCiphertext: row.ValueEncrypted,
		})
		if err != nil {
			return reencrypted, err
		}
		if updatedRows == 1 {
			if err := txQueries.ResetAppEnvVarReencryptFailure(ctx, row.ID); err != nil {
				return reencrypted, err
			}
			reencrypted++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return reencrypted, err
	}
	return reencrypted, nil
}

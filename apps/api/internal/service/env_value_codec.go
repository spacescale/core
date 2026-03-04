// This file implements authenticated encryption for app environment-variable
// values and background key-rotation re-encryption.
//
// Design intent:
// - Keep ciphertext format stable (`v1`) while rotating key material via key ids.
// - Encrypt all env var values before persistence.
// - Support active-key encryption and multi-key decrypt during rotation windows.

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

	"github.com/jackc/pgx/v5/pgxpool"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

const (
	envCipherVersionV1  = "v1"     // current storage payload format version.
	envCipherAlgoAESGCM = "aesgcm" // authenticated cipher identifier in payload header.
)

var (
	// ErrInvalidEnvValueCiphertext indicates malformed payload or failed AEAD auth.
	ErrInvalidEnvValueCiphertext = errors.New("invalid env value ciphertext")
)

const (
	defaultEnvReencryptBatchSize   = 100
	defaultEnvReencryptSweepPeriod = 30 * time.Second
)

// EnvValueCipher encrypts/decrypts env values for DB persistence.
//
// Storage format:
//
//	v1:aesgcm:<key-id>:<nonce-base64url>:<ciphertext-base64url>
type EnvValueCipher struct {
	keyID      string
	aead       cipher.AEAD
	randReader io.Reader
}

// EnvValueKeyring encrypts with one active key and decrypts by payload key-id.
type EnvValueKeyring struct {
	activeKeyID string
	ciphers     map[string]*EnvValueCipher
}

// EnvValueReencryptWorkerConfig defines runtime dependencies and sweep behavior.
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

// EnvValueReencryptWorker incrementally migrates legacy-key ciphertext to the
// current active key.
type EnvValueReencryptWorker struct {
	pool         *pgxpool.Pool
	queries      *pgstore.Queries
	keyring      *EnvValueKeyring
	activeKeyID  string
	legacyKeyIDs []string
	batchSize    int
	sweepPeriod  time.Duration
	logger       *slog.Logger
}

// NewEnvValueCipher constructs one AES-256-GCM cipher for a single key-id.
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

	return &EnvValueCipher{
		keyID:      trimmedKeyID,
		aead:       aead,
		randReader: rand.Reader,
	}, nil
}

// NewEnvValueKeyring constructs a rotation-safe keyring.
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

// EncryptForStorage encrypts plaintext with this cipher key id.
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

// DecryptFromStorage decrypts payload with strict key-id ownership.
func (c *EnvValueCipher) DecryptFromStorage(encoded string) (string, error) {
	parts, err := parseEnvValueCiphertextParts(encoded)
	if err != nil {
		return "", ErrInvalidEnvValueCiphertext
	}
	return c.decryptFromParts(parts)
}

// EncryptForStorage encrypts plaintext using the active key id.
func (k *EnvValueKeyring) EncryptForStorage(plaintext string) (string, error) {
	active, exists := k.ciphers[k.activeKeyID]
	if !exists {
		return "", errors.New("env value keyring missing active cipher")
	}
	return active.EncryptForStorage(plaintext)
}

// DecryptFromStorage decrypts payload by routing on payload key-id.
func (k *EnvValueKeyring) DecryptFromStorage(encoded string) (string, error) {
	parts, err := parseEnvValueCiphertextParts(encoded)
	if err != nil {
		return "", err
	}

	keyID := parts[2]
	cipher, exists := k.ciphers[keyID]
	if !exists {
		return "", ErrInvalidEnvValueCiphertext
	}

	return cipher.decryptFromParts(parts)
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

func (c *EnvValueCipher) decryptFromParts(parts []string) (string, error) {
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
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(header))
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

// NewEnvValueReencryptWorker builds a background worker that migrates legacy
// ciphertext to the active key in bounded sweeps.
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

	legacySet := make(map[string]struct{})
	for keyID := range loadedSet {
		if keyID == active {
			continue
		}
		legacySet[keyID] = struct{}{}
	}
	legacy := make([]string, 0, len(legacySet))
	for keyID := range legacySet {
		legacy = append(legacy, keyID)
	}
	sort.Strings(legacy)

	return &EnvValueReencryptWorker{
		pool:         cfg.Pool,
		queries:      cfg.Queries,
		keyring:      cfg.Keyring,
		activeKeyID:  active,
		legacyKeyIDs: legacy,
		batchSize:    batchSize,
		sweepPeriod:  sweepPeriod,
		logger:       logger,
	}, nil
}

// Enabled reports whether there are legacy keys to migrate.
func (w *EnvValueReencryptWorker) Enabled() bool {
	return len(w.legacyKeyIDs) > 0
}

// Run is a blocking sweep loop. Start it in a dedicated goroutine from main.
// The loop blocks in a select waiting on ctx cancellation or ticker ticks.
func (w *EnvValueReencryptWorker) Run(ctx context.Context) {
	if !w.Enabled() {
		w.logger.Info("env re-encryption worker disabled", "reason", "no legacy keys configured")
		return
	}

	w.logger.Info("env re-encryption worker started",
		"active_key_id", w.activeKeyID,
		"legacy_keys", len(w.legacyKeyIDs),
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
	for _, legacyKeyID := range w.legacyKeyIDs {
		for {
			if ctx.Err() != nil {
				return
			}

			n, err := w.reencryptBatchForKey(ctx, legacyKeyID)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				w.logger.Error("env re-encryption batch failed", "legacy_key_id", legacyKeyID, "error", err)
				break
			}
			if n == 0 {
				break
			}
			w.logger.Info("env re-encryption batch complete", "legacy_key_id", legacyKeyID, "rows_reencrypted", n)
		}
	}
}

func (w *EnvValueReencryptWorker) reencryptBatchForKey(ctx context.Context, legacyKeyID string) (int, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	txQueries := w.queries.WithTx(tx)
	rows, err := txQueries.ClaimAppEnvVarsByKeyID(ctx, pgstore.ClaimAppEnvVarsByKeyIDParams{
		KeyID:     &legacyKeyID,
		LimitRows: int32(w.batchSize),
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
		plaintext, err := w.keyring.DecryptFromStorage(row.ValueEncrypted)
		if err != nil {
			w.logger.Error("env re-encryption decrypt failed; leaving row unchanged",
				"env_var_id", row.ID.String(),
				"legacy_key_id", legacyKeyID,
				"error", err,
			)
			continue
		}

		newCiphertext, err := w.keyring.EncryptForStorage(plaintext)
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
			reencrypted++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return reencrypted, err
	}
	return reencrypted, nil
}

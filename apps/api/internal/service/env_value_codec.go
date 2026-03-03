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

// Cipher payload header identifiers.
//
// Stored format is: v1:aesgcm:<key-id>:<nonce>:<ciphertext>.
// Version and algorithm are validated before any cryptographic operation.
const (
	envCipherVersionV1  = "v1"     // current storage payload format version.
	envCipherAlgoAESGCM = "aesgcm" // authenticated cipher identifier in payload header.
)

var (
	// ErrInvalidEnvValueCiphertext is returned when payload structure is invalid,
	// key-id is unknown/mismatched, base64 decoding fails, or AEAD authentication
	// verification fails during decrypt.
	ErrInvalidEnvValueCiphertext = errors.New("invalid env value ciphertext")
)

// Default sweep settings used when worker config does not provide explicit
// values. These defaults keep migration throughput bounded and predictable.
const (
	defaultEnvReencryptBatchSize   = 100
	defaultEnvReencryptSweepPeriod = 30 * time.Second
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

// EnvValueKeyring encrypts with one active key and decrypts with key-id based
// lookup across all loaded keys.
type EnvValueKeyring struct {
	activeKeyID string                     // key id used for new encryption operations.
	ciphers     map[string]*EnvValueCipher // all loaded ciphers indexed by key id.
}

// EnvValueReencryptWorkerConfig defines runtime dependencies and sweep behavior
// for background ciphertext migration.
//
// Config contract:
//   - Pool/Queries/Keyring are required.
//   - ActiveKeyID must match keyring active encryption identity.
//   - LoadedKeyIDs should include active + legacy decrypt-capable ids; the worker
//     derives legacy ids from this list.
//   - BatchSize and SweepPeriod use safe defaults when zero/negative.
//   - Logger defaults to slog.Default() when nil.
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

// EnvValueReencryptWorker incrementally migrates legacy env-value ciphertext to
// the active key in bounded, retryable background sweeps.
//
// The worker is intentionally best-effort:
// - it logs and continues on non-fatal batch failures,
// - it stops cooperatively on context cancellation,
// - and it relies on CAS writes to avoid clobbering concurrent updates.
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
//
// Validation behavior:
// - keyID is trimmed and must be non-empty.
// - keyID must not contain ':' or whitespace so payload parsing remains stable.
// - key must be exactly 32 bytes (AES-256 key size).
//
// Security behavior:
//   - key bytes are copied before cipher initialization so callers can safely
//     mutate their source slice without affecting this instance.
//   - nonce entropy source defaults to crypto/rand.Reader.
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
//
// Keyring behavior:
// - New encryptions always use activeKeyID.
// - Decryption selects key material from ciphertext key-id.
//
// Validation behavior:
// - activeKeyID must be non-empty.
// - at least one key must be provided.
// - every entry is validated through NewEnvValueCipher.
// - activeKeyID must exist in keys.
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

	return &EnvValueKeyring{
		activeKeyID: trimmedActiveKeyID,
		ciphers:     ciphers,
	}, nil
}

// EncryptForStorage encrypts plaintext and returns one storage payload encoded
// as `v1:aesgcm:<key-id>:<nonce-b64url>:<ciphertext-b64url>`.
//
// The payload header (version/algo/key-id) is passed as AEAD associated data,
// so header tampering is detected during decrypt.
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

// DecryptFromStorage decrypts one payload using this cipher instance only.
//
// This method is strict about key ownership: payload key-id must match c.keyID.
// For rotation windows across multiple key ids, callers should prefer
// EnvValueKeyring.DecryptFromStorage.
func (c *EnvValueCipher) DecryptFromStorage(encoded string) (string, error) {
	parts, err := parseEnvValueCiphertextParts(encoded)
	if err != nil {
		return "", ErrInvalidEnvValueCiphertext
	}
	return c.decryptFromParts(parts)
}

// EncryptForStorage encrypts plaintext using the keyring active key id.
//
// This method is the write-path entrypoint used by service workflows so all new
// rows converge on one current key version.
func (k *EnvValueKeyring) EncryptForStorage(plaintext string) (string, error) {
	active, exists := k.ciphers[k.activeKeyID]
	if !exists {
		return "", errors.New("env value keyring missing active cipher")
	}
	return active.EncryptForStorage(plaintext)
}

// DecryptFromStorage decrypts ciphertext by resolving key-id from payload and
// selecting the matching cipher from the keyring.
//
// This enables non-breaking key rotation where legacy ciphertext remains
// decryptable while new writes use the active key.
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

// parseEnvValueCiphertextParts validates storage payload structure and header.
//
// Accepted format: `v1:aesgcm:<key-id>:<nonce>:<ciphertext>`.
// This function only validates shape/version/algo/key-id format; cryptographic
// verification happens later in decryptFromParts.
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

// decryptFromParts performs strict single-key decrypt after payload parsing.
// It expects a five-part payload and rejects key-id mismatch for this cipher.
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

// header returns the authenticated payload prefix for this cipher instance.
func (c *EnvValueCipher) header() string {
	return envCipherVersionV1 + ":" + envCipherAlgoAESGCM + ":" + c.keyID
}

// isValidEnvValueKeyID enforces key-id constraints required by payload parsing.
// Key ids must be non-empty and cannot contain separator ':' or whitespace.
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
//
// The worker derives its legacy key set by subtracting ActiveKeyID from
// LoadedKeyIDs, then sorts that list for deterministic traversal order.
//
// Validation behavior:
//   - ActiveKeyID must match keyring active key id.
//   - ActiveKeyID must exist in keyring and in LoadedKeyIDs.
//   - Every LoadedKeyID must be present in keyring, preventing sweep loops from
//     claiming rows that are not decryptable by configured key material.
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

// Enabled reports whether there are legacy key ids to migrate.
// When false, Run exits immediately after one informational log line.
func (w *EnvValueReencryptWorker) Enabled() bool {
	return len(w.legacyKeyIDs) > 0
}

// Run starts the background sweep loop that upgrades legacy env-value
// ciphertext to the current active key.
//
// Lifecycle behavior:
//   - If no legacy keys are configured, the worker logs a disabled message and
//     returns immediately.
//   - Otherwise it performs one immediate sweep on startup, then continues on a
//     periodic ticker until ctx is canceled.
//   - Cancellation is cooperative; Run exits when ctx is done and logs shutdown.
//   - Run is a blocking call. It is expected to be launched in its own
//     goroutine (as done in `main`) so the process main goroutine can continue
//     serving HTTP and handling shutdown signals.
//   - Internally, a `select` waits on either `ctx.Done()` or ticker ticks,
//     which keeps this goroutine parked between sweep iterations.
//
// Data safety and idempotency:
//   - Actual row selection and rewrite are delegated to reencryptBatchForKey,
//     which uses bounded batches, row locks, and compare-and-swap updates.
//   - Re-running sweeps is safe: already-upgraded rows are skipped and no-op.
//
// Operational intent:
//   - This loop allows key rotation windows where new writes use the active key
//     while old ciphertext is migrated incrementally in the background.
//   - Errors in one batch are logged and the sweep proceeds on later intervals,
//     preserving service availability over strict fail-fast behavior.
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
		// This select has no default case, so it blocks (waits) this worker
		// goroutine until one channel is ready:
		// - ctx.Done(): shutdown/cancel signal
		// - ticker.C: next scheduled sweep tick
		// While blocked here, only this goroutine is parked; other goroutines keep
		// running normally.
		select {
		case <-ctx.Done():
			w.logger.Info("env re-encryption worker stopped")
			return
		case <-ticker.C:
			w.sweepOnce(ctx)

		}
	}
}

// sweepOnce processes all configured legacy keys until each key has no more
// claimable rows in the current pass.
//
// For each legacy key id, it repeatedly executes reencryptBatchForKey until a
// batch returns zero migrated rows. Context cancellation short-circuits the
// sweep to support fast shutdown behavior.
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
				w.logger.Error("env re-encryption batch failed",
					"legacy_key_id", legacyKeyID,
					"error", err,
				)
				break
			}
			if n == 0 {
				break
			}
			w.logger.Info("env re-encryption batch complete",
				"legacy_key_id", legacyKeyID,
				"rows_reencrypted", n,
			)
		}
	}
}

// reencryptBatchForKey migrates one bounded batch of rows encrypted with a
// specific legacy key id.
//
// Batch behavior:
// - Starts one transaction.
// - Claims rows by key id using SKIP LOCKED query semantics.
// - Decrypts with keyring, re-encrypts with active key, writes via CAS update.
// - Commits transaction and returns migrated row count.
//
// Concurrency behavior:
//   - SKIP LOCKED allows multiple workers/processes to sweep concurrently without
//     blocking each other on the same rows.
//   - CAS update prevents stale overwrite when a row was modified after claim.
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
		KeyID:     legacyKeyID,
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

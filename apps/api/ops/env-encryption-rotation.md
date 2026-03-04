# Env Encryption Rotation Runbook

This runbook covers key rotation for app env var encryption in the API.

## What this protects

- App env var values are encrypted at rest with AES-256-GCM.
- Ciphertext format is stable: `v1:aesgcm:<key-id>:<nonce>:<ciphertext>`.
- AEAD associated data (AAD) is bound to row context (`app_id` + env var key), so ciphertext cannot be replayed across rows.

## Important runtime facts

- Env config is loaded at process startup.
- Key changes require a restart/redeploy.
- One active key is used for new encryption.
- Previous keys can be loaded for decrypt during migration.

## Required env vars

- `API_ENV_ENCRYPTION_KEY_ID` - active key id for new encryptions.
- `API_ENV_ENCRYPTION_KEY` - base64-encoded 32-byte key.
- `API_ENV_ENCRYPTION_DECRYPT_KEYS` (optional) - comma-separated `key_id:base64key` pairs for previous keys.
- `API_ENV_ENCRYPTION_REENCRYPT_BATCH_SIZE` (optional) - rows per worker batch.
- `API_ENV_ENCRYPTION_REENCRYPT_SWEEP_PERIOD` (optional) - worker sweep interval (duration string, e.g. `30s`).

Key id rules:

- Must not contain `:`
- Must not contain whitespace

## Generate a new key

```bash
openssl rand -base64 32
```

## Rotation procedure

1. **Pick new key id**
   - Example: current `prod-k1` -> new `prod-k2`

2. **Prepare rollout config**
   - Set active to new key:
     - `API_ENV_ENCRYPTION_KEY_ID=prod-k2`
     - `API_ENV_ENCRYPTION_KEY=<prod-k2-base64>`
   - Keep old key as decrypt-only:
     - `API_ENV_ENCRYPTION_DECRYPT_KEYS=prod-k1:<prod-k1-base64>`

3. **Deploy/restart API**
   - New writes immediately use `prod-k2`.
   - Re-encryption worker starts migrating old rows from `prod-k1` -> `prod-k2`.

4. **Monitor migration**
   - Remaining rows on old key:

```sql
SELECT count(*)
FROM app_env_vars
WHERE cipher_version = 'v1'
  AND cipher_algo = 'aesgcm'
  AND cipher_key_id = 'prod-k1';
```

- Failure/backoff visibility:

```sql
SELECT id, app_id, key, reencrypt_fail_count, reencrypt_failed_at
FROM app_env_vars
WHERE reencrypt_fail_count > 0
ORDER BY reencrypt_failed_at DESC NULLS LAST
LIMIT 100;
```

5. **Finalize rotation**
   - When old key count reaches `0`, remove `prod-k1` from `API_ENV_ENCRYPTION_DECRYPT_KEYS`.
   - Deploy/restart again.

## Rollback

If rollout fails, revert to previous active key and keep both keys available for decrypt.

Example rollback config:

- `API_ENV_ENCRYPTION_KEY_ID=prod-k1`
- `API_ENV_ENCRYPTION_KEY=<prod-k1-base64>`
- `API_ENV_ENCRYPTION_DECRYPT_KEYS=prod-k2:<prod-k2-base64>`

Then redeploy/restart.

## Worker behavior notes

- Worker claims rows in bounded batches (`FOR UPDATE SKIP LOCKED`).
- Rows that repeatedly fail decrypt are backoff-limited using:
  - `reencrypt_fail_count`
  - `reencrypt_failed_at`
- Claim path excludes rows over fail threshold until backoff window passes.

## Preconditions for strict row-context decrypt

Current runtime decrypt is strict row-context-bound. If your DB contains old ciphertext that was not encrypted with row context, those rows will fail decrypt and must be remediated before rollout.

If this is a fresh environment with no old encrypted data, you are safe.

## Security checklist

- Never commit real key material to git.
- Store production keys in your secret manager / deployment secret store.
- Restrict DB write access tightly.
- Rotate keys on a regular schedule and immediately on suspected compromise.

#!/usr/bin/env bash
set -euo pipefail

# Edit this block if the test server changes.
IP="65.109.67.102"
SSH_TARGET="root@65.109.67.102"
REGION="eu-central"
PROVIDER_LOCATION="fsn1"
PROVIDER_ID="colo"
PROVIDER_SERVER_ID="manual-65.109.67.102"
STATE_DIR="/var/lib/spacescale"
INSTALL_PATH="/usr/local/bin/scaled"

if [[ $# -gt 0 ]]; then
  printf 'This script is hardcoded for the current test node. Edit the values at the top instead of passing flags.\n' >&2
  exit 1
fi

# Use Docker's psql so the script does not depend on a local Postgres install.
psql_db() {
  docker-compose exec -T db psql -U spacescale -d spacescale "$@"
}

# Generate a fresh one-time bootstrap token for this provisioning run.
BOOTSTRAP_TOKEN="$(python3 - <<'PY'
import secrets
print(secrets.token_hex(32))
PY
)"

# Only the hash is stored in Postgres. The raw token goes onto the host.
BOOTSTRAP_TOKEN_HASH="$(python3 - <<'PY' "$BOOTSTRAP_TOKEN"
import hashlib
import sys
print(hashlib.sha256(sys.argv[1].encode()).hexdigest())
PY
)"

# Create or reset the test node back to provisioning with the fresh token hash.
psql_db -X -q -v ON_ERROR_STOP=1 <<SQL
INSERT INTO nodes (
  provider,
  provider_server_id,
  primary_ipv4,
  region,
  provider_location,
  bootstrap_token_hash
)
VALUES (
  '${PROVIDER_ID}',
  '${PROVIDER_SERVER_ID}',
  '${IP}',
  '${REGION}',
  '${PROVIDER_LOCATION}',
  '${BOOTSTRAP_TOKEN_HASH}'
)
ON CONFLICT (primary_ipv4) DO UPDATE
SET provider = EXCLUDED.provider,
    provider_server_id = EXCLUDED.provider_server_id,
    region = EXCLUDED.region,
    provider_location = EXCLUDED.provider_location,
    total_cores = 0,
    total_ram_mb = 0,
    total_disk_mb = 0,
    status = 'provisioning',
    bootstrap_token_hash = EXCLUDED.bootstrap_token_hash,
    updated_at = now()
RETURNING id, primary_ipv4, region, provider_location, status;
SQL

# Install the bootstrap token where scaled expects it on first boot.
# shellcheck disable=SC2029
ssh "$SSH_TARGET" "install -d -m 0755 '$STATE_DIR'"
# shellcheck disable=SC2029
printf '%s\n' "$BOOTSTRAP_TOKEN" | ssh "$SSH_TARGET" "cat > '$STATE_DIR/bootstrap_token' && chmod 600 '$STATE_DIR/bootstrap_token'"

# Build a fresh linux amd64 scaled binary, copy it to the host, and clean up locally.
mkdir -p bin
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/scaled ./cmd/scaled
scp "bin/scaled" "$SSH_TARGET:/tmp/scaled"
# shellcheck disable=SC2029
ssh "$SSH_TARGET" "install -m 0755 /tmp/scaled '$INSTALL_PATH' && rm -f /tmp/scaled"
rm -f "bin/scaled"
rmdir "bin" >/dev/null 2>&1 || true

cat <<EOF
Provisioned manual node for $IP

Notes for scaled:
  bootstrap token written to $STATE_DIR/bootstrap_token on $SSH_TARGET
  identity file will be created at $STATE_DIR/identity.json after the first bootstrap handshake
  binary installed at $INSTALL_PATH
EOF

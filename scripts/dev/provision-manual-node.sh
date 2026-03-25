#!/usr/bin/env bash
set -euo pipefail

# Edit this block if the test server changes.
IP="65.109.67.102"
SSH_TARGET="root@65.109.67.102"
REGION="eu-central"
PROVIDER_LOCATION="fsn1"
PROVIDER_ID="manual"
PROVIDER_NAME="manual"
PROVIDER_SERVER_ID="manual-65.109.67.102"
TIER="shared"
CPU_CORES="1"
HOST_OS_FAMILY="debian"
HOST_OS_VERSION="13"
HOST_IMAGE_REF="manual-install"
STATE_DIR="/var/lib/spacescale"
INSTALL_PATH="/usr/local/bin/scaled"

if [[ $# -gt 0 ]]; then
  printf 'This script is hardcoded for the current test node. Edit the values at the top instead of passing flags.\n' >&2
  exit 1
fi

# Use Docker's psql so the script does not depend on a local Postgres install.
psql_db() {
  docker compose exec -T db psql -U spacescale -d spacescale "$@"
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

# Ensure the manual provider exists before seeding the metal row.
psql_db -X -q -v ON_ERROR_STOP=1 <<SQL
INSERT INTO providers (id, name, api_token_encrypted)
VALUES ('${PROVIDER_ID}', '${PROVIDER_NAME}', 'manual-placeholder')
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = now();
SQL

# Create or reset the test metal back to provisioning with the fresh token hash.
psql_db -X -q -v ON_ERROR_STOP=1 <<SQL
INSERT INTO metals (
  provider_id,
  provider_server_id,
  primary_ipv4,
  host_os_family,
  host_os_version,
  host_image_ref,
  region,
  provider_location,
  tier_target,
  total_cpu_core,
  bootstrap_token_hash
)
VALUES (
  '${PROVIDER_ID}',
  '${PROVIDER_SERVER_ID}',
  '${IP}',
  '${HOST_OS_FAMILY}',
  '${HOST_OS_VERSION}',
  '${HOST_IMAGE_REF}',
  '${REGION}',
  '${PROVIDER_LOCATION}',
  '${TIER}',
  ${CPU_CORES},
  '${BOOTSTRAP_TOKEN_HASH}'
)
ON CONFLICT (primary_ipv4) DO UPDATE
SET provider_id = EXCLUDED.provider_id,
    provider_server_id = EXCLUDED.provider_server_id,
    host_os_family = EXCLUDED.host_os_family,
    host_os_version = EXCLUDED.host_os_version,
    host_image_ref = EXCLUDED.host_image_ref,
    region = EXCLUDED.region,
    provider_location = EXCLUDED.provider_location,
    tier_target = EXCLUDED.tier_target,
    total_cpu_core = EXCLUDED.total_cpu_core,
    total_threads = 0,
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
Provisioned manual metal for $IP

Notes for scaled:
  bootstrap token written to $STATE_DIR/bootstrap_token on $SSH_TARGET
  identity file will be created at $STATE_DIR/identity.json after the first bootstrap handshake
  binary installed at $INSTALL_PATH
EOF

#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'Usage: %s [--update-yaak]\n' "$0" >&2
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

mode="print-token"
for arg in "$@"; do
  case "$arg" in
    --update-yaak)
      mode="update-yaak"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 1
      ;;
  esac
done

require_command curl
require_command python3

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
env_file="${ENV_FILE:-$repo_root/.env.local}"
yaak_env_file="${YAAK_ENV_FILE:-$repo_root/docs/api/yaak.ev_hTkEdmm6xq.yaml}"

if [[ ! -f "$env_file" ]]; then
  printf 'Missing env file: %s\n' "$env_file" >&2
  exit 1
fi

# shellcheck disable=SC1090
set -a
. "$env_file"
set +a

: "${BFF_JWT_SECRET:?BFF_JWT_SECRET missing in .env.local}"
: "${BFF_IDENTITY_KEY:?BFF_IDENTITY_KEY missing in .env.local}"
: "${INTERNAL_AUTH_SYNC_SECRET:?INTERNAL_AUTH_SYNC_SECRET missing in .env.local}"

BASE_URL="${BASE_URL:-127.0.0.1:8080}"
BFF_JWT_ISSUER="${BFF_JWT_ISSUER:-spacescale-web-bff}"
BFF_JWT_AUDIENCE="${BFF_JWT_AUDIENCE:-spacescale-api}"
BFF_JWT_TTL_SECONDS="${BFF_JWT_TTL_SECONDS:-3600}"
USER_EMAIL="${BFF_JWT_EMAIL:-}"
if [[ -z "$USER_EMAIL" && "$BFF_IDENTITY_KEY" == email:* ]]; then
  USER_EMAIL="${BFF_IDENTITY_KEY#email:}"
fi
USER_EMAIL="${USER_EMAIL:-dev@example.com}"
USER_NAME="${BFF_JWT_NAME:-Dev User}"
AVATAR_URL="${BFF_JWT_AVATAR_URL:-https://example.com/avatar.png}"
PRIMARY_REGION="${PRIMARY_REGION:-eu-central}"
APP_TIER="${APP_TIER:-starter}"

export BASE_URL BFF_JWT_ISSUER BFF_JWT_AUDIENCE BFF_JWT_TTL_SECONDS
export USER_EMAIL USER_NAME AVATAR_URL PRIMARY_REGION APP_TIER

token="$({
  BFF_JWT_SECRET="$BFF_JWT_SECRET" \
  BFF_IDENTITY_KEY="$BFF_IDENTITY_KEY" \
  BFF_JWT_ISSUER="$BFF_JWT_ISSUER" \
  BFF_JWT_AUDIENCE="$BFF_JWT_AUDIENCE" \
  BFF_JWT_TTL_SECONDS="$BFF_JWT_TTL_SECONDS" \
  USER_EMAIL="$USER_EMAIL" \
  USER_NAME="$USER_NAME" \
  AVATAR_URL="$AVATAR_URL" \
  python3 - <<'PY'
import base64
import hashlib
import hmac
import json
import os
import time


def b64(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode()


now = int(time.time())
ttl_seconds = int(os.environ["BFF_JWT_TTL_SECONDS"])
header = {"alg": "HS256", "typ": "JWT"}
payload = {
    "iss": os.environ["BFF_JWT_ISSUER"],
    "aud": os.environ["BFF_JWT_AUDIENCE"],
    "sub": "github:" + os.environ["BFF_IDENTITY_KEY"],
    "identity_key": os.environ["BFF_IDENTITY_KEY"],
    "exp": now + ttl_seconds,
    "iat": now - 60,
    "email": os.environ["USER_EMAIL"],
    "name": os.environ["USER_NAME"],
    "avatar_url": os.environ["AVATAR_URL"],
}
signing_input = (
    b64(json.dumps(header, separators=(",", ":")).encode())
    + "."
    + b64(json.dumps(payload, separators=(",", ":")).encode())
)
sig = b64(hmac.new(os.environ["BFF_JWT_SECRET"].encode(), signing_input.encode(), hashlib.sha256).digest())
print(signing_input + "." + sig)
PY
})"

if [[ "$mode" == "print-token" ]]; then
  printf '%s\n' "$token"
  exit 0
fi

api_base="http://$BASE_URL"
curl --fail-with-body --silent --show-error "$api_base/healthz" >/dev/null

sync_json="$(python3 - <<'PY' | curl --fail-with-body --silent --show-error -X POST "$api_base/v1/internal/auth-sync" -H "X-Internal-Auth: $INTERNAL_AUTH_SYNC_SECRET" -H "Content-Type: application/json" --data-binary @-
import json
import os

print(json.dumps({
    "identityKey": os.environ["BFF_IDENTITY_KEY"],
    "email": os.environ["USER_EMAIL"],
    "name": os.environ["USER_NAME"],
    "avatarUrl": os.environ["AVATAR_URL"],
}))
PY
)"

bootstrap_json="$(curl --fail-with-body --silent --show-error -X POST "$api_base/v1/bootstrap-defaults" -H "Authorization: Bearer $token" -H "Content-Type: application/json" --data '{}')"
workspaces_json="$(curl --fail-with-body --silent --show-error "$api_base/v1/workspaces" -H "Authorization: Bearer $token")"

workspace_id="$({
  BOOTSTRAP_JSON="$bootstrap_json" \
  WORKSPACES_JSON="$workspaces_json" \
  python3 - <<'PY'
import json
import os

bootstrap = json.loads(os.environ["BOOTSTRAP_JSON"])
if bootstrap.get("workspaceId"):
    print(bootstrap["workspaceId"])
    raise SystemExit(0)

workspaces = json.loads(os.environ["WORKSPACES_JSON"])
items = workspaces.get("workspaces") or []
if items:
    print(items[0]["id"])
PY
})"

if [[ -z "$workspace_id" ]]; then
  printf 'Unable to resolve workspace id from API responses\n' >&2
  exit 1
fi

projects_json="$(curl --fail-with-body --silent --show-error "$api_base/v1/workspaces/$workspace_id/projects" -H "Authorization: Bearer $token")"

project_id="$({
  BOOTSTRAP_JSON="$bootstrap_json" \
  PROJECTS_JSON="$projects_json" \
  python3 - <<'PY'
import json
import os

bootstrap = json.loads(os.environ["BOOTSTRAP_JSON"])
if bootstrap.get("projectId"):
    print(bootstrap["projectId"])
    raise SystemExit(0)

projects = json.loads(os.environ["PROJECTS_JSON"])
items = projects.get("projects") or []
if items:
    print(items[0]["id"])
PY
})"

if [[ -z "$project_id" ]]; then
  printf 'Unable to resolve project id from API responses\n' >&2
  exit 1
fi

BASE_URL="$BASE_URL" \
BFF_TOKEN="$token" \
INTERNAL_AUTH_SYNC_SECRET="$INTERNAL_AUTH_SYNC_SECRET" \
IDENTITY_KEY="$BFF_IDENTITY_KEY" \
USER_EMAIL="$USER_EMAIL" \
USER_NAME="$USER_NAME" \
AVATAR_URL="$AVATAR_URL" \
WORKSPACE_ID="$workspace_id" \
PROJECT_ID="$project_id" \
PRIMARY_REGION="$PRIMARY_REGION" \
APP_TIER="$APP_TIER" \
YAAK_ENV_FILE="$yaak_env_file" \
python3 - <<'PY'
from pathlib import Path
import os


def yaml_scalar(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


path = Path(os.environ["YAAK_ENV_FILE"])
lines = path.read_text().splitlines()
replacements = {
    "BASE_URL": os.environ["BASE_URL"],
    "BFF_TOKEN": os.environ["BFF_TOKEN"],
    "INTERNAL_AUTH_SYNC_SECRET": os.environ["INTERNAL_AUTH_SYNC_SECRET"],
    "IDENTITY_KEY": os.environ["IDENTITY_KEY"],
    "USER_EMAIL": os.environ["USER_EMAIL"],
    "USER_NAME": os.environ["USER_NAME"],
    "AVATAR_URL": os.environ["AVATAR_URL"],
    "WORKSPACE_ID": os.environ["WORKSPACE_ID"],
    "PROJECT_ID": os.environ["PROJECT_ID"],
    "PRIMARY_REGION": os.environ["PRIMARY_REGION"],
    "APP_TIER": os.environ["APP_TIER"],
}

current_name = None
seen = set()
for idx, line in enumerate(lines):
    stripped = line.strip()
    if stripped.startswith("name: "):
        current_name = stripped[len("name: "):]
        continue
    if current_name and stripped.startswith("value:"):
        if current_name in replacements:
            indent = line[: len(line) - len(line.lstrip())]
            lines[idx] = f"{indent}value: {yaml_scalar(replacements[current_name])}"
            seen.add(current_name)
        current_name = None

missing = sorted(set(replacements) - seen)
if missing:
    raise SystemExit(f"Missing Yaak variables in {path}: {', '.join(missing)}")

path.write_text("\n".join(lines) + "\n")
PY

printf 'Yaak LOCAL environment refreshed\n'
printf '  BASE_URL=%s\n' "$BASE_URL"
printf '  WORKSPACE_ID=%s\n' "$workspace_id"
printf '  PROJECT_ID=%s\n' "$project_id"
printf '  PRIMARY_REGION=%s\n' "$PRIMARY_REGION"
printf '  synced_user=%s\n' "$sync_json"

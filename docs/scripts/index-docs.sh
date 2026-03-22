#!/usr/bin/env bash
set -euo pipefail

DOCS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$DOCS_DIR/.env"
CONFIG_FILE="$DOCS_DIR/algolia-crawler-config.json"

# Load .env
if [[ ! -f "$ENV_FILE" ]]; then
  echo "error: $ENV_FILE not found — copy docs/.env.example to docs/.env and fill in the values" >&2
  exit 1
fi
set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

# Validate required vars
missing=()
[[ -z "${ALGOLIA_APP_ID:-}" ]]       && missing+=(ALGOLIA_APP_ID)
[[ -z "${ALGOLIA_WRITE_API_KEY:-}" ]] && missing+=(ALGOLIA_WRITE_API_KEY)

if [[ ${#missing[@]} -gt 0 ]]; then
  echo "error: missing required variables in .env: ${missing[*]}" >&2
  exit 1
fi

if ! command -v docker &>/dev/null; then
  echo "error: docker is not installed or not in PATH" >&2
  exit 1
fi

CONFIG="$(jq -r tostring "$CONFIG_FILE")"

echo "indexing docs → algolia index '$(jq -r .index_name "$CONFIG_FILE")' ..."
docker run --rm \
  -e APPLICATION_ID="$ALGOLIA_APP_ID" \
  -e API_KEY="$ALGOLIA_WRITE_API_KEY" \
  -e "CONFIG=$CONFIG" \
  algolia/docsearch-scraper
echo "done."

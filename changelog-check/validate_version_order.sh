#!/usr/bin/env bash
# Validates that versions and dates in CHANGELOG.md are in descending order.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")"; pwd)"
# shellcheck source=../actions_helpers.sh
source "$SCRIPT_DIR/../actions_helpers.sh"

changelog="${CHANGELOG_PATH:-CHANGELOG.md}"

if [ ! -r "$changelog" ]; then
  log_error "Changelog file '$changelog' does not exist or is not readable."
  exit 1
fi

# Check if semver v1 >= v2. Returns 0 (true) if v1 >= v2, 1 (false) otherwise
is_version_gte() {
  v1=$1
  v2=$2

  IFS='.' read -r major1 minor1 patch1 <<< "$v1"
  IFS='.' read -r major2 minor2 patch2 <<< "$v2"

  if [ "$major1" -gt "$major2" ]; then return 0; fi
  if [ "$major1" -lt "$major2" ]; then return 1; fi
  if [ "$minor1" -gt "$minor2" ]; then return 0; fi
  if [ "$minor1" -lt "$minor2" ]; then return 1; fi
  if [ "$patch1" -ge "$patch2" ]; then return 0; fi
  return 1
}

# Extract all version lines (e.g., "## [1.2.3] - 2024-01-15")
versions=$(grep --extended-regexp "^## \[[0-9]+\.[0-9]+\.[0-9]+\]" "$changelog" || true)

if [ -z "$versions" ]; then
  log_notice "No versions found, skipping order validation"
  exit 0
fi

# Extract version numbers and dates
version_numbers=$(echo "$versions" | sed -E 's/^## \[([0-9]+\.[0-9]+\.[0-9]+)\].*/\1/')
dates=$(echo "$versions" | sed -E 's/^## \[[^]]+\] - ([0-9]{4}-[0-9]{2}-[0-9]{2})/\1/')

# Check version ordering (should be descending)
prev_version=""
version_error=false
while IFS= read -r version; do
  if [ -n "$prev_version" ]; then
    if ! is_version_gte "$prev_version" "$version"; then
      log_error "Versions not in descending order: $prev_version should come before $version"
      version_error=true
    fi
  fi
  prev_version="$version"
done <<< "$version_numbers"

# Check date ordering (should be descending)
prev_date=""
date_error=false
while IFS= read -r date; do
  if [ -n "$prev_date" ]; then
    if [[ "$prev_date" < "$date" ]]; then
      log_error "Dates not in descending order: $prev_date should come before $date"
      date_error=true
    fi
  fi
  prev_date="$date"
done <<< "$dates"

if [ "$version_error" = true ] || [ "$date_error" = true ]; then
  echo ""
  log_error "CHANGELOG versions must be in descending order (newest first)"
  exit 1
fi

log_notice "Version and date ordering is correct"

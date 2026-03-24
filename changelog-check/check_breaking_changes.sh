#!/usr/bin/env bash
# Checks for breaking changes in CHANGELOG.md
# Detects breaking changes via:
#   - Entries prefixed with "Breaking Change:" in any section
#   - Presence of a "### Removed" section header
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")"; pwd)"
# shellcheck source=../actions_helpers.sh
source "$SCRIPT_DIR/../actions_helpers.sh"

check_mode="${CHECK_MODE:-unreleased}"
base_ref="${BASE_REF:-}"
changelog="${CHANGELOG_PATH:-CHANGELOG.md}"
unreleased_content="${UNRELEASED_CONTENT:-}"

if [ "$check_mode" = "diff" ] && [ -n "$base_ref" ]; then
  log_notice "Diff mode: checking changelog changes only"
  # Get diff, filter to additions, remove file header, strip leading +
  # || true prevents script failure when CHANGELOG is unchanged (grep exits 1 on no matches)
  text_to_check=$(git diff "origin/$base_ref" HEAD -- "$changelog" | grep "^+" | grep --invert-match "^+++" | sed 's/^+//' || true)
else
  log_notice "Checking entire Unreleased section"
  # Use the Unreleased section content from mindsers action
  text_to_check="$unreleased_content"
fi

# Check for breaking changes (entries prefixed with "Breaking Change:" or Removed section)
if echo "$text_to_check" | grep --quiet --extended-regexp "^[[:space:]]*-[[:space:]]*Breaking Change:"; then
  set_output has_breaking true
  log_warning "Breaking changes detected (breaking change entries found)"
elif echo "$text_to_check" | grep --quiet --extended-regexp "^### Removed"; then
  set_output has_breaking true
  log_warning "Breaking changes detected (Removed section found)"
else
  set_output has_breaking false
  log_notice "No breaking changes detected"
fi

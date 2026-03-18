#!/usr/bin/env bash
# Tests for calculate_next_version.sh
set -euo pipefail
trap 'echo "Error occurred at line $LINENO"; exit 1' ERR

cd "$(dirname "${BASH_SOURCE[0]}")"
source ../test_helpers.sh
source ../actions_helpers.sh

# Set up temporary directory for test outputs
TMPDIR_TEST=$(mktemp -d)
trap 'rm -rf "$TMPDIR_TEST"' EXIT

# Test helper: assert that calculate_next_version produces expected output
assert_next_version() {
  local current_version="$1"
  local bump_type="$2"
  local expected_next="$3"

  local out
  out="$(mktemp "${TMPDIR_TEST}/github_output.XXXXXX")"
  : >"$out"

  GITHUB_OUTPUT="$out" \
    CURRENT_VERSION="$current_version" \
    BUMP_TYPE="$bump_type" \
    ./calculate_next_version.sh

  grep --quiet --extended-regexp "^next_version=${expected_next}$" "$out"
}

# =============================================
# First release (no version -> 0.1.0) tests
# =============================================

expect_success "first release: no version -> 0.1.0" assert_next_version "" "initial" "0.1.0"

# Even if bump_type is "major", first release should be 0.1.0
expect_success "first release: ignores bump type" assert_next_version "" "major" "0.1.0"

# =============================================
# No bump (none) tests
# =============================================

expect_success "no bump: keeps version unchanged" assert_next_version "1.2.3" "" "1.2.3"

# =============================================
# Major bump tests
# =============================================

expect_success "major bump: 1.0.0 -> 2.0.0" assert_next_version "1.0.0" "major" "2.0.0"
expect_success "major bump: resets minor and patch to 0" assert_next_version "3.7.9" "major" "4.0.0"
expect_success "major bump: 0.5.2 -> 1.0.0" assert_next_version "0.5.2" "major" "1.0.0"
expect_success "major bump: handles multi-digit versions" assert_next_version "9.15.23" "major" "10.0.0"

# =============================================
# Minor bump tests
# =============================================

expect_success "minor bump: 1.2.3 -> 1.3.0" assert_next_version "1.2.3" "minor" "1.3.0"
expect_success "minor bump: resets patch to 0" assert_next_version "2.5.9" "minor" "2.6.0"
expect_success "minor bump: keeps major version" assert_next_version "5.0.0" "minor" "5.1.0"
expect_success "minor bump: handles multi-digit minor versions" assert_next_version "1.99.5" "minor" "1.100.0"

# =============================================
# Patch bump tests
# =============================================

expect_success "patch bump: 1.2.3 -> 1.2.4" assert_next_version "1.2.3" "patch" "1.2.4"
expect_success "patch bump: keeps major and minor" assert_next_version "3.7.0" "patch" "3.7.1"
expect_success "patch bump: handles multi-digit patch versions" assert_next_version "2.3.99" "patch" "2.3.100"

# =============================================
# Edge cases and defaults
# =============================================

expect_failure "unknown bump type: fails with error" "Unknown bump type" \
  env CURRENT_VERSION="1.2.3" BUMP_TYPE="unknown" ./calculate_next_version.sh

# No env vars set - should default to empty version and empty bump type, which keeps the empty version
expect_success "default values: empty version with empty bump type -> empty" assert_next_version "" "" ""

# =============================================
# Realistic version sequences
# =============================================

test_version_sequence_patch_to_minor() {
  GITHUB_OUTPUT="$TMPDIR_TEST/out17"
  touch "$GITHUB_OUTPUT"

  # Start at 1.0.0
  local v1
  v1=$(GITHUB_OUTPUT="$GITHUB_OUTPUT" CURRENT_VERSION="1.0.0" BUMP_TYPE="patch" "./calculate_next_version.sh" 2>&1 | grep "Next version:" | awk '{print $3}')
  [ "$v1" = "1.0.1" ]

  # Then minor bump
  GITHUB_OUTPUT="$TMPDIR_TEST/out17b"
  touch "$GITHUB_OUTPUT"
  local v2
  v2=$(GITHUB_OUTPUT="$GITHUB_OUTPUT" CURRENT_VERSION="1.0.1" BUMP_TYPE="minor" "./calculate_next_version.sh" 2>&1 | grep "Next version:" | awk '{print $3}')
  [ "$v2" = "1.1.0" ]
}
expect_success "version sequence: patch then minor" test_version_sequence_patch_to_minor

test_version_sequence_minor_to_major() {
  GITHUB_OUTPUT="$TMPDIR_TEST/out18"
  touch "$GITHUB_OUTPUT"

  # Start at 2.5.0
  local v1
  v1=$(GITHUB_OUTPUT="$GITHUB_OUTPUT" CURRENT_VERSION="2.5.0" BUMP_TYPE="minor" "./calculate_next_version.sh" 2>&1 | grep "Next version:" | awk '{print $3}')
  [ "$v1" = "2.6.0" ]

  # Then major bump
  GITHUB_OUTPUT="$TMPDIR_TEST/out18b"
  touch "$GITHUB_OUTPUT"
  local v2
  v2=$(GITHUB_OUTPUT="$GITHUB_OUTPUT" CURRENT_VERSION="2.6.0" BUMP_TYPE="major" "./calculate_next_version.sh" 2>&1 | grep "Next version:" | awk '{print $3}')
  [ "$v2" = "3.0.0" ]
}
expect_success "version sequence: minor then major" test_version_sequence_minor_to_major

print_results

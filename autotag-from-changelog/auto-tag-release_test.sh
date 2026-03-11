#!/usr/bin/env bash
# Tests for auto-tag-release.sh
set -euo pipefail
trap 'echo "Error occurred at line $LINENO"; exit 1' ERR

SCRIPT_DIR="$(cd "$(dirname "$0")"; pwd)"
PASS=0
FAIL=0

run_test() {
  local name="$1"
  local expected_exit="$2"
  local expected_output="$3"
  shift 3

  local output exit_code
  output=$("$@" 2>&1) && exit_code=0 || exit_code=$?

  if [ "$exit_code" -ne "$expected_exit" ]; then
    echo "FAIL: $name — expected exit $expected_exit, got $exit_code"
    echo "  output: $output"
    FAIL=$((FAIL + 1))
    return
  fi

  if [ -n "$expected_output" ] && ! echo "$output" | grep -qF "$expected_output"; then
    echo "FAIL: $name — expected output containing: $expected_output"
    echo "  actual: $output"
    FAIL=$((FAIL + 1))
    return
  fi

  echo "PASS: $name"
  PASS=$((PASS + 1))
}

# Set up a temporary bare repo to act as "origin" and a working clone.
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

git config user.email >/dev/null 2>&1 || git config --global user.email "test@test.com"
git config user.name >/dev/null 2>&1 || git config --global user.name "test"
git init --bare --initial-branch=main "$TMPDIR/origin.git" >/dev/null
git clone "$TMPDIR/origin.git" "$TMPDIR/work" >/dev/null
cd "$TMPDIR/work"
git commit --allow-empty -m "initial" >/dev/null
git push origin main >/dev/null

# =============================================
# parse_changelog unit tests
# =============================================

# Source just the function from the script. We use a subshell trick:
# the script exits early because $changelog isn't set, so we extract
# the function definition directly.
eval "$(sed -n '/^parse_changelog()/,/^}/p' "$SCRIPT_DIR/auto-tag-release.sh")"

run_parse_test() {
  local name="$1"
  local expected_version="$2"
  local expected_has_unreleased="$3"  # "true" or "false"
  local changelog_file="$4"

  parse_changelog "$changelog_file"

  if [ "$version" != "$expected_version" ]; then
    echo "FAIL: $name — expected version '$expected_version', got '$version'"
    FAIL=$((FAIL + 1))
    return
  fi

  local has_unreleased="false"
  if [ -n "$unreleased_content" ]; then
    has_unreleased="true"
  fi

  if [ "$has_unreleased" != "$expected_has_unreleased" ]; then
    echo "FAIL: $name — expected unreleased_content=$expected_has_unreleased, got $has_unreleased"
    FAIL=$((FAIL + 1))
    return
  fi

  echo "PASS: $name"
  PASS=$((PASS + 1))
}

# parse_changelog: standard changelog with unreleased content
cat <<'EOF' > CHANGELOG.md
## [Unreleased]

### Added

- A new feature

## [1.2.3] - 2026-01-01
EOF
run_parse_test "parse: unreleased content with version" "1.2.3" "true" CHANGELOG.md

# parse_changelog: empty unreleased section
cat <<'EOF' > CHANGELOG.md
## [Unreleased]

## [3.0.0] - 2026-01-01
EOF
run_parse_test "parse: empty unreleased section" "3.0.0" "false" CHANGELOG.md

# parse_changelog: no version after unreleased
cat <<'EOF' > CHANGELOG.md
## [Unreleased]
EOF
run_parse_test "parse: no version after unreleased" "" "false" CHANGELOG.md

# parse_changelog: no unreleased section at all
cat <<'EOF' > CHANGELOG.md
## [2.0.0] - 2026-01-01
EOF
run_parse_test "parse: no unreleased section" "" "false" CHANGELOG.md

# parse_changelog: multiple versions, picks first
cat <<'EOF' > CHANGELOG.md
## [Unreleased]

## [2.1.0] - 2026-03-01

## [2.0.0] - 2026-01-01
EOF
run_parse_test "parse: multiple versions picks first" "2.1.0" "false" CHANGELOG.md

# parse_changelog: only blank lines under unreleased
cat <<'EOF' > CHANGELOG.md
## [Unreleased]



## [1.0.0] - 2026-01-01
EOF
run_parse_test "parse: only blank lines under unreleased" "1.0.0" "false" CHANGELOG.md

# parse_changelog: content before unreleased is ignored
cat <<'EOF' > CHANGELOG.md
# Changelog

Some preamble text.

## [Unreleased]

### Fixed

- A bug fix

## [0.9.0] - 2025-12-01
EOF
run_parse_test "parse: content before unreleased is ignored" "0.9.0" "true" CHANGELOG.md

# =============================================
# Integration tests
# =============================================

# --- Content under [Unreleased], previous version not tagged — should fail ---
cat <<'EOF' > CHANGELOG.md
## [Unreleased]

### Added

- A new feature

## [1.0.0] - 2026-01-01
EOF

run_test "content under [Unreleased], untagged version" 1 "is not tagged" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/auto-tag-release.sh"

# --- Empty [Unreleased], new version, no existing tag — should tag ---
cat <<'EOF' > CHANGELOG.md
## [Unreleased]

## [1.0.0] - 2026-01-01
EOF

run_test "creates new tag" 0 "Tagged v1.0.0 successfully" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/auto-tag-release.sh"

# Verify the tag was actually created
if git rev-parse v1.0.0 >/dev/null 2>&1; then
  echo "PASS: tag v1.0.0 exists"
  PASS=$((PASS + 1))
else
  echo "FAIL: tag v1.0.0 was not created"
  FAIL=$((FAIL + 1))
fi

# --- Tag already exists — should skip ---
run_test "tag already exists" 0 "already exists" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/auto-tag-release.sh"

# --- Content under [Unreleased], previous version already tagged — should pass ---
cat <<'EOF' > CHANGELOG.md
## [Unreleased]

### Added

- A new feature

## [1.0.0] - 2026-01-01
EOF

run_test "content under [Unreleased], tagged version" 0 "already tagged" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/auto-tag-release.sh"

# --- No version sections at all — should skip ---
cat <<'EOF' > CHANGELOG.md
## [Unreleased]
EOF

run_test "no released version" 0 "No released version" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/auto-tag-release.sh"

# --- No [Unreleased] section — should skip ---
cat <<'EOF' > CHANGELOG.md
## [2.0.0] - 2026-01-01
EOF

run_test "no [Unreleased] section" 0 "No released version" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/auto-tag-release.sh"

# --- [Unreleased] with only blank lines (no content) — should tag ---
cat <<'EOF' > CHANGELOG.md
## [Unreleased]



## [2.0.0] - 2026-01-01
EOF

run_test "blank lines under [Unreleased]" 0 "Tagged v2.0.0 successfully" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/auto-tag-release.sh"

# --- Summary ---
echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1

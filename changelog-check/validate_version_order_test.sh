#!/usr/bin/env bash
# Tests for validate_version_order.sh
set -euo pipefail
trap 'echo "Error occurred at line $LINENO"; exit 1' ERR

cd "$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$PWD"
source ../test_helpers.sh

# Set up temporary directory for test files
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT
cd "$TMPDIR"

# =============================================
# is_version_gte unit tests
# =============================================

# Extract the is_version_gte function for unit testing
eval "$(sed -n '/^is_version_gte()/,/^}/p' "$SCRIPT_DIR/validate_version_order.sh")"

test_version_gte_equal() {
  is_version_gte "1.2.3" "1.2.3"
}
expect_success "is_version_gte: equal versions" test_version_gte_equal

test_version_gte_major_greater() {
  is_version_gte "2.0.0" "1.9.9"
}
expect_success "is_version_gte: major version greater" test_version_gte_major_greater

test_version_gte_major_less() {
  ! is_version_gte "1.9.9" "2.0.0"
}
expect_success "is_version_gte: major version less" test_version_gte_major_less

test_version_gte_minor_greater() {
  is_version_gte "1.3.0" "1.2.9"
}
expect_success "is_version_gte: minor version greater" test_version_gte_minor_greater

test_version_gte_minor_less() {
  ! is_version_gte "1.2.9" "1.3.0"
}
expect_success "is_version_gte: minor version less" test_version_gte_minor_less

test_version_gte_patch_greater() {
  is_version_gte "1.2.4" "1.2.3"
}
expect_success "is_version_gte: patch version greater" test_version_gte_patch_greater

test_version_gte_patch_less() {
  ! is_version_gte "1.2.3" "1.2.4"
}
expect_success "is_version_gte: patch version less" test_version_gte_patch_less

test_version_gte_multi_digit() {
  is_version_gte "1.10.0" "1.9.0"
}
expect_success "is_version_gte: handles multi-digit versions" test_version_gte_multi_digit

# =============================================
# Integration tests
# =============================================

# --- Valid descending version order ---
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

## [1.1.0] - 2026-03-15

### Added
- New feature

## [1.0.0] - 2026-03-01

### Changed
- Breaking Change: Changed authentication API

## [0.2.0] - 2026-02-15

### Added
- New feature

## [0.1.0] - 2026-01-01

### Added
- Initial release
EOF

expect_success "valid descending version order" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/validate_version_order.sh"

# --- Valid descending dates ---
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

## [0.3.0] - 2026-03-15

### Added
- New feature

## [0.2.0] - 2026-03-10

### Added
- New feature

## [0.1.0] - 2026-03-01

### Added
- Initial release
EOF

expect_success "valid descending dates" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/validate_version_order.sh"

# --- Single version (should pass) ---
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

## [0.1.0] - 2026-01-01

### Added
- Initial release
EOF

expect_success "single version passes" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/validate_version_order.sh"

# --- No versions (should skip validation) ---
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- New feature
EOF

expect_success "no versions skips validation" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/validate_version_order.sh"

# --- Invalid version order (ascending instead of descending) ---
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

## [0.1.1] - 2026-01-01

### Added
- New feature

## [0.2.0] - 2026-03-01

### Added
- New feature
EOF

expect_failure "invalid ascending version order" "not in descending order" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/validate_version_order.sh"

# --- Invalid date order (ascending instead of descending) ---
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

## [0.2.0] - 2026-01-01

### Added
- New feature

## [0.1.0] - 2026-03-01

### Added
- Initial release
EOF

expect_failure "invalid ascending date order" "not in descending order" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/validate_version_order.sh"

# --- Mixed ordering issues ---
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

## [1.0.0] - 2026-03-01

### Changed
- Breaking Change: Major API redesign

## [0.5.0] - 2026-03-15

### Added
- New feature

## [0.1.0] - 2026-03-10

### Added
- Initial release
EOF

expect_failure "mixed ordering issues" "not in descending order" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/validate_version_order.sh"

# --- Missing changelog file ---
expect_failure "missing changelog file" "does not exist" \
  env CHANGELOG_PATH=nonexistent.md "$SCRIPT_DIR/validate_version_order.sh"

# --- Out of order with correct dates ---
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

## [0.5.0] - 2026-03-15

### Added
- New feature

## [1.0.0] - 2026-03-10

### Changed
- Breaking Change: Major API redesign

## [0.1.0] - 2026-03-01

### Added
- Initial release
EOF

expect_failure "versions out of order despite correct dates" "not in descending order" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/validate_version_order.sh"

# --- Versions with different date formats (still valid) ---
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

## [2.0.0] - 2026-12-31

### Changed
- Breaking Change: Major refactor

## [1.0.0] - 2026-01-01

### Changed
- Breaking Change: Stable release

## [0.1.0] - 2025-12-31

### Added
- Initial release
EOF

expect_success "versions across year boundary" \
  env CHANGELOG_PATH=CHANGELOG.md "$SCRIPT_DIR/validate_version_order.sh"

print_results

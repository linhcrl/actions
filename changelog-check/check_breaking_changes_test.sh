#!/usr/bin/env bash
# Tests for check_breaking_changes.sh
set -euo pipefail
trap 'echo "Error occurred at line $LINENO"; exit 1' ERR

cd "$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$PWD"
source ../test_helpers.sh

# Set up temporary directory for test files
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT
cd "$TMPDIR"

# Mock GITHUB_OUTPUT
export GITHUB_OUTPUT="$TMPDIR/github_output.txt"

# Helper to reset GITHUB_OUTPUT
reset_output() {
  : > "$GITHUB_OUTPUT"
}

# Helper to check output
check_has_breaking() {
  expected=$1
  actual=$(grep "has_breaking=" "$GITHUB_OUTPUT" | cut -d= -f2)
  if [ "$actual" != "$expected" ]; then
    echo "Expected has_breaking=$expected, got has_breaking=$actual"
    return 1
  fi
}

# =============================================
# Unreleased mode tests
# =============================================

# --- No breaking changes in unreleased content ---
reset_output
test_no_breaking_unreleased() {
  env CHECK_MODE=unreleased \
      UNRELEASED_CONTENT="- Added new feature
- Fixed bug in login" \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "false"
}
expect_success "unreleased mode: no breaking changes" test_no_breaking_unreleased

# --- Breaking change in unreleased content ---
reset_output
test_breaking_unreleased() {
  env CHECK_MODE=unreleased \
      UNRELEASED_CONTENT="- Breaking Change: Removed legacy API
- Added new feature" \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "true"
}
expect_success "unreleased mode: has breaking changes" test_breaking_unreleased

# --- Breaking change with leading spaces ---
reset_output
test_breaking_with_spaces() {
  env CHECK_MODE=unreleased \
      UNRELEASED_CONTENT="  - Breaking Change: Updated authentication
- Fixed bug" \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "true"
}
expect_success "unreleased mode: breaking change with spaces" test_breaking_with_spaces

# --- Empty unreleased content ---
reset_output
test_empty_unreleased() {
  env CHECK_MODE=unreleased \
      UNRELEASED_CONTENT="" \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "false"
}
expect_success "unreleased mode: empty content" test_empty_unreleased

# --- Removed section triggers breaking change ---
reset_output
test_removed_section() {
  env CHECK_MODE=unreleased \
      UNRELEASED_CONTENT="### Removed

- Deprecated API endpoints
- Old authentication method" \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "true"
}
expect_success "unreleased mode: Removed section triggers breaking" test_removed_section

# --- Removed section with other sections ---
reset_output
test_removed_with_other_sections() {
  env CHECK_MODE=unreleased \
      UNRELEASED_CONTENT="### Added

- New feature

### Removed

- Legacy API" \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "true"
}
expect_success "unreleased mode: Removed section with other sections" test_removed_with_other_sections

# =============================================
# Diff mode tests
# =============================================

# Initialize git repo for diff tests
git init > /dev/null 2>&1
git config user.email "test@example.com"
git config user.name "Test User"

# Create initial CHANGELOG on main branch
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Initial commit" > /dev/null 2>&1
git branch -M main

# Set up origin remote and fetch to create tracking branch (simulates production)
git remote add origin . > /dev/null 2>&1
git fetch origin > /dev/null 2>&1

# Create a feature branch
git checkout -b feature-branch > /dev/null 2>&1

# --- Diff mode: Add non-breaking change ---
# This creates a diff with only + lines for non-breaking content
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature
- New feature without breaking changes

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Add non-breaking feature" > /dev/null 2>&1

# Verify what the diff looks like (for test clarity)
# git diff main HEAD -- CHANGELOG.md should show:
# +- New feature without breaking changes

reset_output
test_diff_no_breaking() {
  env CHECK_MODE=diff \
      BASE_REF=main \
      CHANGELOG_PATH=CHANGELOG.md \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "false"
}
expect_success "diff mode: no breaking changes in additions" test_diff_no_breaking

# --- Diff mode: Add breaking change ---
# This creates a diff with + lines including breaking change
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature
- New feature without breaking changes

### Changed
- Breaking Change: Removed deprecated API endpoints

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Add breaking change" > /dev/null 2>&1

# Verify what the diff looks like (for test clarity)
# git diff main HEAD -- CHANGELOG.md should show:
# +- New feature without breaking changes
# +
# +### Changed
# +- Breaking Change: Removed deprecated API endpoints

reset_output
test_diff_has_breaking() {
  env CHECK_MODE=diff \
      BASE_REF=main \
      CHANGELOG_PATH=CHANGELOG.md \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "true"
}
expect_success "diff mode: breaking change in additions" test_diff_has_breaking

# --- Diff mode: Breaking change exists in base, not in PR diff ---
# Reset to a state where main has a breaking change
git checkout main > /dev/null 2>&1
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature

### Changed
- Breaking Change: This breaking change is already in main

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Add breaking change to main" > /dev/null 2>&1
git fetch origin > /dev/null 2>&1

# Create new feature branch from updated main
git checkout -b feature-no-breaking > /dev/null 2>&1

# Add only non-breaking content
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature
- PR adds this non-breaking feature

### Changed
- Breaking Change: This breaking change is already in main

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Add non-breaking feature to PR" > /dev/null 2>&1

# The diff from main should only show:
# +- PR adds this non-breaking feature
# The "Breaking Change" line is in both main and feature branch, so not in diff

reset_output
test_diff_breaking_in_base_not_in_pr() {
  env CHECK_MODE=diff \
      BASE_REF=main \
      CHANGELOG_PATH=CHANGELOG.md \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "false"
}
expect_success "diff mode: breaking change in base but not in PR additions" test_diff_breaking_in_base_not_in_pr

# --- Diff mode: Removal and addition (complex diff) ---
git checkout main > /dev/null 2>&1
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature
- Feature that will be removed

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Update main with removable feature" > /dev/null 2>&1

git checkout -b feature-replace > /dev/null 2>&1

# Remove one line and add another with breaking change
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature
- Breaking Change: Replaced old feature with breaking API

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Replace feature with breaking change" > /dev/null 2>&1

# Diff should show:
# -- Feature that will be removed
# +- Breaking Change: Replaced old feature with breaking API
# Script should detect the breaking change in the + line

reset_output
test_diff_with_removal_and_breaking_addition() {
  env CHECK_MODE=diff \
      BASE_REF=main \
      CHANGELOG_PATH=CHANGELOG.md \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "true"
}
expect_success "diff mode: removal and breaking addition" test_diff_with_removal_and_breaking_addition

# --- Diff mode: Breaking change is removed ---
git checkout main > /dev/null 2>&1
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature

### Changed
- Breaking Change: This breaking change will be removed in the PR

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Add breaking change to main that will be removed" > /dev/null 2>&1

git checkout -b feature-remove-breaking > /dev/null 2>&1

# Remove the breaking change entry
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature
- Non-breaking feature added

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Remove breaking change and add non-breaking feature" > /dev/null 2>&1

# Diff should show:
# +- Non-breaking feature added
# +
# -### Changed
# -- Breaking Change: This breaking change will be removed in the PR
# Script should NOT detect breaking change since it's in removal (- lines)

reset_output
test_diff_breaking_removed() {
  env CHECK_MODE=diff \
      BASE_REF=main \
      CHANGELOG_PATH=CHANGELOG.md \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "false"
}
expect_success "diff mode: breaking change removed in PR" test_diff_breaking_removed

# --- Diff mode: Removed section added in PR ---
git checkout main > /dev/null 2>&1
cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Main without Removed section" > /dev/null 2>&1

git checkout -b feature-add-removed > /dev/null 2>&1

cat <<'EOF' > CHANGELOG.md
# Changelog

## [Unreleased]

### Added
- Initial feature

### Removed
- Deprecated API endpoints

## [1.0.0] - 2026-01-01

### Added
- First release
EOF

git add CHANGELOG.md
git commit -m "Add Removed section" > /dev/null 2>&1

# Diff should show the new Removed section
reset_output
test_diff_removed_section() {
  env CHECK_MODE=diff \
      BASE_REF=main \
      CHANGELOG_PATH=CHANGELOG.md \
      "$SCRIPT_DIR/check_breaking_changes.sh"
  check_has_breaking "true"
}
expect_success "diff mode: Removed section added in PR" test_diff_removed_section

echo ""
echo "All tests passed!"

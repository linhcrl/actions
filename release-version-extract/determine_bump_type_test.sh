#!/usr/bin/env bash
# Tests for determine_bump_type.sh
set -euo pipefail
trap 'echo "Error occurred at line $LINENO"; exit 1' ERR

cd "$(dirname "${BASH_SOURCE[0]}")"
source ../test_helpers.sh
source ../actions_helpers.sh

# Set up temporary directory for test outputs
TMPDIR_TEST=$(mktemp -d)
trap 'rm -rf "$TMPDIR_TEST"' EXIT

# Test helper: assert that determine_bump_type produces expected output
assert_bump_type() {
  local current_version="$1"
  local unreleased_changes="$2"
  local expected_bump="$3"

  local out
  out="$(mktemp "${TMPDIR_TEST}/github_output.XXXXXX")"
  : >"$out"

  GITHUB_OUTPUT="$out" \
    CURRENT_VERSION="$current_version" \
    UNRELEASED_CHANGES="$unreleased_changes" \
    ./determine_bump_type.sh

  grep --quiet --extended-regexp "^bump_type=${expected_bump}$" "$out"
}

# =============================================
# First release (no version) tests
# =============================================

test_first_release() {
  local changes
  changes="### Added

- Initial feature"

  assert_bump_type "" "$changes" "initial"
}
expect_success "first release: outputs initial bump type" test_first_release

# =============================================
# No changes tests
# =============================================

expect_success "no changes: empty unreleased section" \
  assert_bump_type "1.2.3" "" ""

expect_success "no changes: no version and no unreleased changes" \
  assert_bump_type "" "" ""

# =============================================
# Breaking changes (major bump) tests
# =============================================

test_breaking_change_major() {
  local changes
  changes="### Changed

- Breaking Change: Removed deprecated API
- Updated documentation"

  assert_bump_type "1.2.3" "$changes" "major"
}
expect_success "breaking change: triggers major bump" test_breaking_change_major

test_breaking_change_with_whitespace() {
  local changes
  changes="### Fixed

  - Breaking Change: Changed API signature"

  assert_bump_type "2.0.0" "$changes" "major"
}
expect_success "breaking change: handles leading whitespace" test_breaking_change_with_whitespace

test_removed_section_major() {
  local changes
  changes="### Removed

- Deprecated API endpoints"

  assert_bump_type "1.5.0" "$changes" "major"
}
expect_success "Removed section: triggers major bump" test_removed_section_major

# =============================================
# Minor bump tests (Added, Changed, Deprecated)
# =============================================

test_added_section_minor() {
  local changes
  changes="### Added

- New authentication feature
- Added logging support"

  assert_bump_type "1.2.3" "$changes" "minor"
}
expect_success "Added section: triggers minor bump" test_added_section_minor

test_changed_section_minor() {
  local changes
  changes="### Changed

- Improved performance of API calls
- Updated dependencies"

  assert_bump_type "1.0.0" "$changes" "minor"
}
expect_success "Changed section: triggers minor bump" test_changed_section_minor

test_deprecated_section_minor() {
  local changes
  changes="### Deprecated

- Old API will be removed in v3.0.0"

  assert_bump_type "2.5.1" "$changes" "minor"
}
expect_success "Deprecated section: triggers minor bump" test_deprecated_section_minor

# =============================================
# Patch bump tests (Fixed, Security)
# =============================================

test_fixed_section_patch() {
  local changes
  changes="### Fixed

- Fixed memory leak
- Corrected typo in error message"

  assert_bump_type "1.2.3" "$changes" "patch"
}
expect_success "Fixed section: triggers patch bump" test_fixed_section_patch

test_security_section_patch() {
  local changes
  changes="### Security

- Patched SQL injection vulnerability"

  assert_bump_type "1.0.0" "$changes" "patch"
}
expect_success "Security section: triggers patch bump" test_security_section_patch

# =============================================
# Priority tests (major takes precedence)
# =============================================

test_breaking_takes_precedence_over_added() {
  local changes
  changes="### Added

- New feature

### Changed

- Breaking Change: Removed old API"

  assert_bump_type "1.5.0" "$changes" "major"
}
expect_success "priority: breaking change takes precedence over Added" test_breaking_takes_precedence_over_added

test_removed_takes_precedence_over_added() {
  local changes
  changes="### Added

- New feature

### Removed

- Old deprecated API"

  assert_bump_type "1.5.0" "$changes" "major"
}
expect_success "priority: Removed takes precedence over Added" test_removed_takes_precedence_over_added

test_added_takes_precedence_over_fixed() {
  local changes
  changes="### Added

- New feature

### Fixed

- Bug fix"

  assert_bump_type "1.0.0" "$changes" "minor"
}
expect_success "priority: Added takes precedence over Fixed" test_added_takes_precedence_over_fixed

# =============================================
# Error cases
# =============================================

expect_failure "unrecognized category: fails with error" "no recognized categories" \
  env CURRENT_VERSION="1.0.0" UNRELEASED_CHANGES="### CustomCategory" "./determine_bump_type.sh"

expect_failure "changes without category header: fails" "no recognized categories" \
  env CURRENT_VERSION="1.0.0" UNRELEASED_CHANGES="- Some random change
- Another change" "./determine_bump_type.sh"

# =============================================
# Multiple sections tests
# =============================================

test_multiple_sections_all_types() {
  local changes
  changes="### Added

- New feature

### Fixed

- Bug fix

### Security

- Security patch"

  # Added should take precedence (minor over patch)
  assert_bump_type "1.0.0" "$changes" "minor"
}
expect_success "multiple sections: Added takes precedence" test_multiple_sections_all_types

test_multiple_sections_with_breaking() {
  local changes
  changes="### Added

- New feature

### Changed

- Breaking Change: Removed deprecated API
- Improved performance

### Fixed

- Bug fix

### Security

- Security patch"

  # Breaking change should take precedence over all others
  assert_bump_type "2.0.0" "$changes" "major"
}
expect_success "multiple sections: Breaking change takes precedence over all" test_multiple_sections_with_breaking

print_results

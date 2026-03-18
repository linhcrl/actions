#!/usr/bin/env bash
# Determines the version bump type based on unreleased changes in CHANGELOG.md.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"/..
source ./actions_helpers.sh

current_version="${CURRENT_VERSION:-}"
unreleased_changes="${UNRELEASED_CHANGES:-}"

# No unreleased changes - no bump needed
if [ -z "$unreleased_changes" ]; then
  set_output "bump_type" ""
  log_notice "No unreleased changes"
  exit 0
fi

# First release - no bump type analysis needed
if [ -z "$current_version" ]; then
  set_output "bump_type" "initial"
  log_notice "First release"
  exit 0
fi

# Check for breaking changes (major bump)
if echo "$unreleased_changes" | grep --quiet --extended-regexp "^[[:space:]]*-[[:space:]]*Breaking Change:"; then
  set_output "bump_type" "major"
  log_warning "Major version bump required (breaking changes detected)"
# Check for Removed section (major bump)
elif echo "$unreleased_changes" | grep --quiet --extended-regexp "^### Removed"; then
  set_output "bump_type" "major"
  log_warning "Major version bump required (removed features detected)"
# Check for Added, Changed, or Deprecated (minor bump)
elif echo "$unreleased_changes" | grep --quiet --extended-regexp "^### (Added|Changed|Deprecated)"; then
  set_output "bump_type" "minor"
  log_notice "Minor version bump required (new features, changes, or deprecations)"
# Check for Fixed or Security (patch bump)
elif echo "$unreleased_changes" | grep --quiet --extended-regexp "^### (Fixed|Security)"; then
  set_output "bump_type" "patch"
  log_notice "Patch version bump required (fixes or security updates)"
else
  # Changes exist but no recognized category - fail
  error_message=$(cat <<'ERRMSG'
Unreleased section has changes but no recognized categories

Please use one of the following categories:
  - ### Added (for new features)
  - ### Changed (for changes in existing functionality)
  - ### Deprecated (for soon-to-be removed features)
  - ### Removed (for removed features)
  - ### Fixed (for bug fixes)
  - ### Security (for security fixes)

Note: Prefix breaking change items with 'Breaking Change:' for major version bumps
Note: ### Removed section automatically triggers a major version bump
ERRMSG
)
  log_error "$error_message"
  exit 1
fi

#!/usr/bin/env bash
# Calculates the next version based on current version and bump type.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"/..
source ./actions_helpers.sh

current_version="${CURRENT_VERSION:-}"
bump_type="${BUMP_TYPE:-}"

# No changes - keep current version
if [ -z "$bump_type" ]; then
  next_version="$current_version"
# First release is always 0.1.0 (initial development version)
elif [ -z "$current_version" ]; then
  next_version="0.1.0"
else
  # Split version into major.minor.patch
  IFS='.' read -r major minor patch <<< "$current_version"

  # Calculate next version based on bump type
  case $bump_type in
    major)
      next_version="$((major + 1)).0.0"
      ;;
    minor)
      next_version="$major.$((minor + 1)).0"
      ;;
    patch)
      next_version="$major.$minor.$((patch + 1))"
      ;;
    *)
      error_message=$(cat <<ERRMSG
Unknown bump type: $bump_type

Valid bump types: major, minor, patch
ERRMSG
)
      log_error "$error_message"
      exit 1
      ;;
  esac
fi

set_output "next_version" "$next_version"
if [ -z "$current_version" ]; then
  log_notice "Next version: $next_version (initial release)"
else
  log_notice "Next version: $next_version (from $current_version)"
fi

#!/usr/bin/env bash
set -euo pipefail

# Commits release changes including CHANGELOG.md and specified files.
#
# Usage: commit-release-changes.sh
#
# Environment variables:
#   VERSION            - Version being released (e.g., "1.2.3")
#   FILES_TO_COMMIT    - Newline-separated list of files to commit (in addition to CHANGELOG.md)

# Save current directory (where files to commit are expected)
WORK_DIR="$(pwd)"

# Change to script directory for relative sourcing
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

source ../../actions_helpers.sh

# Go back to work directory where files are
cd "$WORK_DIR"

version="$VERSION"
files_to_commit="${FILES_TO_COMMIT:-}"

# Stage CHANGELOG.md (always updated by the workflow)
git add CHANGELOG.md

# Stage additional files from newline-separated list
if [ -n "$files_to_commit" ]; then
  while IFS= read -r file; do
    # Trim whitespace using bash parameter expansion (avoids command substitution
    # with user input, safer than sed/xargs for potentially malicious filenames)
    file="${file#"${file%%[![:space:]]*}"}"
    file="${file%"${file##*[![:space:]]}"}"
    if [ -n "$file" ]; then
      if [ ! -f "$file" ]; then
        log_warning "File not found, skipping: $file"
        continue
      fi
      git add "$file"
    fi
  done <<< "$files_to_commit"
fi

# Check for leftover unstaged or untracked files
unstaged=$(git diff --name-only)
untracked=$(git ls-files --others --exclude-standard)

# Combine unstaged and untracked files (add newline separator if both exist)
leftover=""
if [ -n "$unstaged" ] && [ -n "$untracked" ]; then
  leftover="${unstaged}"$'\n'"${untracked}"
elif [ -n "$unstaged" ]; then
  leftover="$unstaged"
elif [ -n "$untracked" ]; then
  leftover="$untracked"
fi

if [ -n "$leftover" ]; then
  log_error "Leftover files detected. These files were modified or created but not included in files_to_commit:"
  echo "$leftover" >&2
  exit 1
fi

git commit -m "Prepare v${version} release" \
  -m "" \
  -m "Update version to ${version} in repository files."

log_notice "Changes committed successfully"

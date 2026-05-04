#!/usr/bin/env bash
# Tests for commit-release-changes.sh
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
source ../../test_helpers.sh

SCRIPT_DIR="$(pwd)"
TMPDIR_TEST=$(mktemp -d)
trap 'rm -rf "$TMPDIR_TEST"' EXIT

# Helper: set up a git repo with an initial commit and modified CHANGELOG.md
# Creates optional additional untracked files for testing
# Usage: setup_repo "$repo_path" [file1 file2 ...]
setup_repo() {
  local repo="$1"
  shift  # Remove repo from arguments, rest are files to create

  mkdir -p "$repo"
  cd "$repo"

  git init
  git config user.name "Test"
  git config user.email "test@test.com"

  # Create initial commit
  echo "# Changelog" > CHANGELOG.md
  git add CHANGELOG.md
  git commit -m "Initial commit"

  # Modify CHANGELOG
  echo "## [Unreleased]" >> CHANGELOG.md

  # Create any additional files passed as arguments
  for file in "$@"; do
    echo "content" > "$file"
  done
}

test_commits_specified_file() {
  local repo="$TMPDIR_TEST/test1"
  setup_repo "$repo" file1.txt

  export FILES_TO_COMMIT="file1.txt"
  export VERSION="1.0.0"
  bash "$SCRIPT_DIR/commit_release_changes.sh"

  expect_files_in_commit "CHANGELOG.md" "file1.txt"
}
expect_success "commits specified file plus CHANGELOG.md" test_commits_specified_file

test_multiple_files_newline_separated() {
  local repo="$TMPDIR_TEST/test2"
  setup_repo "$repo" file1.txt file2.txt

  export FILES_TO_COMMIT="file1.txt
file2.txt"
  export VERSION="1.0.0"
  bash "$SCRIPT_DIR/commit_release_changes.sh"

  expect_files_in_commit "CHANGELOG.md" "file1.txt" "file2.txt"
}
expect_success "commits multiple newline-separated files" test_multiple_files_newline_separated

test_handles_whitespace_in_list() {
  local repo="$TMPDIR_TEST/test3"
  setup_repo "$repo" file1.txt file2.txt credentials.json

  export FILES_TO_COMMIT="  file1.txt
  file2.txt
  credentials.json  "
  export VERSION="1.0.0"
  bash "$SCRIPT_DIR/commit_release_changes.sh"

  expect_files_in_commit "CHANGELOG.md" "file1.txt" "file2.txt" "credentials.json"
}
expect_success "handles whitespace in newline-separated list" test_handles_whitespace_in_list

test_empty_files_list_commits_only_changelog() {
  local repo="$TMPDIR_TEST/test4"
  setup_repo "$repo"

  export FILES_TO_COMMIT=""
  export VERSION="1.0.0"
  bash "$SCRIPT_DIR/commit_release_changes.sh"

  expect_files_in_commit "CHANGELOG.md"
}
expect_success "empty files list commits only CHANGELOG.md" test_empty_files_list_commits_only_changelog

test_commit_message_includes_version() {
  local repo="$TMPDIR_TEST/test5"
  setup_repo "$repo" file1.txt file2.txt credentials.json

  export FILES_TO_COMMIT="file1.txt
file2.txt
credentials.json"
  export VERSION="2.5.3"
  bash "$SCRIPT_DIR/commit_release_changes.sh"

  git log --max-count=1 --pretty=%B | check_contains "Prepare v2.5.3 release"
}
expect_success "commit message includes version" test_commit_message_includes_version

test_fails_when_leftover_untracked_files_exist() {
  local repo="$TMPDIR_TEST/test7"
  setup_repo "$repo" file1.txt .env

  export FILES_TO_COMMIT="file1.txt"
  export VERSION="1.0.0"
  bash "$SCRIPT_DIR/commit_release_changes.sh" 2>&1
}
expect_failure_output "fails when leftover untracked files exist" "Leftover files detected" test_fails_when_leftover_untracked_files_exist

test_warns_and_skips_missing_files() {
  local repo="$TMPDIR_TEST/test8"
  setup_repo "$repo" file1.txt file2.txt credentials.json

  export FILES_TO_COMMIT="file1.txt
file2.txt
credentials.json
missing_file.txt"

  # Capture output to check for warning
  local output
  export VERSION="1.0.0"
  output=$(bash "$SCRIPT_DIR/commit_release_changes.sh" 2>&1)

  expect_files_in_commit "file1.txt" "file2.txt" "credentials.json"
  expect_files_not_in_commit "missing_file.txt"

  # Should log warning about missing file
  echo "$output" | check_contains_pattern "::warning::.*File not found.*missing_file.txt"
}
expect_success "warns and skips missing files" test_warns_and_skips_missing_files

test_fails_when_leftover_unstaged_changes_exist() {
  local repo="$TMPDIR_TEST/test9"
  setup_repo "$repo"

  # Create and commit a file, then modify it without staging
  echo "original" > file1.txt
  git add file1.txt
  git commit -m "Add file1.txt"

  # Modify CHANGELOG again for the release
  echo "## [Unreleased]" >> CHANGELOG.md

  # Now modify file1.txt but don't include it in FILES_TO_COMMIT
  echo "modified" > file1.txt

  export FILES_TO_COMMIT=""
  export VERSION="1.0.0"
  bash "$SCRIPT_DIR/commit_release_changes.sh" 2>&1
}
expect_failure_output "fails when leftover unstaged changes exist" "Leftover files detected" test_fails_when_leftover_unstaged_changes_exist

# Return to original directory before cleanup
cd "$SCRIPT_DIR"

print_results

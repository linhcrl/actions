#!/usr/bin/env bash
# Tests for actions_helpers.sh helpers.
set -euo pipefail
trap 'echo "Error occurred at line $LINENO"; exit 1' ERR

cd "$(dirname "${BASH_SOURCE[0]}")"
source ./test_helpers.sh
source ./actions_helpers.sh

TMPDIR_TEST=$(mktemp -d)
trap 'rm -rf "$TMPDIR_TEST"' EXIT

# --- log_* tests ---

test_log_error() {
  local output
  output=$(log_error "something broke")
  [ "$output" = "::error::something broke" ]
}
expect_success "log_error: formats correctly" test_log_error

test_log_warning() {
  local output
  output=$(log_warning "watch out")
  [ "$output" = "::warning::watch out" ]
}
expect_success "log_warning: formats correctly" test_log_warning

test_log_notice() {
  local output
  output=$(log_notice "fyi")
  [ "$output" = "::notice::fyi" ]
}
expect_success "log_notice: formats correctly" test_log_notice

# --- set_output tests ---

test_set_output() {
  GITHUB_OUTPUT="$TMPDIR_TEST/gh_out_single"
  touch "$GITHUB_OUTPUT"
  set_output "mykey" "myvalue"
  grep -q 'mykey=myvalue' "$GITHUB_OUTPUT"
}
expect_success "set_output: writes key=value" test_set_output

test_set_output_empty_value() {
  GITHUB_OUTPUT="$TMPDIR_TEST/gh_out_empty"
  touch "$GITHUB_OUTPUT"
  set_output "mykey" ""
  grep -q 'mykey=$' "$GITHUB_OUTPUT"
}
expect_success "set_output: handles empty value" test_set_output_empty_value

# --- set_output_multiline tests ---

test_set_output_multiline() {
  GITHUB_OUTPUT="$TMPDIR_TEST/gh_out_multi"
  touch "$GITHUB_OUTPUT"
  set_output_multiline "desc" "line one
line two"
  grep -q 'line one' "$GITHUB_OUTPUT" && grep -q 'line two' "$GITHUB_OUTPUT"
}
expect_success "set_output_multiline: writes multiline content" test_set_output_multiline

test_set_output_multiline_delimiters() {
  GITHUB_OUTPUT="$TMPDIR_TEST/gh_out_delim"
  touch "$GITHUB_OUTPUT"
  set_output_multiline "desc" "content"
  # Should have opening delimiter (desc<<GHEOF_...) and closing delimiter (GHEOF_...)
  grep -q '^desc<<GHEOF_' "$GITHUB_OUTPUT" && grep -q '^GHEOF_' "$GITHUB_OUTPUT"
}
expect_success "set_output_multiline: uses GHEOF delimiters" test_set_output_multiline_delimiters

test_set_output_multiline_empty() {
  GITHUB_OUTPUT="$TMPDIR_TEST/gh_out_multi_empty"
  touch "$GITHUB_OUTPUT"
  set_output_multiline "desc" ""
  grep -q '^desc<<GHEOF_' "$GITHUB_OUTPUT"
}
expect_success "set_output_multiline: handles empty value" test_set_output_multiline_empty

print_results

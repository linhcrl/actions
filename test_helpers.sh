#!/usr/bin/env bash
# Shared test helpers for all test files in this repository.
#
# Usage:
#   source test_helpers.sh
#   expect_success "test name" command [args...]
#   expect_failure "test name" "expected output substring" command [args...]
#   expect_failure "test name" command [args...]
#   print_results

PASS=0
FAIL=0

_run_test() {
  local name="$1"
  local expected_exit="$2"  # exact code, or "nonzero" for any non-zero
  local expected_output="$3"
  shift 3

  local output exit_code
  output=$("$@" 2>&1) && exit_code=0 || exit_code=$?

  if [ "$expected_exit" = "nonzero" ]; then
    if [ "$exit_code" -eq 0 ]; then
      echo "FAIL: $name — expected non-zero exit, got 0"
      echo "  output: $output"
      FAIL=$((FAIL + 1))
      return
    fi
  elif [ "$exit_code" -ne "$expected_exit" ]; then
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

# expect_success "test name" command [args...]
# Asserts the command exits 0.
expect_success() {
  local name="$1"; shift
  _run_test "$name" 0 "" "$@"
}

# expect_failure "test name" ["expected output substring"] command [args...]
# Asserts the command exits non-zero. If the second arg is not a valid command,
# it is treated as an expected output substring.
expect_failure() {
  local name="$1"; shift
  local expected_output=""
  if ! command -v "$1" >/dev/null 2>&1 && ! declare -F "$1" >/dev/null 2>&1; then
    expected_output="$1"; shift
  fi
  _run_test "$name" "nonzero" "$expected_output" "$@"
}

print_results() {
  echo ""
  echo "Results: $PASS passed, $FAIL failed"
  [ "$FAIL" -eq 0 ] || exit 1
}

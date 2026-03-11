#!/usr/bin/env bash
# Run all tests in the repository.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")"; pwd)"
FAILED=0

test_files=()
while IFS= read -r f; do
  test_files+=("$f")
done < <(find "$SCRIPT_DIR" -mindepth 2 -name '*_test.sh' | sort)

if [ "${#test_files[@]}" -eq 0 ]; then
  echo "No tests found."
  exit 0
fi

for test_file in "${test_files[@]}"; do
  echo "=== $test_file ==="
  if bash "$test_file"; then
    echo ""
  else
    echo "FAILED"
    echo ""
    FAILED=1
  fi
done

if [ "$FAILED" -ne 0 ]; then
  echo "Some tests failed."
  exit 1
fi

echo "All tests passed."

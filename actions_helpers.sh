#!/usr/bin/env bash
# Shared shell helpers for all GitHub Actions in this repo.

# GitHub Actions log commands — structured annotations in CI, plain stderr locally.
log_error()   { echo "::error::$*"; }
log_warning() { echo "::warning::$*"; }
log_notice()  { echo "::notice::$*"; }

# Write a single-line output: set_output key value
set_output() {
  echo "$1=$2" >> "${GITHUB_OUTPUT:-/dev/null}"
}

# Write a multiline output: set_output_multiline key value
set_output_multiline() {
  local delim
  delim="GHEOF_$$_$(date +%s)"
  {
    echo "$1<<$delim"
    echo "$2"
    echo "$delim"
  } >> "${GITHUB_OUTPUT:-/dev/null}"
}

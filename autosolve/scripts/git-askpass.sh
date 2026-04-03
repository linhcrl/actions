#!/bin/sh
# GIT_ASKPASS script for fork authentication. Reads credentials from
# environment variables so the token is never written to disk.
set -e

if [ -z "$GIT_FORK_USER" ] || [ -z "$GIT_FORK_PASSWORD" ]; then
  echo "error: GIT_FORK_USER and GIT_FORK_PASSWORD must be set" >&2
  exit 1
fi

case "$1" in
  Username*) echo "$GIT_FORK_USER" ;;
  Password*) echo "$GIT_FORK_PASSWORD" ;;
esac

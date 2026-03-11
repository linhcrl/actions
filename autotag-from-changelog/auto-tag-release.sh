#!/usr/bin/env bash
# Auto-tag releases based on CHANGELOG.md versions.
#
# When the latest released version in CHANGELOG.md (the first ## [x.y.z] after
# ## [Unreleased]) does not have a corresponding git tag, this script creates
# and pushes it.
#
# If there is content under [Unreleased], the script verifies that the latest
# released version is already tagged. If it is, this is normal development and
# the script exits cleanly. If it isn't, something went wrong (a release was
# not tagged) and the script fails.
set -euo pipefail

changelog="${CHANGELOG_PATH:-CHANGELOG.md}"

# Check that the changelog file exists and is readable.
if [ ! -r "$changelog" ]; then
  echo "::error::Changelog file '$changelog' does not exist or is not readable."
  exit 1
fi
# Parse the changelog to extract the unreleased content and latest version.
# Sets two global variables:
#   unreleased_content — non-blank text under ## [Unreleased] (empty if none)
#   version — the first x.y.z version after ## [Unreleased] (empty if none)
parse_changelog() {
  local file="$1"
  unreleased_content=""
  version=""
  local in_unreleased=false
  while IFS= read -r line; do
    if [[ "$line" =~ ^##\ \[Unreleased\] ]]; then
      in_unreleased=true
      continue
    fi
    if $in_unreleased && [[ "$line" =~ ^##\ \[([0-9]+\.[0-9]+\.[0-9]+)\] ]]; then
      version="${BASH_REMATCH[1]}"
      break
    fi
    if $in_unreleased && [[ "$line" =~ [^[:space:]] ]]; then
      unreleased_content+="$line"
    fi
  done < "$file"
}

parse_changelog "$changelog"

# Check if the version string is empty.
if [ -z "$version" ]; then
  echo "No released version found in CHANGELOG.md, skipping."
  exit 0
fi

tag="v${version}"

# Check if the tag already exists on the remote.
remote_output=$(git ls-remote --tags origin "refs/tags/${tag}")
tag_exists=false
# Check if the remote output is non-empty (tag was found).
if [ -n "$remote_output" ]; then
  tag_exists=true
fi

# Check if there is content under the [Unreleased] section.
if [ -n "$unreleased_content" ]; then
  # There is content under [Unreleased]. The previous release must already
  # be tagged — if it isn't, something went wrong.
  if [ "$tag_exists" = true ]; then
    echo "Content under [Unreleased] and ${tag} already tagged, nothing to do."
    exit 0
  else
    echo "::error::CHANGELOG.md has content under [Unreleased] but ${tag} is not tagged. Tag the previous release before adding new entries."
    exit 1
  fi
fi

if [ "$tag_exists" = true ]; then
  echo "Tag ${tag} already exists, nothing to do."
  exit 0
fi

echo "Creating tag ${tag}..."
git tag "$tag"
git push origin "$tag"
echo "Tagged ${tag} successfully."

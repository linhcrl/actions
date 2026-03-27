# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

Breaking changes are prefixed with "Breaking Change: ".

## [Unreleased]

### Changed

- `create-release-pr` reusable workflow: use a hardcoded commit message format instead
  of including changelog entries in the commit body, and fix backtick handling in PR
  descriptions to prevent bash command substitution errors.

## [0.4.0] - 2026-04-09

### Added

- `autotag-from-changelog` action: add `create-major-tag` input (default `true`) to
  control whether major version tags (e.g., `v1`) are created alongside semver tags.
- `create-release-pr` reusable workflow: automates version bump PRs by checking for
  unreleased changes in CHANGELOG, extracting the next version, updating
  the CHANGELOG with new version and release date, optionally running custom update
  scripts, and creating a PR from a fork to the upstream repository.

## [0.3.0] - 2026-04-02

### Changed

- `pr-changelog-check` workflow: removed PR commenting functionality. The workflow
  now only validates CHANGELOG.md and detects breaking changes without posting comments.

## [0.2.0] - 2026-03-31

### Changed

- `autotag-from-changelog` action: now automatically creates and updates major
  version tags (e.g., `v1`) when pushing semver tags (e.g., `v1.2.3`), allowing
  users to reference actions using major version tags that always point to the
  latest release.

### Added

- `pr-changelog-check` workflow: reusable workflow that validates CHANGELOG.md
  changes in PRs, detects breaking changes, and posts status comments.
- `autotag-from-changelog` now exposes `tag_created` and `tag` outputs so
  callers can react to whether a new tag was pushed.
- `expect_step_output` test helper for asserting GitHub Actions step outputs.
- `autosolve/assess` action: evaluate tasks for automated resolution suitability
  using Claude in read-only mode.
- `autosolve/implement` action: autonomously implement solutions, validate
  security, push to fork, and create PRs using Claude. Includes AI security
  review, token usage tracking, and per-file batched diff analysis.
- `get-workflow-ref` action: resolve the ref a caller used to invoke a reusable
  workflow by parsing the caller's workflow file — no API calls or extra
  permissions needed.
- Shared shell helpers (`actions_helpers.sh`) and test framework (`test_helpers.sh`)
  for consistent logging, output handling, and test assertions across actions.

## [0.1.0] - 2026-03-23

### Added

- `release-version-extract` action: extracts current version and determines next
  version based on unreleased changes in CHANGELOG.md.
- `changelog-check` action: validates CHANGELOG.md format, version ordering,
  and detects breaking changes.
- `autotag-from-changelog` action: tag and push from CHANGELOG.md version
  change.

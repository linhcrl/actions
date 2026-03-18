# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

Breaking changes are prefixed with "Breaking Change: ".

## [Unreleased]

### Added

- `release-version-extract` action: extracts current version and determines next
  version based on unreleased changes in CHANGELOG.md.
- `changelog-check` action: validates CHANGELOG.md format, version ordering,
  and detects breaking changes.
- `autotag-from-changelog` action: tag and push from CHANGELOG.md version
  change.

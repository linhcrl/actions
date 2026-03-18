# cockroachdb/actions

Reusable GitHub Actions and workflows for CockroachDB projects.

## Actions

### autotag-from-changelog

Creates git tags from CHANGELOG.md versions. Fails only when there is content
under `[Unreleased]` and the previous release version tag does not yet exist;
otherwise it succeeds even if `[Unreleased]` contains entries.

**Usage:**

```yaml
- uses: cockroachdb/actions/autotag-from-changelog@v1
```

**Inputs:**

| Name             | Default        | Description                |
| ---------------- | -------------- | -------------------------- |
| `changelog-path` | `CHANGELOG.md` | Path to the changelog file |

**Required permissions:**

```yaml
permissions:
  contents: write
```

### changelog-check

Validates that CHANGELOG.md follows the [Keep a Changelog](https://keepachangelog.com/)
standard. Ensures proper changelog structure, version ordering,
and detects breaking changes to enable automated version bump determination.

**Usage:**

```yaml
- uses: cockroachdb/actions/changelog-check@v1
  with:
    check-mode: diff
    base-ref: ${{ github.event.pull_request.base.ref }}
```

**Inputs:**

| Name               | Default        | Description                                                              |
| ------------------ | -------------- | ------------------------------------------------------------------------ |
| `changelog-path`   | `CHANGELOG.md` | Path to the changelog file                                               |
| `validation-depth` | `1`            | How many changelog entries to validate starting from the most recent     |
| `check-mode`       | `unreleased`   | Check mode for breaking change detection: `unreleased` (entire Unreleased section) or `diff` (PR changes only). Does not affect format/version validation, which always runs. |
| `base-ref`         | `''`           | Required when `check-mode` is `diff`. The base git ref to compare against for detecting breaking changes in the diff only (e.g., `main`, or `github.event.pull_request.base.ref` in PRs). Not needed for `unreleased` mode. |

**Outputs:**

| Name            | Description                                                      |
| --------------- | ---------------------------------------------------------------- |
| `is_valid`      | Whether the CHANGELOG format and version ordering are valid      |
| `has_breaking`  | Whether breaking changes were detected                           |

**Features:**

- Validates CHANGELOG.md format using Keep a Changelog standard
- Checks that versions are in descending order (newest first)
- Checks that release dates are in descending order
- Detects breaking changes via two methods:
  - Entries prefixed with `Breaking Change:` in any section
  - Presence of a `### Removed` section header
- Supports checking entire Unreleased section or only PR diff

### release-version-extract

Extracts the current version from CHANGELOG.md and determines the next version
based on unreleased changes. Analyzes changelog entries to automatically
determine whether a major, minor, or patch version bump is needed.

**Usage:**

```yaml
- uses: cockroachdb/actions/release-version-extract@v1
  id: version
- run: echo "Next version will be ${{ steps.version.outputs.next_version }}"
```

**Inputs:**

| Name             | Default        | Description                |
| ---------------- | -------------- | -------------------------- |
| `changelog-path` | `CHANGELOG.md` | Path to the changelog file |

**Outputs:**

| Name              | Description                                                      |
| ----------------- | ---------------------------------------------------------------- |
| `current_version` | Current latest released version (empty if no releases)           |
| `next_version`    | Suggested next version based on unreleased changes               |
| `bump_type`       | Type of version bump (`major`/`minor`/`patch`/`initial`, or empty if no changes) |
| `has_unreleased`  | Whether there are unreleased changes (`true`/`false`)            |

**Features:**

- Automatically determines version bump type from changelog entries
- Detects major bumps when breaking changes are present (lines prefixed with `Breaking Change:` or `### Removed` section)
- Handles initial releases (first release → 0.1.0)
- Returns empty `bump_type` when there are no unreleased changes
- Follows semantic versioning principles

## Development

Run all tests locally:

```sh
./test.sh
```

Tests also run automatically on pull requests via CI.

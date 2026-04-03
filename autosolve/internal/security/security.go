// Package security enforces blocked-path restrictions, symlink detection,
// and sensitive file checks on the git working tree.
package security

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/actions/autosolve/internal/action"
	"github.com/cockroachdb/actions/autosolve/internal/git"
)

// sensitivePatterns are filename patterns that should never be committed.
// Matches are checked against the basename of each changed file.
var sensitivePatterns = []string{
	"gha-creds-",       // google-github-actions/auth credential files
	".env",             // environment variable files
	"credentials.json", // GCP service account keys
	"service-account",  // service account key files
}

// sensitiveExtensions are file extensions that indicate sensitive content.
var sensitiveExtensions = []string{
	".pem",
	".key",
	".p12",
	".pfx",
	".keystore",
}

// Check scans the working tree for modifications to blocked paths and
// sensitive files. It returns a list of violations. If violations are
// found, it resets the staging area.
func Check(gitClient git.Client, blockedPaths []string) ([]string, error) {
	changed, err := git.ChangedFiles(gitClient)
	if err != nil {
		return nil, fmt.Errorf("listing changed files: %w", err)
	}

	repoRootBytes, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, fmt.Errorf("getting repo root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(repoRootBytes))

	var violations []string
	for _, file := range changed {
		// Check the file path itself against blocked prefixes
		for _, blocked := range blockedPaths {
			if strings.HasPrefix(file, blocked) {
				violations = append(violations, fmt.Sprintf("blocked path modified: %s (matches prefix %s)", file, blocked))
			}
		}

		// Check for sensitive files
		if v := checkSensitiveFile(file); v != "" {
			violations = append(violations, v)
		}

		// Resolve the real path to catch symlinks (both the file itself and
		// any symlinked parent directories)
		absPath := filepath.Join(repoRoot, file)
		realPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				// File was deleted or not yet on disk — nothing to resolve.
				continue
			}
			violations = append(violations, fmt.Sprintf("cannot resolve path: %s (%v)", file, err))
			continue
		}
		// If the real path differs from the expected path, a symlink is involved
		if realPath != absPath {
			for _, blocked := range blockedPaths {
				blockedAbs := filepath.Clean(filepath.Join(repoRoot, blocked))
				if strings.HasPrefix(realPath, blockedAbs+string(filepath.Separator)) || realPath == blockedAbs {
					violations = append(violations, fmt.Sprintf("symlink to blocked path: %s -> %s", file, realPath))
				}
			}
		}
	}

	if len(violations) > 0 {
		// Best-effort unstage; safe to continue because the caller
		// treats any violation as a terminal error before pushing.
		if err := gitClient.ResetHead(); err != nil {
			action.LogWarning(fmt.Sprintf("failed to reset staged changes: %v", err))
		}
	}

	return violations, nil
}

// checkSensitiveFile returns a violation message if the file matches a
// known sensitive pattern, or empty string if it's safe.
func checkSensitiveFile(file string) string {
	base := filepath.Base(file)
	lower := strings.ToLower(base)

	for _, pattern := range sensitivePatterns {
		if strings.Contains(lower, pattern) {
			return fmt.Sprintf("sensitive file detected: %s (matches pattern %q)", file, pattern)
		}
	}

	ext := strings.ToLower(filepath.Ext(file))
	for _, sensitiveExt := range sensitiveExtensions {
		if ext == sensitiveExt {
			return fmt.Sprintf("sensitive file detected: %s (has sensitive extension %s)", file, sensitiveExt)
		}
	}

	return ""
}

// gitignorePatterns are the credential patterns we recommend excluding.
var gitignorePatterns = []string{
	"gha-creds-*.json",
	"*.pem",
	"*.key",
	"*.p12",
	"*.pfx",
	"*.keystore",
	"credentials.json",
	"service-account*.json",
}

// CheckGitignore logs a warning if the repo's .gitignore does not contain
// credential exclusion patterns. It does not modify the file — repo owners
// should add the patterns themselves for defense-in-depth.
func CheckGitignore(logWarning func(string)) {
	data, err := os.ReadFile(".gitignore")
	if err != nil {
		logWarning("No .gitignore found. For defense-in-depth, add one with credential exclusion patterns: " +
			strings.Join(gitignorePatterns, ", "))
		return
	}
	content := string(data)
	var missing []string
	for _, p := range gitignorePatterns {
		if !strings.Contains(content, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		logWarning("Repo .gitignore is missing recommended credential exclusion patterns: " +
			strings.Join(missing, ", "))
	}
}

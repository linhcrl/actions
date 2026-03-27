// Package action provides helpers for GitHub Actions I/O: outputs, summaries,
// and structured log annotations.
package action

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SetOutput writes a single-line output to $GITHUB_OUTPUT.
func SetOutput(key, value string) {
	appendToFile(os.Getenv("GITHUB_OUTPUT"), fmt.Sprintf("%s=%s", key, value))
}

// SetOutputMultiline writes a multiline output to $GITHUB_OUTPUT using a
// heredoc-style delimiter with a random suffix to avoid collisions.
func SetOutputMultiline(key, value string) {
	delim := randomDelimiter()
	content := fmt.Sprintf("%s<<%s\n%s\n%s", key, delim, value, delim)
	appendToFile(os.Getenv("GITHUB_OUTPUT"), content)
}

// WriteStepSummary appends markdown content to $GITHUB_STEP_SUMMARY.
func WriteStepSummary(content string) {
	appendToFile(os.Getenv("GITHUB_STEP_SUMMARY"), content)
}

// LogError emits a GitHub Actions error annotation.
func LogError(msg string) {
	fmt.Fprintf(os.Stderr, "::error::%s\n", msg)
}

// LogWarning emits a GitHub Actions warning annotation.
func LogWarning(msg string) {
	fmt.Fprintf(os.Stderr, "::warning::%s\n", msg)
}

// LogNotice emits a GitHub Actions notice annotation.
func LogNotice(msg string) {
	fmt.Fprintf(os.Stderr, "::notice::%s\n", msg)
}

// LogInfo writes informational output (no annotation).
func LogInfo(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

// TruncateOutput limits text to maxLines, appending a truncation notice if needed.
func TruncateOutput(maxLines int, text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	truncated := strings.Join(lines[:maxLines], "\n")
	return fmt.Sprintf("%s\n[... truncated (%d lines total, showing first %d)]", truncated, len(lines), maxLines)
}

// SaveLogArtifact copies a file to $RUNNER_TEMP/autosolve-logs/ so the calling
// workflow can upload it as an artifact for debugging.
func SaveLogArtifact(srcPath, name string) {
	dir := os.Getenv("RUNNER_TEMP")
	if dir == "" {
		dir = os.TempDir()
	}
	logDir := filepath.Join(dir, "autosolve-logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		LogWarning(fmt.Sprintf("failed to create log artifact dir: %v", err))
		return
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		LogWarning(fmt.Sprintf("failed to read %s for artifact: %v", srcPath, err))
		return
	}
	dst := filepath.Join(logDir, name)
	if err := os.WriteFile(dst, data, 0644); err != nil {
		LogWarning(fmt.Sprintf("failed to write log artifact %s: %v", dst, err))
		return
	}
	LogInfo(fmt.Sprintf("Saved log artifact: %s", dst))
}

func randomDelimiter() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("GHEOF_%d", os.Getpid())
	}
	return "GHEOF_" + hex.EncodeToString(b)
}

func appendToFile(path, content string) {
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "::warning::failed to open %s: %v\n", path, err)
		return
	}
	defer f.Close()
	fmt.Fprintln(f, content)
}

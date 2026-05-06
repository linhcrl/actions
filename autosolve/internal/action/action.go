// Package action provides helpers for GitHub Actions I/O: outputs, summaries,
// and structured log annotations.
package action

import (
	"fmt"
	"os"
	"strings"
)

// SetOutput writes a single-line output to $GITHUB_OUTPUT.
func SetOutput(key, value string) error {
	return appendToFile(os.Getenv("GITHUB_OUTPUT"), fmt.Sprintf("%s=%s", key, value))
}

// SetOutputMultiline writes a multiline output to $GITHUB_OUTPUT using a
// heredoc-style delimiter.
func SetOutputMultiline(key, value string) error {
	content := fmt.Sprintf("%s<<GHEOF\n%s\nGHEOF", key, value)
	return appendToFile(os.Getenv("GITHUB_OUTPUT"), content)
}

// WriteStepSummary appends markdown content to $GITHUB_STEP_SUMMARY.
func WriteStepSummary(content string) error {
	return appendToFile(os.Getenv("GITHUB_STEP_SUMMARY"), content)
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

// BeginLogGroup opens a collapsible ::group:: in the GitHub Actions step log.
// Content streamed to stderr between Begin and End is visible in real-time
// but collapsed by default once complete.
func BeginLogGroup(name string) {
	fmt.Fprintf(os.Stderr, "::group::%s\n", name)
}

// EndLogGroup closes the current ::group:: block.
func EndLogGroup() {
	fmt.Fprintln(os.Stderr, "::endgroup::")
}

func appendToFile(path, content string) error {
	if path == "" {
		return fmt.Errorf("output file path is empty (missing GITHUB_OUTPUT or GITHUB_STEP_SUMMARY env var)")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, content); err != nil {
		return fmt.Errorf("writing to %s: %w", path, err)
	}
	return nil
}

// Package action provides helpers for GitHub Actions I/O: outputs, summaries,
// and structured log annotations.
package action

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/actions/autosolve/internal/claude"
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

// SaveLogArtifact copies a file to $RUNNER_TEMP/autosolve-logs/ so the calling
// workflow can upload it as an artifact for debugging.
func SaveLogArtifact(srcPath, name string) error {
	dir := os.Getenv("RUNNER_TEMP")
	if dir == "" {
		dir = os.TempDir()
	}
	logDir := filepath.Join(dir, "autosolve-logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("creating log artifact dir: %w", err)
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("reading %s for artifact: %w", srcPath, err)
	}
	dst := filepath.Join(logDir, name)
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("writing log artifact %s: %w", dst, err)
	}
	LogInfo(fmt.Sprintf("Saved log artifact: %s", dst))
	return nil
}

// LogResult records usage for a Claude invocation, logs token counts, and
// saves the output file as a log artifact. Call immediately after runner.Run
// and before checking the error so that usage and artifacts are captured
// even on failure.
func LogResult(tracker *claude.UsageTracker, result *claude.Result, section, outputFile string) {
	tracker.Record(section, result.Usage)
	LogInfo(fmt.Sprintf("%s usage: input=%d output=%d cost=$%.4f",
		section, result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.CostUSD))
	artifactName := strings.NewReplacer(" ", "_", "(", "", ")", "").Replace(section) + ".json"
	if err := SaveLogArtifact(outputFile, artifactName); err != nil {
		LogWarning(fmt.Sprintf("failed to save log artifact: %v", err))
	}
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

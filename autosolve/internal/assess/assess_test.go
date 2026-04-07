package assess

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/cockroachdb/actions/autosolve/internal/claude"
	"github.com/cockroachdb/actions/autosolve/internal/config"
)

type mockRunner struct {
	resultText string
	sessionID  string
	exitCode   int
	err        error
}

func (m *mockRunner) Run(ctx context.Context, opts claude.RunOptions) (*claude.Result, error) {
	if m.err != nil {
		return &claude.Result{}, m.err
	}
	// Write the mock output to the output file
	out := struct {
		Type      string `json:"type"`
		Result    string `json:"result"`
		SessionID string `json:"session_id"`
	}{
		Type:      "result",
		Result:    m.resultText,
		SessionID: m.sessionID,
	}
	data, _ := json.Marshal(out)
	os.WriteFile(opts.OutputFile, data, 0644)

	result := &claude.Result{
		ResultText: m.resultText,
		SessionID:  m.sessionID,
		ExitCode:   m.exitCode,
	}
	if m.resultText == "" {
		return result, fmt.Errorf("claude produced empty result (exit code %d)", m.exitCode)
	}
	return result, nil
}

func TestRun_Proceed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("GITHUB_OUTPUT", tmpDir+"/output")
	t.Setenv("GITHUB_STEP_SUMMARY", tmpDir+"/summary")

	cfg := &config.Config{
		SystemPrompt: "Fix the bug",
		Model:        "sonnet",
		BlockedPaths: []string{".github/workflows/"},
		FooterType:   "assessment",
	}

	runner := &mockRunner{
		resultText: "The task is clear and bounded.\n\nASSESSMENT_RESULT - PROCEED",
	}

	err := Run(context.Background(), cfg, runner, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Check outputs were written
	data, _ := os.ReadFile(tmpDir + "/output")
	content := string(data)
	if content == "" {
		t.Error("expected output to be written")
	}
}

func TestRun_Skip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("GITHUB_OUTPUT", tmpDir+"/output")
	t.Setenv("GITHUB_STEP_SUMMARY", tmpDir+"/summary")

	cfg := &config.Config{
		SystemPrompt: "Refactor everything",
		Model:        "sonnet",
		BlockedPaths: []string{".github/workflows/"},
		FooterType:   "assessment",
	}

	runner := &mockRunner{
		resultText: "Too complex for automation.\n\nASSESSMENT_RESULT - SKIP",
	}

	err := Run(context.Background(), cfg, runner, tmpDir)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRun_NoResult(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("GITHUB_OUTPUT", tmpDir+"/output")
	t.Setenv("GITHUB_STEP_SUMMARY", tmpDir+"/summary")

	cfg := &config.Config{
		SystemPrompt: "Fix it",
		Model:        "sonnet",
		BlockedPaths: []string{".github/workflows/"},
		FooterType:   "assessment",
	}

	runner := &mockRunner{
		resultText: "",
	}

	err := Run(context.Background(), cfg, runner, tmpDir)
	if err == nil {
		t.Error("expected error when no result")
	}
}

func TestExtractSummary(t *testing.T) {
	text := "Line 1\nLine 2\nASSESSMENT_RESULT - PROCEED"
	summary := extractSummary(text, "ASSESSMENT_RESULT")
	if summary != "Line 1\nLine 2" {
		t.Errorf("expected summary without marker, got %q", summary)
	}
}

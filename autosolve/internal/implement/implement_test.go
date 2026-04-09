package implement

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/actions/autosolve/internal/claude"
	"github.com/cockroachdb/actions/autosolve/internal/config"
	"github.com/cockroachdb/actions/autosolve/internal/github"
)

type mockRunner struct {
	calls      int
	results    []string // result text per attempt
	sessionIDs []string
	exitCodes  []int
}

func (m *mockRunner) Run(ctx context.Context, opts claude.RunOptions) (*claude.Result, error) {
	idx := m.calls
	m.calls++

	resultText := ""
	if idx < len(m.results) {
		resultText = m.results[idx]
	}
	sessionID := ""
	if idx < len(m.sessionIDs) {
		sessionID = m.sessionIDs[idx]
	}
	exitCode := 0
	if idx < len(m.exitCodes) {
		exitCode = m.exitCodes[idx]
	}

	// Write mock output to the output file
	out := struct {
		Type      string `json:"type"`
		Result    string `json:"result"`
		SessionID string `json:"session_id"`
	}{
		Type:      "result",
		Result:    resultText,
		SessionID: sessionID,
	}
	data, _ := json.Marshal(out)
	os.WriteFile(opts.OutputFile, data, 0644)

	// Simulate Claude writing metadata files on success
	if strings.Contains(resultText, "SUCCESS") {
		os.WriteFile(".autosolve-commit-message", []byte("fix: mock commit"), 0644)
	}

	result := &claude.Result{
		ResultText: resultText,
		SessionID:  sessionID,
		ExitCode:   exitCode,
	}
	if resultText == "" {
		return result, fmt.Errorf("claude produced empty result (exit code %d)", exitCode)
	}
	return result, nil
}

type mockGHClient struct {
	labels []string
	prURL  string
	prErr  error
}

func (m *mockGHClient) CreatePR(_ context.Context, opts github.PullRequestOptions) (string, error) {
	if m.prErr != nil {
		return "", m.prErr
	}
	return m.prURL, nil
}

func (m *mockGHClient) CreateLabel(_ context.Context, _ string, name string) error {
	m.labels = append(m.labels, name)
	return nil
}

type mockGitClient struct{}

func (m *mockGitClient) Diff(args ...string) (string, error)    { return "", nil }
func (m *mockGitClient) LsFiles(args ...string) (string, error) { return "", nil }
func (m *mockGitClient) Config(args ...string) error            { return nil }
func (m *mockGitClient) Remote(args ...string) (string, error)  { return "", nil }
func (m *mockGitClient) Checkout(args ...string) error          { return nil }
func (m *mockGitClient) Add(args ...string) error               { return nil }
func (m *mockGitClient) Commit(message string) error            { return nil }
func (m *mockGitClient) Push(args ...string) error              { return nil }
func (m *mockGitClient) Log(args ...string) (string, error)     { return "", nil }
func (m *mockGitClient) ResetHead() error                       { return nil }

func init() {
	RetryDelay = 0 * time.Millisecond
}

func TestRun_SuccessNoPR(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { os.Remove(".autosolve-commit-message") })
	t.Setenv("GITHUB_OUTPUT", tmpDir+"/output")
	t.Setenv("GITHUB_STEP_SUMMARY", tmpDir+"/summary")

	cfg := &config.Config{
		SystemPrompt: "Fix the bug",
		Model:        "sonnet",
		BlockedPaths: []string{".github/workflows/"},
		FooterType:   "implementation",
		MaxRetries:   3,
		AllowedTools: "Read,Write,Edit",
		CreatePR:     false,
	}

	runner := &mockRunner{
		results: []string{"Fixed it.\n\nIMPLEMENTATION_RESULT - SUCCESS"},
	}

	err := Run(context.Background(), cfg, runner, &mockGHClient{}, &mockGitClient{}, tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Errorf("expected 1 call, got %d", runner.calls)
	}
}

func TestRun_RetryThenSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { os.Remove(".autosolve-commit-message") })
	t.Setenv("GITHUB_OUTPUT", tmpDir+"/output")
	t.Setenv("GITHUB_STEP_SUMMARY", tmpDir+"/summary")

	cfg := &config.Config{
		SystemPrompt: "Fix the bug",
		Model:        "sonnet",
		BlockedPaths: []string{".github/workflows/"},
		FooterType:   "implementation",
		MaxRetries:   3,
		AllowedTools: "Read,Write,Edit",
		CreatePR:     false,
	}

	runner := &mockRunner{
		results:    []string{"IMPLEMENTATION_RESULT - FAILED", "IMPLEMENTATION_RESULT - SUCCESS"},
		sessionIDs: []string{"sess-1", "sess-1"},
	}

	err := Run(context.Background(), cfg, runner, &mockGHClient{}, &mockGitClient{}, tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 2 {
		t.Errorf("expected 2 calls (1 retry), got %d", runner.calls)
	}
}

func TestRun_AllRetriesFail(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("GITHUB_OUTPUT", tmpDir+"/output")
	t.Setenv("GITHUB_STEP_SUMMARY", tmpDir+"/summary")

	cfg := &config.Config{
		SystemPrompt: "Fix the bug",
		Model:        "sonnet",
		BlockedPaths: []string{".github/workflows/"},
		FooterType:   "implementation",
		MaxRetries:   2,
		AllowedTools: "Read,Write,Edit",
		CreatePR:     false,
	}

	runner := &mockRunner{
		results: []string{"IMPLEMENTATION_RESULT - FAILED", "IMPLEMENTATION_RESULT - FAILED"},
	}

	// Should return an error so the step exits non-zero.
	err := Run(context.Background(), cfg, runner, &mockGHClient{}, &mockGitClient{}, tmpDir)
	if err == nil {
		t.Fatal("expected error when all retries fail")
	}
	if runner.calls != 2 {
		t.Errorf("expected 2 calls, got %d", runner.calls)
	}
}

func TestExtractSummary(t *testing.T) {
	text := "Fixed the timeout issue.\nAdded test.\nIMPLEMENTATION_RESULT - SUCCESS"
	summary := extractSummary(text, "IMPLEMENTATION_RESULT")
	if summary != "Fixed the timeout issue.\nAdded test." {
		t.Errorf("unexpected summary: %q", summary)
	}
}

func TestWriteOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("GITHUB_OUTPUT", tmpDir+"/output")
	t.Setenv("GITHUB_STEP_SUMMARY", tmpDir+"/summary")

	err := writeOutputs("SUCCESS", "https://github.com/org/repo/pull/1", "autosolve/fix-123", "Done\nIMPLEMENTATION_RESULT - SUCCESS", nil)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(tmpDir + "/output")
	content := string(data)
	if content == "" {
		t.Error("expected outputs to be written")
	}

	summaryData, _ := os.ReadFile(tmpDir + "/summary")
	summary := string(summaryData)
	if summary == "" {
		t.Error("expected step summary to be written")
	}
}

func TestReadCommitMessage(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	t.Run("missing file returns error", func(t *testing.T) {
		_, _, err := readCommitMessage()
		if err == nil {
			t.Error("expected error when file is missing")
		}
	})

	t.Run("subject only", func(t *testing.T) {
		os.WriteFile(".autosolve-commit-message", []byte("fix: broken build"), 0644)
		subject, body, err := readCommitMessage()
		if err != nil {
			t.Fatal(err)
		}
		if subject != "fix: broken build" {
			t.Errorf("unexpected subject: %q", subject)
		}
		if body != "" {
			t.Errorf("expected empty body, got: %q", body)
		}
	})

	t.Run("subject and body", func(t *testing.T) {
		os.WriteFile(".autosolve-commit-message", []byte("fix: broken build\n\nDetailed explanation here."), 0644)
		subject, body, err := readCommitMessage()
		if err != nil {
			t.Fatal(err)
		}
		if subject != "fix: broken build" {
			t.Errorf("unexpected subject: %q", subject)
		}
		if body != "Detailed explanation here." {
			t.Errorf("unexpected body: %q", body)
		}
	})

	t.Run("file is removed after read", func(t *testing.T) {
		os.WriteFile(".autosolve-commit-message", []byte("subject"), 0644)
		readCommitMessage()
		if _, err := os.Stat(".autosolve-commit-message"); !os.IsNotExist(err) {
			t.Error("expected file to be removed after read")
		}
	})
}

func TestBuildPRBody(t *testing.T) {
	t.Run("uses template with placeholders", func(t *testing.T) {
		cfg := &config.Config{
			PRBodyTemplate: "Branch: {{BRANCH}}\nSummary: {{SUMMARY}}",
			PRFooter:       "-- footer",
		}
		body := buildPRBody(cfg, t.TempDir(), "autosolve/fix-1", "Fixed it.\nIMPLEMENTATION_RESULT - SUCCESS")
		if body != "Branch: autosolve/fix-1\nSummary: Fixed it.\n\n-- footer" {
			t.Errorf("unexpected body: %q", body)
		}
	})

	t.Run("uses pr-body file when no template", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "autosolve-pr-body"), []byte("Custom PR body from Claude."), 0644)

		cfg := &config.Config{PRFooter: "-- footer"}
		body := buildPRBody(cfg, tmpDir, "autosolve/fix-1", "result text")
		if body != "Custom PR body from Claude.\n\n-- footer" {
			t.Errorf("unexpected body: %q", body)
		}
	})

	t.Run("no template or file appends footer only", func(t *testing.T) {
		cfg := &config.Config{PRFooter: "-- footer"}
		body := buildPRBody(cfg, t.TempDir(), "autosolve/fix-1", "result text")
		if body != "\n\n-- footer" {
			t.Errorf("unexpected body: %q", body)
		}
	})
}

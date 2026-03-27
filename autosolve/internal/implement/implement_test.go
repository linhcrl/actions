package implement

import (
	"context"
	"encoding/json"
	"os"
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

	return &claude.Result{
		ResultText: resultText,
		SessionID:  sessionID,
		ExitCode:   exitCode,
	}, nil
}

type mockGHClient struct {
	comments []string
	labels   []string
	prURL    string
	prErr    error
}

func (m *mockGHClient) CreateComment(_ context.Context, _ string, _ int, body string) error {
	m.comments = append(m.comments, body)
	return nil
}

func (m *mockGHClient) RemoveLabel(_ context.Context, _ string, _ int, label string) error {
	m.labels = append(m.labels, label)
	return nil
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

func (m *mockGHClient) FindPRByLabel(_ context.Context, _ string, _ string) (string, error) {
	return "", nil
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
func (m *mockGitClient) SymbolicRef(ref string) (string, error) { return "", nil }

func init() {
	RetryDelay = 0 * time.Millisecond
}

func TestRun_SuccessNoPR(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("GITHUB_OUTPUT", tmpDir+"/output")
	t.Setenv("GITHUB_STEP_SUMMARY", tmpDir+"/summary")

	cfg := &config.Config{
		Prompt:       "Fix the bug",
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
	t.Setenv("GITHUB_OUTPUT", tmpDir+"/output")
	t.Setenv("GITHUB_STEP_SUMMARY", tmpDir+"/summary")

	cfg := &config.Config{
		Prompt:       "Fix the bug",
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
		Prompt:       "Fix the bug",
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

	// Should not return error — just sets status to FAILED
	err := Run(context.Background(), cfg, runner, &mockGHClient{}, &mockGitClient{}, tmpDir)
	if err != nil {
		t.Fatal(err)
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

// Package claude provides an interface for invoking the Claude CLI and
// parsing its JSON output.
package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/actions/autosolve/internal/action"
)

// Runner invokes the Claude CLI.
type Runner interface {
	Run(ctx context.Context, opts RunOptions) (*Result, error)
}

// RunOptions configures a Claude CLI invocation.
type RunOptions struct {
	Model        string
	AllowedTools string
	MaxTurns     int
	Prompt       string   // prompt text (used as stdin on first attempt)
	PromptFile   string   // path to prompt file (used as stdin on first attempt; Prompt takes precedence)
	Resume       string   // session ID for --resume
	RetryPrompt  string   // prompt text for retry attempts (used as stdin with --resume)
	OutputFile   string   // path to write JSON output
	ContextVars  []string // env var names to pass through to the Claude subprocess
	LogLevel     string   // "error", "info", or "debug" — controls real-time streaming to stderr
}

// BaselineEnvVars are environment variables always passed to the Claude CLI
// subprocess regardless of ContextVars. These are required for the CLI to
// function and for basic tool operation (git, compilers, etc.).
//
// Caller-specified context vars (e.g., ISSUE_TITLE, ISSUE_BODY) must be
// listed in ContextVars to be visible to Claude.
var BaselineEnvVars = []string{
	// System essentials
	"PATH",
	"HOME",
	"USER",
	"SHELL",
	"TMPDIR",
	"LANG",
	"LC_ALL",

	// Claude CLI authentication (Vertex AI)
	"CLAUDE_CODE_USE_VERTEX",
	"ANTHROPIC_VERTEX_PROJECT_ID",
	"CLOUD_ML_REGION",
	"GOOGLE_APPLICATION_CREDENTIALS",

	// GitHub Actions runtime
	"RUNNER_TEMP",
	"GITHUB_WORKSPACE",
	"GITHUB_REPOSITORY",
}

// Result holds parsed Claude CLI output.
type Result struct {
	ResultText        string
	SessionID         string
	ExitCode          int
	Usage             Usage
	PermissionDenials int
}

// Usage holds token counts and cost from a Claude CLI invocation.
type Usage struct {
	InputTokens              int     `json:"input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens"`
	CostUSD                  float64 `json:"cost_usd"`
}

// Add combines the token counts and cost from another Usage into this one.
func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheCreationInputTokens += other.CacheCreationInputTokens
	u.CacheReadInputTokens += other.CacheReadInputTokens
	u.CostUSD += other.CostUSD
}

// UsageTracker accumulates token usage across multiple Claude invocations,
// organized by named sections (e.g. "assess", "implement", "security-review").
type UsageTracker struct {
	Sections []UsageSection
}

// UsageSection records usage for a named phase.
type UsageSection struct {
	Name  string
	Usage Usage
}

// Record adds usage to the named section.
func (t *UsageTracker) Record(section string, u Usage) {
	for i := range t.Sections {
		if t.Sections[i].Name == section {
			t.Sections[i].Usage.Add(u)
			return
		}
	}
	t.Sections = append(t.Sections, UsageSection{Name: section, Usage: u})
}

// Total returns the combined usage across all sections.
func (t *UsageTracker) Total() Usage {
	var total Usage
	for _, s := range t.Sections {
		total.Add(s.Usage)
	}
	return total
}

// FormatSummary returns a markdown table summarizing usage by section with totals.
func (t *UsageTracker) FormatSummary() string {
	if len(t.Sections) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Token Usage\n")
	b.WriteString("| Section | Input | Output | Cache Create | Cache Read | Cost |\n")
	b.WriteString("|---------|------:|-------:|-------------:|-----------:|-----:|\n")
	for _, s := range t.Sections {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | $%.4f |\n",
			s.Name, s.Usage.InputTokens, s.Usage.OutputTokens,
			s.Usage.CacheCreationInputTokens, s.Usage.CacheReadInputTokens,
			s.Usage.CostUSD)
	}
	total := t.Total()
	fmt.Fprintf(&b, "| **Total** | **%d** | **%d** | **%d** | **%d** | **$%.4f** |\n",
		total.InputTokens, total.OutputTokens,
		total.CacheCreationInputTokens, total.CacheReadInputTokens,
		total.CostUSD)
	return b.String()
}

// UsageSummaryPath returns the path to the rendered markdown usage table,
// using RUNNER_TEMP if available (GitHub Actions), falling back to the
// system temp directory.
func UsageSummaryPath() string {
	dir := os.Getenv("RUNNER_TEMP")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "autosolve-usage.md")
}

// Load reads previously saved usage sections by parsing the markdown
// table at UsageSummaryPath. Silently returns if the file doesn't exist
// or can't be parsed. Loaded sections are prepended so earlier phases
// appear first.
func (t *UsageTracker) Load() {
	data, err := os.ReadFile(UsageSummaryPath())
	if err != nil {
		return
	}
	sections := ParseSummary(string(data))
	t.Sections = append(sections, t.Sections...)
}

// Save writes the formatted markdown usage table to UsageSummaryPath.
// The file is always a complete, self-contained table ready to append
// to GITHUB_STEP_SUMMARY.
func (t *UsageTracker) Save() error {
	return os.WriteFile(UsageSummaryPath(), []byte(t.FormatSummary()), 0644)
}

// LogResult records usage for a Claude invocation and logs token counts.
// Call immediately after runner.Run and before checking the error so that
// usage is captured even on failure.
func LogResult(tracker *UsageTracker, result *Result, section string) {
	tracker.Record(section, result.Usage)
	action.LogInfo(fmt.Sprintf("%s usage: input=%d output=%d cost=$%.4f",
		section, result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.CostUSD))
	if result.PermissionDenials > 0 {
		action.LogWarning(fmt.Sprintf("%s: %d tool call(s) were denied by permission policy",
			section, result.PermissionDenials))
	}
}

// ParseSummary parses a markdown usage table (as produced by
// FormatSummary) back into UsageSection entries. It skips header rows
// and the totals row.
func ParseSummary(md string) []UsageSection {
	var sections []UsageSection
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		// Skip header, separator, and totals rows
		if strings.Contains(line, "Section") ||
			strings.Contains(line, "---") ||
			strings.Contains(line, "**Total**") {
			continue
		}
		parts := strings.Split(line, "|")
		// Expected: empty, name, input, output, cache_create, cache_read, cost, empty
		if len(parts) < 8 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		if name == "" {
			continue
		}
		var s UsageSection
		s.Name = name
		fmt.Sscanf(strings.TrimSpace(parts[2]), "%d", &s.Usage.InputTokens)
		fmt.Sscanf(strings.TrimSpace(parts[3]), "%d", &s.Usage.OutputTokens)
		fmt.Sscanf(strings.TrimSpace(parts[4]), "%d", &s.Usage.CacheCreationInputTokens)
		fmt.Sscanf(strings.TrimSpace(parts[5]), "%d", &s.Usage.CacheReadInputTokens)
		fmt.Sscanf(strings.TrimSpace(parts[6]), "$%f", &s.Usage.CostUSD)
		sections = append(sections, s)
	}
	return sections
}

// CLIRunner is the production Runner that shells out to the claude binary.
type CLIRunner struct{}

// Run executes the claude CLI with the given options.
func (r *CLIRunner) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	args := []string{
		"--print",
		"--verbose",
		"--model", opts.Model,
		"--output-format", "stream-json",
		"--max-turns", fmt.Sprintf("%d", opts.MaxTurns),
	}
	if opts.AllowedTools != "" {
		args = append(args, "--allowedTools", opts.AllowedTools)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}

	if opts.Prompt != "" && opts.PromptFile != "" {
		return nil, fmt.Errorf("Prompt and PromptFile are mutually exclusive")
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = buildEnv(opts.ContextVars)
	cmd.Stderr = os.Stderr

	// Set up stdin: direct prompt text, prompt file, or retry prompt
	if opts.Resume != "" && opts.RetryPrompt != "" {
		cmd.Stdin = strings.NewReader(opts.RetryPrompt)
	} else if opts.Prompt != "" {
		cmd.Stdin = strings.NewReader(opts.Prompt)
	} else if opts.PromptFile != "" {
		f, err := os.Open(opts.PromptFile)
		if err != nil {
			return nil, fmt.Errorf("opening prompt file: %w", err)
		}
		defer f.Close()
		cmd.Stdin = f
	}

	// Capture stdout to output file, optionally teeing to a stream
	// logger for real-time output.
	outFile, err := os.Create(opts.OutputFile)
	if err != nil {
		return nil, fmt.Errorf("creating output file: %w", err)
	}
	defer outFile.Close()

	logLevel := opts.LogLevel
	if logLevel == "" {
		logLevel = "error"
	}
	if logLevel == "error" {
		cmd.Stdout = outFile
	} else {
		logger := &streamLogger{level: logLevel}
		cmd.Stdout = io.MultiWriter(outFile, logger)
	}

	var result Result
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return &result, fmt.Errorf("running claude CLI: %w", err)
		}
	}

	// Parse the output. Always return a non-nil Result so callers can
	// unconditionally access usage and session ID.
	parsed, parseErr := parseOutput(opts.OutputFile)
	if parseErr != nil {
		return &result, fmt.Errorf("parsing claude output: %w", parseErr)
	}
	parsed.ExitCode = result.ExitCode
	if parsed.ResultText == "" {
		return parsed, fmt.Errorf("claude produced empty result (exit code %d)", parsed.ExitCode)
	}
	return parsed, nil
}

// ExtractResult extracts the result text from Claude JSON output and checks
// for the expected marker. Returns the result text and whether the marker
// was found with a positive outcome (PROCEED/SUCCESS).
func ExtractResult(outputFile, markerPrefix string) (text string, positive bool, err error) {
	text, err = extractResultText(outputFile)
	if err != nil {
		return "", false, err
	}

	// Anchor to the last occurrence of the marker prefix so an early echo
	// (e.g. Claude repeating the prompt) doesn't cause a false match.
	line, ok := lastLineContaining(text, markerPrefix)
	if !ok {
		return text, false, fmt.Errorf("no valid %s marker found in output", markerPrefix)
	}
	if strings.Contains(line, "SUCCESS") || strings.Contains(line, "PROCEED") {
		return text, true, nil
	}
	if strings.Contains(line, "FAILED") || strings.Contains(line, "SKIP") {
		return text, false, nil
	}
	return text, false, fmt.Errorf("no valid %s marker found in output", markerPrefix)
}

// claudeOutput represents the result event from Claude CLI output.
// In stream-json mode this is the last NDJSON line with type "result".
// In json mode (used by test mocks) it is the entire file.
type claudeOutput struct {
	Type              string          `json:"type"`
	Result            string          `json:"result"`
	SessionID         string          `json:"session_id"`
	TotalCostUSD      float64         `json:"total_cost_usd"`
	Usage             json.RawMessage `json:"usage"`
	PermissionDenials json.RawMessage `json:"permission_denials"`
}

type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// parseOutput extracts the result from Claude CLI output. Handles both
// single-JSON (test mocks) and NDJSON stream-json format by scanning
// for the last line with type "result".
func parseOutput(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Find the last result event. For single-JSON files this is the
	// only line; for NDJSON it is the final result event in the stream.
	var resultLine []byte
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		// Quick pre-check to avoid parsing every line.
		if bytes.Contains(line, []byte(`"result"`)) {
			var peek struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(line, &peek) == nil && peek.Type == "result" {
				resultLine = append([]byte(nil), line...)
			}
		}
	}
	if resultLine == nil {
		return nil, fmt.Errorf("no result event found in output")
	}

	var out claudeOutput
	if err := json.Unmarshal(resultLine, &out); err != nil {
		return nil, fmt.Errorf("parsing result event: %w", err)
	}

	usage := Usage{CostUSD: out.TotalCostUSD}
	if out.Usage != nil {
		var u claudeUsage
		if err := json.Unmarshal(out.Usage, &u); err == nil {
			usage.InputTokens = u.InputTokens
			usage.OutputTokens = u.OutputTokens
			usage.CacheCreationInputTokens = u.CacheCreationInputTokens
			usage.CacheReadInputTokens = u.CacheReadInputTokens
		}
	}

	var denialCount int
	if out.PermissionDenials != nil {
		var denials []json.RawMessage
		if json.Unmarshal(out.PermissionDenials, &denials) == nil {
			denialCount = len(denials)
		}
	}

	return &Result{
		ResultText:        out.Result,
		SessionID:         out.SessionID,
		Usage:             usage,
		PermissionDenials: denialCount,
	}, nil
}

// lastLineContaining returns the line containing the last occurrence of
// substr in text, or ("", false) if substr is not found.
func lastLineContaining(text, substr string) (string, bool) {
	idx := strings.LastIndex(text, substr)
	if idx < 0 {
		return "", false
	}
	line := text[idx:]
	if nl := strings.IndexByte(line, '\n'); nl >= 0 {
		line = line[:nl]
	}
	return line, true
}

func extractResultText(path string) (string, error) {
	result, err := parseOutput(path)
	if err != nil {
		return "", err
	}
	if result.ResultText == "" {
		return "", fmt.Errorf("empty result text")
	}
	return result.ResultText, nil
}

// streamLogger is an io.Writer that parses NDJSON lines from the Claude CLI's
// stream-json output and logs formatted events via the action package
// for real-time visibility in GitHub Actions step logs.
type streamLogger struct {
	level string // "info" or "debug"
	buf   []byte // incomplete line buffer
}

// Write buffers input and processes each complete NDJSON line.
func (s *streamLogger) Write(p []byte) (int, error) {
	s.buf = append(s.buf, p...)
	for {
		idx := bytes.IndexByte(s.buf, '\n')
		if idx < 0 {
			break
		}
		line := s.buf[:idx]
		s.buf = s.buf[idx+1:]
		s.processLine(line)
	}
	return len(p), nil
}

// contentBlock represents a single content block inside a Claude CLI
// assistant or user message.
type contentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name,omitempty"`
	Text  string          `json:"text,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

func (s *streamLogger) processLine(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}

	var event struct {
		Type    string `json:"type"`
		Message struct {
			Content []contentBlock `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return
	}

	switch event.Type {
	case "assistant":
		if s.level == "debug" {
			for _, block := range event.Message.Content {
				switch block.Type {
				case "tool_use":
					action.LogInfo(fmt.Sprintf("[tool] %s", block.Name))
					if len(block.Input) > 0 {
						action.LogInfo(fmt.Sprintf("  %s", string(block.Input)))
					}
				case "text":
					if block.Text != "" {
						action.LogInfo(block.Text)
					}
				}
			}
		}
	case "result":
		var pretty bytes.Buffer
		if json.Indent(&pretty, line, "", "  ") == nil {
			action.LogInfo(pretty.String())
		} else {
			action.LogInfo(string(line))
		}
	}
}

// buildEnv constructs an explicit environment for the Claude CLI subprocess.
// Only baseline vars and caller-specified context vars are included, so
// secrets and other sensitive env vars are not leaked to Claude.
func buildEnv(contextVars []string) []string {
	vars := make(map[string]bool, len(BaselineEnvVars)+len(contextVars))
	for _, k := range BaselineEnvVars {
		vars[k] = true
	}
	for _, k := range contextVars {
		vars[k] = true
	}

	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if vars[key] {
			env = append(env, entry)
		}
	}
	return env
}

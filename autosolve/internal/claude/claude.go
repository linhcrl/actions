// Package claude provides an interface for invoking the Claude CLI and
// parsing its JSON output.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	PromptFile   string // path to prompt file (used as stdin on first attempt)
	Resume       string // session ID for --resume
	RetryPrompt  string // prompt text for retry attempts (used as stdin with --resume)
	OutputFile   string // path to write JSON output
}

// Result holds parsed Claude CLI output.
type Result struct {
	ResultText string
	SessionID  string
	ExitCode   int
	Usage      Usage
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
func (t *UsageTracker) Save() {
	if err := os.WriteFile(UsageSummaryPath(), []byte(t.FormatSummary()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write usage summary: %v\n", err)
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
		"--model", opts.Model,
		"--output-format", "json",
		"--max-turns", fmt.Sprintf("%d", opts.MaxTurns),
	}
	if opts.AllowedTools != "" {
		args = append(args, "--allowedTools", opts.AllowedTools)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stderr = os.Stderr

	// Set up stdin: either the prompt file or the retry prompt text
	if opts.Resume != "" && opts.RetryPrompt != "" {
		cmd.Stdin = strings.NewReader(opts.RetryPrompt)
	} else if opts.PromptFile != "" {
		f, err := os.Open(opts.PromptFile)
		if err != nil {
			return nil, fmt.Errorf("opening prompt file: %w", err)
		}
		defer f.Close()
		cmd.Stdin = f
	}

	// Capture stdout to output file
	outFile, err := os.Create(opts.OutputFile)
	if err != nil {
		return nil, fmt.Errorf("creating output file: %w", err)
	}
	defer outFile.Close()
	cmd.Stdout = outFile

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("running claude CLI: %w", err)
		}
	}

	// Parse the output
	result, err := parseOutput(opts.OutputFile)
	if err != nil {
		return &Result{ExitCode: exitCode}, nil
	}
	result.ExitCode = exitCode
	return result, nil
}

// ExtractResult extracts the result text from Claude JSON output and checks
// for the expected marker. Returns the result text and whether the marker
// was found with a positive outcome (PROCEED/SUCCESS).
func ExtractResult(outputFile, markerPrefix string) (text string, positive bool, err error) {
	text, err = extractResultText(outputFile)
	if err != nil {
		return "", false, err
	}

	if strings.Contains(text, markerPrefix+" - SUCCESS") || strings.Contains(text, markerPrefix+" - PROCEED") {
		return text, true, nil
	}
	if strings.Contains(text, markerPrefix+" - FAILED") || strings.Contains(text, markerPrefix+" - SKIP") {
		return text, false, nil
	}
	return text, false, fmt.Errorf("no valid %s marker found in output", markerPrefix)
}

// ExtractSessionID extracts the session ID from Claude JSON output.
func ExtractSessionID(outputFile string) string {
	result, err := parseOutput(outputFile)
	if err != nil {
		return ""
	}
	return result.SessionID
}

// claudeOutput represents the JSON structure from claude --print --output-format json.
type claudeOutput struct {
	Type         string          `json:"type"`
	Result       string          `json:"result"`
	SessionID    string          `json:"session_id"`
	TotalCostUSD float64         `json:"total_cost_usd"`
	Usage        json.RawMessage `json:"usage"`
}

type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func parseOutput(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var out claudeOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing claude output: %w", err)
	}

	if out.Type != "result" {
		return nil, fmt.Errorf("unexpected output type: %s", out.Type)
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

	return &Result{
		ResultText: out.Result,
		SessionID:  out.SessionID,
		Usage:      usage,
	}, nil
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

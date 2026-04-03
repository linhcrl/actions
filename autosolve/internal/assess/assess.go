// Package assess implements the assessment phase of autosolve.
package assess

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/actions/autosolve/internal/action"
	"github.com/cockroachdb/actions/autosolve/internal/claude"
	"github.com/cockroachdb/actions/autosolve/internal/config"
	"github.com/cockroachdb/actions/autosolve/internal/prompt"
)

// Run executes the assessment phase.
func Run(ctx context.Context, cfg *config.Config, runner claude.Runner, tmpDir string) error {
	// Build prompt
	promptFile, err := prompt.Build(cfg, tmpDir)
	if err != nil {
		return fmt.Errorf("building prompt: %w", err)
	}

	action.LogInfo(fmt.Sprintf("Running assessment with model: %s", cfg.Model))

	outputFile := filepath.Join(tmpDir, "assessment.json")

	var tracker claude.UsageTracker

	// Invoke Claude in read-only mode
	result, err := runner.Run(ctx, claude.RunOptions{
		Model:        cfg.Model,
		AllowedTools: "Read,Grep,Glob",
		MaxTurns:     30,
		PromptFile:   promptFile,
		OutputFile:   outputFile,
	})
	tracker.Record("assess", result.Usage)
	tracker.Save()
	action.LogInfo(fmt.Sprintf("Assessment usage: input=%d output=%d cost=$%.4f",
		result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.CostUSD))
	if err != nil {
		return fmt.Errorf("running claude: %w", err)
	}

	// Extract result
	resultText, positive, err := claude.ExtractResult(outputFile, "ASSESSMENT_RESULT")
	action.SaveLogArtifact(outputFile, "assessment.json")
	if err != nil {
		action.LogError("No assessment result found in Claude output")
		action.SetOutput("assessment", "ERROR")
		return fmt.Errorf("no assessment result found in Claude output")
	}

	action.LogInfo(resultText)

	var assessment string
	if positive {
		assessment = "PROCEED"
		action.LogNotice("Assessment: PROCEED")
	} else if strings.Contains(resultText, "ASSESSMENT_RESULT - SKIP") {
		assessment = "SKIP"
		action.LogNotice("Assessment: SKIP")
	} else {
		action.LogError("Assessment result did not contain a valid PROCEED or SKIP marker")
		action.SetOutput("assessment", "ERROR")
		return fmt.Errorf("invalid assessment result")
	}

	// Write outputs
	summary := extractSummary(resultText, "ASSESSMENT_RESULT")
	summary = action.TruncateOutput(200, summary)

	action.SetOutput("assessment", assessment)
	action.SetOutputMultiline("summary", summary)
	action.SetOutputMultiline("result", resultText)

	action.WriteStepSummary(formatStepSummary(assessment, summary))

	return nil
}

func extractSummary(resultText, marker string) string {
	var lines []string
	for _, line := range strings.Split(resultText, "\n") {
		if !strings.HasPrefix(line, marker) {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func formatStepSummary(assessment, summary string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Autosolve Assessment\n**Result:** %s\n", assessment)
	if summary != "" {
		fmt.Fprintf(&b, "### Summary\n%s\n", summary)
	}
	return b.String()
}

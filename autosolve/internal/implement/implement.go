// Package implement orchestrates the implementation phase of autosolve,
// including retry logic, security checks, and PR creation.
package implement

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/actions/autosolve/internal/action"
	"github.com/cockroachdb/actions/autosolve/internal/claude"
	"github.com/cockroachdb/actions/autosolve/internal/config"
	"github.com/cockroachdb/actions/autosolve/internal/git"
	"github.com/cockroachdb/actions/autosolve/internal/github"
	"github.com/cockroachdb/actions/autosolve/internal/prompt"
	"github.com/cockroachdb/actions/autosolve/internal/security"
)

const (
	retryPrompt = "The previous attempt did not succeed. Please review what went wrong, try a different approach if needed, and attempt the fix again. Remember to end your response with IMPLEMENTATION_RESULT - SUCCESS or IMPLEMENTATION_RESULT - FAILED."

	// maxCommitSubjectLen is the maximum length for a git commit subject line.
	maxCommitSubjectLen = 72
)

// RetryDelay is the pause between retry attempts. Exported for testing.
var RetryDelay = 10 * time.Second

// Run executes the implementation phase.
func Run(
	ctx context.Context,
	cfg *config.Config,
	runner claude.Runner,
	ghClient github.Client,
	gitClient git.Client,
	tmpDir string,
) error {
	// Warn if the repo is missing recommended .gitignore patterns
	security.CheckGitignore(action.LogWarning)

	// Build prompt
	promptFile, err := prompt.Build(cfg, tmpDir)
	if err != nil {
		return fmt.Errorf("building prompt: %w", err)
	}

	action.LogInfo(fmt.Sprintf("Running implementation with model: %s (max retries: %d)", cfg.Model, cfg.MaxRetries))

	outputFile := filepath.Join(tmpDir, "implementation.json")
	resultFile := filepath.Join(tmpDir, "implementation_result.txt")

	var (
		sessionID  string
		implStatus = "FAILED"
		resultText string
		tracker    claude.UsageTracker
	)

	// Retry loop
	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		action.LogInfo(fmt.Sprintf("--- Attempt %d of %d ---", attempt, cfg.MaxRetries))

		opts := claude.RunOptions{
			Model:        cfg.Model,
			AllowedTools: cfg.AllowedTools,
			MaxTurns:     200,
			OutputFile:   outputFile,
		}

		if attempt == 1 {
			opts.PromptFile = promptFile
		} else {
			if sessionID == "" {
				action.LogWarning("No session ID from previous attempt; restarting with original prompt")
				opts.PromptFile = promptFile
			} else {
				opts.Resume = sessionID
				opts.RetryPrompt = retryPrompt
			}
		}

		result, err := runner.Run(ctx, opts)
		if err != nil {
			return fmt.Errorf("running claude (attempt %d): %w", attempt, err)
		}
		section := fmt.Sprintf("implement (attempt %d)", attempt)
		tracker.Record(section, result.Usage)
		action.LogInfo(fmt.Sprintf("Attempt %d usage: input=%d output=%d cost=$%.4f",
			attempt, result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.CostUSD))
		if result.ExitCode != 0 {
			action.LogWarning(fmt.Sprintf("Claude CLI exited with code %d on attempt %d", result.ExitCode, attempt))
		}

		// Extract result
		var positive bool
		resultText, positive, err = claude.ExtractResult(outputFile, "IMPLEMENTATION_RESULT")
		action.SaveLogArtifact(outputFile, fmt.Sprintf("implementation_attempt_%d.json", attempt))
		if err != nil || resultText == "" {
			action.LogWarning(fmt.Sprintf("No result text extracted from Claude output on attempt %d — see uploaded artifacts for raw output", attempt))
		} else {
			action.LogInfo(fmt.Sprintf("Claude result (attempt %d):", attempt))
			action.LogInfo(resultText)
		}

		// Save session ID for retry
		sessionID = claude.ExtractSessionID(outputFile)

		if positive {
			action.LogNotice(fmt.Sprintf("Implementation succeeded on attempt %d", attempt))
			implStatus = "SUCCESS"
			if err := os.WriteFile(resultFile, []byte(resultText), 0644); err != nil {
				action.LogWarning(fmt.Sprintf("Failed to write result file: %v", err))
			}
			break
		}

		action.LogWarning(fmt.Sprintf("Attempt %d did not succeed", attempt))
		if resultText != "" {
			if err := os.WriteFile(resultFile, []byte(resultText), 0644); err != nil {
				action.LogWarning(fmt.Sprintf("Failed to write result file: %v", err))
			}
		}

		if attempt < cfg.MaxRetries {
			action.LogInfo(fmt.Sprintf("Waiting %s before retry...", RetryDelay))
			timer := time.NewTimer(RetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}

	// Security check
	if implStatus == "SUCCESS" {
		violations, err := security.Check(gitClient, cfg.BlockedPaths)
		if err != nil {
			return fmt.Errorf("security check: %w", err)
		}
		if len(violations) > 0 {
			for _, v := range violations {
				action.LogWarning(v)
			}
			action.LogWarning("Security check failed: violations found in staged changes")
			return writeOutputs("FAILED", "", "", resultText, &tracker)
		}
		action.LogNotice("Security check passed")
	}

	// PR creation
	var prURL, branchName string
	if implStatus == "SUCCESS" && cfg.CreatePR {
		var err error
		prURL, branchName, err = pushAndPR(ctx, cfg, runner, ghClient, gitClient, tmpDir, resultText, &tracker)
		if err != nil {
			action.LogWarning(fmt.Sprintf("PR creation failed: %v", err))
			return writeOutputs("FAILED", "", "", resultText, &tracker)
		}
	}

	status := "FAILED"
	if implStatus == "SUCCESS" {
		if cfg.CreatePR {
			if prURL != "" {
				status = "SUCCESS"
			}
		} else {
			status = "SUCCESS"
		}
	}

	return writeOutputs(status, prURL, branchName, resultText, &tracker)
}

// Cleanup removes temporary state. The error is ignored because the
// fork remote may not have been configured yet if the run failed early.
func Cleanup(gitClient git.Client) {
	_, _ = gitClient.Remote("remove", "fork")
}

func pushAndPR(
	ctx context.Context,
	cfg *config.Config,
	runner claude.Runner,
	ghClient github.Client,
	gitClient git.Client,
	tmpDir, resultText string,
	tracker *claude.UsageTracker,
) (prURL, branchName string, err error) {
	// Default base branch
	baseBranch := cfg.PRBaseBranch
	if baseBranch == "" {
		ref, err := gitClient.SymbolicRef("refs/remotes/origin/HEAD")
		if err != nil {
			baseBranch = "main"
		} else {
			baseBranch = strings.TrimPrefix(ref, "refs/remotes/origin/")
		}
	}

	// Configure git identity
	if err := gitClient.Config("user.name", cfg.GitUserName); err != nil {
		return "", "", fmt.Errorf("setting git user.name: %w", err)
	}
	if err := gitClient.Config("user.email", cfg.GitUserEmail); err != nil {
		return "", "", fmt.Errorf("setting git user.email: %w", err)
	}

	// Set fork credentials and GIT_ASKPASS for the git push subprocess
	// only, so the token is never in the broader process environment or
	// written to disk.
	if cliClient, ok := gitClient.(*git.CLIClient); ok {
		askpass := filepath.Join(os.Getenv("SCRIPTS_DIR"), "git-askpass.sh")
		cliClient.PushEnv = []string{
			"GIT_ASKPASS=" + askpass,
			"GIT_FORK_USER=" + cfg.ForkOwner,
			"GIT_FORK_PASSWORD=" + cfg.ForkPushToken,
			"GIT_TERMINAL_PROMPT=0",
		}
	}

	forkURL := fmt.Sprintf("https://github.com/%s/%s.git", cfg.ForkOwner, cfg.ForkRepo)

	// Check if fork remote exists (exact line match to avoid matching e.g. "forked")
	remotes, _ := gitClient.Remote()
	hasFork := false
	for _, line := range strings.Split(remotes, "\n") {
		if strings.TrimSpace(line) == "fork" {
			hasFork = true
			break
		}
	}
	if hasFork {
		_, _ = gitClient.Remote("set-url", "fork", forkURL)
	} else {
		_, _ = gitClient.Remote("add", "fork", forkURL)
	}

	// Create branch
	suffix := cfg.BranchSuffix
	if suffix == "" {
		suffix = time.Now().Format("20060102-150405")
	}
	branchName = cfg.BranchPrefix + suffix

	if err := gitClient.Checkout("-b", branchName); err != nil {
		return "", "", fmt.Errorf("creating branch: %w", err)
	}

	// Read and remove Claude-generated metadata files
	commitSubject, commitBody := readCommitMessage()
	copyPRBody(tmpDir)

	// Stage only files that appear in the working tree diff (unstaged,
	// staged, and untracked). This avoids blindly staging credential files
	// or other artifacts dropped by action steps (e.g., gha-creds-*.json).
	changedFiles, err := git.ChangedFiles(gitClient)
	if err != nil {
		return "", "", fmt.Errorf("listing changed files: %w", err)
	}
	for _, f := range changedFiles {
		if err := gitClient.Add(f); err != nil {
			action.LogWarning(fmt.Sprintf("Failed to stage %s: %v", f, err))
		}
	}

	// Run security check on final staged changeset
	violations, err := security.Check(gitClient, cfg.BlockedPaths)
	if err != nil {
		return "", "", fmt.Errorf("security check: %w", err)
	}
	if len(violations) > 0 {
		for _, v := range violations {
			action.LogWarning(v)
		}
		return "", "", fmt.Errorf("security check failed: %d violation(s) found", len(violations))
	}

	// Verify there are staged changes
	stagedFiles, err := gitClient.Diff("--cached", "--name-only")
	if err != nil {
		return "", "", fmt.Errorf("checking staged changes: %w", err)
	}
	if strings.TrimSpace(stagedFiles) == "" {
		return "", "", fmt.Errorf("no changes to commit")
	}

	// AI security review: have Claude scan the staged diff for sensitive content
	if err := aiSecurityReview(ctx, cfg, runner, gitClient, tmpDir, tracker); err != nil {
		return "", "", fmt.Errorf("AI security review failed: %w", err)
	}

	// Build commit message — normalize subject to first line, trimmed
	pullRequestTitle := cfg.PullRequestTitle
	if pullRequestTitle == "" && commitSubject != "" {
		pullRequestTitle = commitSubject
	}
	if pullRequestTitle == "" {
		p := cfg.Prompt
		if p == "" {
			p = "automated change"
		}
		// Take only the first line
		if idx := strings.IndexAny(p, "\n\r"); idx >= 0 {
			p = p[:idx]
		}
		p = strings.TrimSpace(p)
		prefix := "autosolve: "
		maxLen := maxCommitSubjectLen - len(prefix)
		if len(p) > maxLen {
			p = p[:maxLen]
		}
		pullRequestTitle = prefix + p
	}

	commitMsg := pullRequestTitle
	if commitBody != "" {
		commitMsg += "\n\n" + commitBody
	}
	commitMsg += "\n\n" + cfg.CommitSignature

	if err := gitClient.Commit(commitMsg); err != nil {
		return "", "", fmt.Errorf("committing: %w", err)
	}

	// Force push to fork
	if err := gitClient.Push("--set-upstream", "fork", branchName, "--force"); err != nil {
		return "", "", fmt.Errorf("pushing to fork: %w", err)
	}

	// Build PR body
	prBody := buildPRBody(cfg, gitClient, tmpDir, baseBranch, branchName, resultText)

	// Ensure labels exist
	if cfg.PRLabels != "" {
		for _, label := range strings.Split(cfg.PRLabels, ",") {
			label = strings.TrimSpace(label)
			if label != "" {
				_ = ghClient.CreateLabel(ctx, cfg.GithubRepository, label)
			}
		}
	}

	// Create PR
	prURL, err = ghClient.CreatePR(ctx, github.PullRequestOptions{
		Repo:   cfg.GithubRepository,
		Head:   fmt.Sprintf("%s:%s", cfg.ForkOwner, branchName),
		Base:   baseBranch,
		Title:  pullRequestTitle,
		Body:   prBody,
		Labels: cfg.PRLabels,
		Draft:  cfg.PRDraft,
	})
	if err != nil {
		return "", "", fmt.Errorf("creating PR: %w", err)
	}

	action.LogNotice(fmt.Sprintf("PR created: %s", prURL))
	action.SetOutput("pr_url", prURL)
	action.SetOutput("branch_name", branchName)

	return prURL, branchName, nil
}

func readCommitMessage() (subject, body string) {
	data, err := os.ReadFile(".autosolve-commit-message")
	if err != nil {
		return "", ""
	}
	_ = os.Remove(".autosolve-commit-message")

	lines := strings.SplitN(string(data), "\n", 3)
	if len(lines) > 0 {
		subject = strings.TrimSpace(lines[0])
	}
	if len(lines) > 2 {
		body = strings.TrimSpace(lines[2])
	}
	return subject, body
}

func copyPRBody(tmpDir string) {
	data, err := os.ReadFile(".autosolve-pr-body")
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "autosolve-pr-body"), data, 0644); err != nil {
		action.LogWarning(fmt.Sprintf("Failed to copy PR body: %v", err))
	}
	_ = os.Remove(".autosolve-pr-body")
}

func buildPRBody(
	cfg *config.Config, gitClient git.Client, tmpDir, baseBranch, branchName, resultText string,
) string {
	var body string

	if cfg.PRBodyTemplate != "" {
		body = cfg.PRBodyTemplate
		summary := extractSummary(resultText, "IMPLEMENTATION_RESULT")
		summary = action.TruncateOutput(200, summary)
		body = strings.ReplaceAll(body, "{{SUMMARY}}", summary)
		body = strings.ReplaceAll(body, "{{BRANCH}}", branchName)
	} else if data, err := os.ReadFile(filepath.Join(tmpDir, "autosolve-pr-body")); err == nil {
		body = string(data)
	} else {
		out, err := gitClient.Log(fmt.Sprintf("%s..HEAD", baseBranch), "--format=%B")
		if err == nil {
			lines := strings.Split(out, "\n")
			if len(lines) > maxPRBodyLines {
				lines = lines[:maxPRBodyLines]
			}
			body = strings.Join(lines, "\n")
		}
	}

	body += "\n\n" + cfg.PRFooter
	return body
}

const maxPRBodyLines = 200

func extractSummary(resultText, marker string) string {
	var lines []string
	for _, line := range strings.Split(resultText, "\n") {
		if !strings.HasPrefix(line, marker) {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

const securityReviewFirstBatchPrompt = `You are a security reviewer. Your ONLY task is to review the following
changes for sensitive content that should NOT be committed to a repository.

Look for:
- Credentials, API keys, tokens, passwords (hardcoded or in config)
- Private keys, certificates, keystores
- Cloud provider credential files (e.g., gha-creds-*.json, service account keys)
- .env files or environment variable files containing secrets
- Database connection strings with embedded passwords
- Any other secrets or sensitive data

## All changed files in this commit

%s

## Diff to review (batch %d of %d)

%s

**OUTPUT REQUIREMENT**: End your response with exactly one of:
SECURITY_REVIEW - SUCCESS (if no sensitive content found)
SECURITY_REVIEW - FAILED (if any sensitive content found)

If you find sensitive content, list each finding before the FAIL marker.`

const securityReviewBatchPrompt = `You are a security reviewer. Your ONLY task is to review the following
diff for sensitive content that should NOT be committed to a repository.

Look for:
- Credentials, API keys, tokens, passwords (hardcoded or in config)
- Private keys, certificates, keystores
- Cloud provider credential files (e.g., gha-creds-*.json, service account keys)
- .env files or environment variable files containing secrets
- Database connection strings with embedded passwords
- Any other secrets or sensitive data

## Diff to review (batch %d of %d)

%s

**OUTPUT REQUIREMENT**: End your response with exactly one of:
SECURITY_REVIEW - SUCCESS (if no sensitive content found)
SECURITY_REVIEW - FAILED (if any sensitive content found)

If you find sensitive content, list each finding before the FAIL marker.`

// maxBatchSize is the approximate max character size for a batch of diffs
// sent to the AI security reviewer. Leaves room for the prompt template
// and file list.
const maxBatchSize = 80000

// generatedMarkers are strings that indicate a file is auto-generated.
var generatedMarkers = []string{
	"// Code generated",
	"# Code generated",
	"/* Code generated",
	"// DO NOT EDIT",
	"# DO NOT EDIT",
	"// auto-generated",
	"# auto-generated",
	"generated by",
}

// isGeneratedDiff checks whether a per-file diff contains a generated-file
// marker in its first few added lines.
func isGeneratedDiff(diff string) bool {
	lines := strings.Split(diff, "\n")
	checked := 0
	for _, line := range lines {
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		for _, marker := range generatedMarkers {
			if strings.Contains(strings.ToLower(line), strings.ToLower(marker)) {
				return true
			}
		}
		checked++
		if checked >= 10 {
			break
		}
	}
	return false
}

// aiSecurityReview runs a lightweight Claude invocation to scan the staged
// diff for sensitive content that pattern matching might miss. It reviews
// all changed file names and batches diffs to avoid truncation.
func aiSecurityReview(
	ctx context.Context,
	cfg *config.Config,
	runner claude.Runner,
	gitClient git.Client,
	tmpDir string,
	tracker *claude.UsageTracker,
) error {
	action.LogInfo("Running AI security review on staged changes...")

	// Get the list of staged files
	stagedOutput, err := gitClient.Diff("--cached", "--name-only")
	if err != nil {
		return fmt.Errorf("listing staged files: %w", err)
	}
	if stagedOutput == "" {
		return nil
	}

	var allFiles []string
	for _, f := range strings.Split(stagedOutput, "\n") {
		if f != "" {
			allFiles = append(allFiles, f)
		}
	}
	fileList := strings.Join(allFiles, "\n")

	// Collect per-file diffs, skipping generated files
	type fileDiff struct {
		name string
		diff string
	}
	var diffs []fileDiff
	for _, f := range allFiles {
		d, err := gitClient.Diff("--cached", "--", f)
		if err != nil {
			action.LogWarning(fmt.Sprintf("Could not get diff for %s, skipping", f))
			continue
		}
		if d == "" {
			continue
		}
		if isGeneratedDiff(d) {
			action.LogInfo(fmt.Sprintf("Skipping generated file: %s", f))
			continue
		}
		diffs = append(diffs, fileDiff{name: f, diff: d})
	}

	if len(diffs) == 0 {
		action.LogInfo("No non-generated diffs to review")
		return nil
	}

	// Build batches that fit within maxBatchSize
	var batches []string
	var current strings.Builder
	for _, fd := range diffs {
		// If adding this diff would exceed the limit, finalize the current batch
		if current.Len() > 0 && current.Len()+len(fd.diff) > maxBatchSize {
			batches = append(batches, current.String())
			current.Reset()
		}
		// If a single file exceeds the limit, include it as its own batch and warn
		if len(fd.diff) > maxBatchSize {
			action.LogWarning(fmt.Sprintf("File %s diff (%d bytes) exceeds batch size limit (%d bytes)", fd.name, len(fd.diff), maxBatchSize))
		}
		current.WriteString(fd.diff)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		batches = append(batches, current.String())
	}

	action.LogInfo(fmt.Sprintf("AI security review: %d file(s), %d batch(es)", len(diffs), len(batches)))

	// Review each batch
	for i, batch := range batches {
		batchNum := i + 1
		var promptText string
		if batchNum == 1 {
			promptText = fmt.Sprintf(securityReviewFirstBatchPrompt, fileList, batchNum, len(batches), batch)
		} else {
			promptText = fmt.Sprintf(securityReviewBatchPrompt, batchNum, len(batches), batch)
		}
		promptFile := filepath.Join(tmpDir, fmt.Sprintf("security_review_prompt_%d.txt", batchNum))
		if err := os.WriteFile(promptFile, []byte(promptText), 0644); err != nil {
			return fmt.Errorf("writing security review prompt: %w", err)
		}

		outputFile := filepath.Join(tmpDir, fmt.Sprintf("security_review_%d.json", batchNum))
		result, err := runner.Run(ctx, claude.RunOptions{
			Model:        cfg.SecurityReviewModel(),
			AllowedTools: "",
			MaxTurns:     1,
			PromptFile:   promptFile,
			OutputFile:   outputFile,
		})
		if err != nil {
			return fmt.Errorf("AI security review batch %d: %w", batchNum, err)
		}
		tracker.Record("security review", result.Usage)
		action.LogInfo(fmt.Sprintf("Security review batch %d usage: input=%d output=%d cost=$%.4f",
			batchNum, result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.CostUSD))

		resultText, positive, _ := claude.ExtractResult(outputFile, "SECURITY_REVIEW")
		action.SaveLogArtifact(outputFile, fmt.Sprintf("security_review_%d.json", batchNum))
		if result.ExitCode != 0 || resultText == "" {
			return fmt.Errorf("AI security review batch %d did not produce a result (exit code %d)", batchNum, result.ExitCode)
		}

		if !positive {
			action.LogWarning(fmt.Sprintf("AI security review found sensitive content in batch %d:", batchNum))
			action.LogWarning(resultText)
			_ = gitClient.ResetHead()
			return fmt.Errorf("sensitive content detected in staged changes")
		}

		action.LogInfo(fmt.Sprintf("AI security review batch %d/%d passed", batchNum, len(batches)))
	}

	action.LogNotice("AI security review passed")
	return nil
}

func writeOutputs(
	status, prURL, branchName, resultText string, tracker *claude.UsageTracker,
) error {
	summary := extractSummary(resultText, "IMPLEMENTATION_RESULT")
	summary = action.TruncateOutput(200, summary)

	action.SetOutput("status", status)
	action.SetOutput("pr_url", prURL)
	action.SetOutput("branch_name", branchName)
	action.SetOutputMultiline("summary", summary)
	action.SetOutputMultiline("result", resultText)

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Autosolve Implementation\n**Status:** %s\n", status)
	if prURL != "" {
		fmt.Fprintf(&sb, "**PR:** %s\n", prURL)
	}
	if branchName != "" {
		fmt.Fprintf(&sb, "**Branch:** `%s`\n", branchName)
	}
	if summary != "" {
		fmt.Fprintf(&sb, "### Summary\n%s\n", summary)
	}
	if tracker != nil {
		// Load usage from earlier steps (e.g. assess) so the table is combined
		tracker.Load()
		tracker.Save()
		total := tracker.Total()
		action.LogInfo(fmt.Sprintf("Total usage: input=%d output=%d cost=$%.4f",
			total.InputTokens, total.OutputTokens, total.CostUSD))
	}
	action.WriteStepSummary(sb.String())

	return nil
}

// Package github provides an interface for GitHub API interactions,
// with a production implementation that shells out to the gh CLI.
package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Client defines GitHub API interactions needed by autosolve.
type Client interface {
	CreateComment(ctx context.Context, repo string, issue int, body string) error
	RemoveLabel(ctx context.Context, repo string, issue int, label string) error
	CreatePR(ctx context.Context, opts PullRequestOptions) (string, error)
	CreateLabel(ctx context.Context, repo string, name string) error
	FindPRByLabel(ctx context.Context, repo string, label string) (string, error)
}

// PullRequestOptions configures PR creation.
type PullRequestOptions struct {
	Repo   string
	Head   string // e.g., "fork_owner:branch_name"
	Base   string
	Title  string
	Body   string
	Labels string // comma-separated
	Draft  bool
}

// GithubClient implements Client by shelling out to the gh CLI.
type GithubClient struct {
	Token string
}

func (c *GithubClient) CreateComment(
	ctx context.Context, repo string, issue int, body string,
) error {
	cmd := c.command(ctx, "issue", "comment", fmt.Sprintf("%d", issue),
		"--repo", repo,
		"--body", body)
	return cmd.Run()
}

func (c *GithubClient) RemoveLabel(
	ctx context.Context, repo string, issue int, label string,
) error {
	cmd := c.command(ctx, "issue", "edit", fmt.Sprintf("%d", issue),
		"--repo", repo,
		"--remove-label", label)
	// Best-effort: label may already be removed
	_ = cmd.Run()
	return nil
}

func (c *GithubClient) CreatePR(ctx context.Context, opts PullRequestOptions) (string, error) {
	args := []string{"pr", "create",
		"--repo", opts.Repo,
		"--head", opts.Head,
		"--base", opts.Base,
		"--title", opts.Title,
		"--body", opts.Body,
	}
	if opts.Labels != "" {
		args = append(args, "--label", opts.Labels)
	}
	if opts.Draft {
		args = append(args, "--draft")
	}

	cmd := c.command(ctx, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("creating PR: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *GithubClient) CreateLabel(ctx context.Context, repo string, name string) error {
	cmd := c.command(ctx, "label", "create", name,
		"--repo", repo,
		"--color", "6f42c1")
	// Best-effort: label may already exist
	_ = cmd.Run()
	return nil
}

func (c *GithubClient) FindPRByLabel(
	ctx context.Context, repo string, label string,
) (string, error) {
	cmd := c.command(ctx, "pr", "list",
		"--repo", repo,
		"--label", label,
		"--json", "url",
		"--jq", ".[0].url // empty")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("searching for PRs with label %q: %w", label, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *GithubClient) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("GH_TOKEN=%s", c.Token))
	cmd.Stderr = os.Stderr
	return cmd
}

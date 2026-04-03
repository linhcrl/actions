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
	CreatePR(ctx context.Context, opts PullRequestOptions) (string, error)
	CreateLabel(ctx context.Context, repo string, name string) error
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
	// Capture stderr so we can distinguish "already exists" from real errors.
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "already exists") {
			return nil
		}
		return fmt.Errorf("creating label %q: %s", name, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (c *GithubClient) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("GH_TOKEN=%s", c.Token))
	cmd.Stderr = os.Stderr
	return cmd
}

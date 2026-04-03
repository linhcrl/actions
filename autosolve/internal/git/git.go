// Package git abstracts git CLI operations behind an interface for testability.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Client defines git operations needed by autosolve.
type Client interface {
	Diff(args ...string) (string, error)
	LsFiles(args ...string) (string, error)
	Config(args ...string) error
	Remote(args ...string) (string, error)
	Checkout(args ...string) error
	Add(args ...string) error
	Commit(message string) error
	Push(args ...string) error
	Log(args ...string) (string, error)
	ResetHead() error
	SymbolicRef(ref string) (string, error)
}

// CLIClient implements Client by shelling out to the git binary.
// Extra env vars (e.g. for authentication) can be set via PushEnv;
// they are applied only to git push commands.
type CLIClient struct {
	PushEnv []string
}

func (c *CLIClient) Diff(args ...string) (string, error) {
	return c.output(append([]string{"diff"}, args...)...)
}

func (c *CLIClient) LsFiles(args ...string) (string, error) {
	return c.output(append([]string{"ls-files"}, args...)...)
}

func (c *CLIClient) Config(args ...string) error {
	return c.run(append([]string{"config"}, args...)...)
}

func (c *CLIClient) Remote(args ...string) (string, error) {
	return c.output(append([]string{"remote"}, args...)...)
}

func (c *CLIClient) Checkout(args ...string) error {
	return c.run(append([]string{"checkout"}, args...)...)
}

func (c *CLIClient) Add(args ...string) error {
	return c.run(append([]string{"add"}, args...)...)
}

func (c *CLIClient) Commit(message string) error {
	cmd := exec.Command("git", "commit", "--message", message)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *CLIClient) Push(args ...string) error {
	cmd := exec.Command("git", append([]string{"push"}, args...)...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if len(c.PushEnv) > 0 {
		cmd.Env = append(os.Environ(), c.PushEnv...)
	}
	return cmd.Run()
}

func (c *CLIClient) Log(args ...string) (string, error) {
	return c.output(append([]string{"log"}, args...)...)
}

func (c *CLIClient) ResetHead() error {
	cmd := exec.Command("git", "reset", "HEAD")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *CLIClient) SymbolicRef(ref string) (string, error) {
	return c.output("symbolic-ref", ref)
}

func (c *CLIClient) run(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *CLIClient) output(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ChangedFiles returns a deduplicated, sorted list of all changed files
// (unstaged, staged, and untracked) using the given git client.
func ChangedFiles(g Client) ([]string, error) {
	seen := make(map[string]bool)

	unstaged, err := g.Diff("--name-only")
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	addLines(seen, unstaged)

	staged, err := g.Diff("--name-only", "--cached")
	if err != nil {
		return nil, fmt.Errorf("git diff --cached: %w", err)
	}
	addLines(seen, staged)

	untracked, err := g.LsFiles("--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	addLines(seen, untracked)

	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, nil
}

func addLines(seen map[string]bool, output string) {
	for _, line := range strings.Split(output, "\n") {
		if line != "" {
			seen[line] = true
		}
	}
}

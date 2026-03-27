package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/cockroachdb/actions/autosolve/internal/action"
	"github.com/cockroachdb/actions/autosolve/internal/assess"
	"github.com/cockroachdb/actions/autosolve/internal/claude"
	"github.com/cockroachdb/actions/autosolve/internal/config"
	"github.com/cockroachdb/actions/autosolve/internal/git"
	"github.com/cockroachdb/actions/autosolve/internal/github"
	"github.com/cockroachdb/actions/autosolve/internal/implement"
)

// BuildSHA is set at build time via -ldflags.
var BuildSHA = "dev"

const usage = `Usage: autosolve <command>

Commands:
  assess      Run assessment phase
  implement   Run implementation phase
  version     Print the git SHA this binary was built from
`

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if len(os.Args) < 2 {
		fatalf(usage)
	}

	var err error
	switch os.Args[1] {
	case "assess":
		err = runAssess(ctx)
	case "implement":
		err = runImplement(ctx)
	case "version":
		fmt.Println(BuildSHA)
		return
	default:
		fatalf("unknown command: %s\n\n%s", os.Args[1], usage)
	}

	if err != nil {
		action.LogError(err.Error())
		os.Exit(1)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func runAssess(ctx context.Context) error {
	cfg, err := config.LoadAssessConfig()
	if err != nil {
		return err
	}
	if err := config.ValidateAuth(); err != nil {
		return err
	}
	tmpDir, err := ensureTmpDir()
	if err != nil {
		return err
	}
	return assess.Run(ctx, cfg, &claude.CLIRunner{}, tmpDir)
}

func runImplement(ctx context.Context) error {
	cfg, err := config.LoadImplementConfig()
	if err != nil {
		return err
	}
	if err := config.ValidateAuth(); err != nil {
		return err
	}
	tmpDir, err := ensureTmpDir()
	if err != nil {
		return err
	}

	gitClient := &git.CLIClient{}
	defer implement.Cleanup(gitClient)

	ghClient := &github.GithubClient{Token: cfg.PRCreateToken}
	return implement.Run(ctx, cfg, &claude.CLIRunner{}, ghClient, gitClient, tmpDir)
}

func ensureTmpDir() (string, error) {
	dir := os.Getenv("AUTOSOLVE_TMPDIR")
	if dir != "" {
		return dir, nil
	}
	dir, err := os.MkdirTemp("", "autosolve_*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	os.Setenv("AUTOSOLVE_TMPDIR", dir)
	return dir, nil
}

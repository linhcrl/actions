package security

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/cockroachdb/actions/autosolve/internal/git"
)

func TestCheck_NoChanges(t *testing.T) {
	dir := setupGitRepo(t)
	chdir(t, dir)

	violations, err := Check(&git.CLIClient{}, []string{".github/workflows/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("expected no violations, got: %v", violations)
	}
}

func TestCheck_AllowedChange(t *testing.T) {
	dir := setupGitRepo(t)
	chdir(t, dir)

	// Create an allowed file
	if err := os.MkdirAll("src", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("src/main.go", []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	violations, err := Check(&git.CLIClient{}, []string{".github/workflows/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("expected no violations for allowed file, got: %v", violations)
	}
}

func TestCheck_BlockedChange(t *testing.T) {
	dir := setupGitRepo(t)
	chdir(t, dir)

	// Create a blocked file
	if err := os.MkdirAll(".github/workflows", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".github/workflows/ci.yml", []byte("name: ci"), 0644); err != nil {
		t.Fatal(err)
	}

	violations, err := Check(&git.CLIClient{}, []string{".github/workflows/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) == 0 {
		t.Error("expected violations for blocked path")
	}
}

func TestCheck_MultipleBlockedPaths(t *testing.T) {
	dir := setupGitRepo(t)
	chdir(t, dir)

	if err := os.MkdirAll("secrets", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("secrets/key.txt", []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	violations, err := Check(&git.CLIClient{}, []string{".github/workflows/", "secrets/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) == 0 {
		t.Error("expected violations for secrets/ path")
	}
}

func TestCheck_StagedBlockedChange(t *testing.T) {
	dir := setupGitRepo(t)
	chdir(t, dir)

	if err := os.MkdirAll(".github/workflows", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".github/workflows/ci.yml", []byte("name: ci"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "add", ".github/workflows/ci.yml").CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}

	violations, err := Check(&git.CLIClient{}, []string{".github/workflows/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) == 0 {
		t.Error("expected violations for staged blocked file")
	}
}

func TestCheck_SensitiveCredentialFile(t *testing.T) {
	dir := setupGitRepo(t)
	chdir(t, dir)

	if err := os.WriteFile("gha-creds-abc123.json", []byte(`{"type":"authorized_user"}`), 0644); err != nil {
		t.Fatal(err)
	}

	violations, err := Check(&git.CLIClient{}, []string{".github/workflows/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) == 0 {
		t.Error("expected violation for credential file")
	}
}

func TestCheck_SensitiveKeyFile(t *testing.T) {
	dir := setupGitRepo(t)
	chdir(t, dir)

	if err := os.WriteFile("server.pem", []byte("-----BEGIN PRIVATE KEY-----"), 0644); err != nil {
		t.Fatal(err)
	}

	violations, err := Check(&git.CLIClient{}, []string{".github/workflows/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) == 0 {
		t.Error("expected violation for .pem file")
	}
}

func TestCheck_SensitiveEnvFile(t *testing.T) {
	dir := setupGitRepo(t)
	chdir(t, dir)

	if err := os.WriteFile(".env", []byte("SECRET=foo"), 0644); err != nil {
		t.Fatal(err)
	}

	violations, err := Check(&git.CLIClient{}, []string{".github/workflows/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) == 0 {
		t.Error("expected violation for .env file")
	}
}

func TestCheckSensitiveFile(t *testing.T) {
	tests := []struct {
		file    string
		wantHit bool
	}{
		{"gha-creds-abc123.json", true},
		{"credentials.json", true},
		{"service-account-key.json", true},
		{".env", true},
		{"server.pem", true},
		{"tls.key", true},
		{"keystore.p12", true},
		{"cert.pfx", true},
		{"app.keystore", true},
		{"main.go", false},
		{"README.md", false},
		{"config.yaml", false},
	}

	for _, tt := range tests {
		v := checkSensitiveFile(tt.file)
		if tt.wantHit && v == "" {
			t.Errorf("expected violation for %q, got none", tt.file)
		}
		if !tt.wantHit && v != "" {
			t.Errorf("unexpected violation for %q: %s", tt.file, v)
		}
	}
}

func TestCheck_SymlinkToBlockedPath(t *testing.T) {
	dir := setupGitRepo(t)
	chdir(t, dir)

	// Create a blocked directory with a file
	if err := os.MkdirAll(".github/workflows", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".github/workflows/ci.yml", []byte("name: ci"), 0644); err != nil {
		t.Fatal(err)
	}
	// Commit the blocked file so it's not itself a change
	if out, err := exec.Command("git", "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "commit", "-m", "add workflow").CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	// Create a symlink from an allowed path into the blocked directory
	if err := os.Symlink(".github/workflows/ci.yml", "sneaky-link.yml"); err != nil {
		t.Fatal(err)
	}

	violations, err := Check(&git.CLIClient{}, []string{".github/workflows/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) == 0 {
		t.Error("expected violation for symlink pointing into blocked path")
	}
	found := false
	for _, v := range violations {
		if strings.Contains(v, "symlink to blocked path") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected symlink violation, got: %v", violations)
	}
}

func TestCheck_DeletedFileNoFalseViolation(t *testing.T) {
	dir := setupGitRepo(t)
	chdir(t, dir)

	// Create and commit a file, then delete it so it shows up in the diff
	// but EvalSymlinks returns ErrNotExist.
	if err := os.WriteFile("temp.go", []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "add", "temp.go").CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "commit", "-m", "add temp").CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
	if err := os.Remove("temp.go"); err != nil {
		t.Fatal(err)
	}

	violations, err := Check(&git.CLIClient{}, []string{".github/workflows/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("expected no violations for deleted file, got: %v", violations)
	}
}

func TestCheckGitignore_NoFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	var warnings []string
	CheckGitignore(func(msg string) { warnings = append(warnings, msg) })

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0], "No .gitignore found") {
		t.Errorf("unexpected warning: %s", warnings[0])
	}
}

func TestCheckGitignore_MissingPatterns(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if err := os.WriteFile(".gitignore", []byte("node_modules/\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var warnings []string
	CheckGitignore(func(msg string) { warnings = append(warnings, msg) })

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0], "missing recommended") {
		t.Errorf("unexpected warning: %s", warnings[0])
	}
}

func TestCheckGitignore_AllPresent(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	content := strings.Join([]string{
		"gha-creds-*.json", "*.pem", "*.key", "*.p12", "*.pfx",
		"*.keystore", "credentials.json", "service-account*.json",
	}, "\n") + "\n"
	if err := os.WriteFile(".gitignore", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	var warnings []string
	CheckGitignore(func(msg string) { warnings = append(warnings, msg) })

	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
}

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "--message", "initial"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v failed: %v\n%s", args, err, out)
		}
	}

	return dir
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

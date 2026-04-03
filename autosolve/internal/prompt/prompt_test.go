package prompt

import (
	"os"
	"strings"
	"testing"

	"github.com/cockroachdb/actions/autosolve/internal/config"
)

func TestBuild_Assessment(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Prompt:       "Fix the bug in foo.go",
		BlockedPaths: []string{".github/workflows/"},
		FooterType:   "assessment",
	}

	path, err := Build(cfg, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Check all sections present
	checks := []string{
		"system_instruction",
		"BLOCKED",
		".github/workflows/",
		"<task>",
		"Fix the bug in foo.go",
		"</task>",
		"ASSESSMENT_RESULT",
		"PROCEED",
		"SKIP",
	}
	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Errorf("expected %q in prompt, got:\n%s", c, content)
		}
	}
}

func TestBuild_Implementation(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Prompt:       "Add a new feature",
		BlockedPaths: []string{"secrets/"},
		FooterType:   "implementation",
	}

	path, err := Build(cfg, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "IMPLEMENTATION_RESULT") {
		t.Error("expected IMPLEMENTATION_RESULT in implementation prompt")
	}
	if !strings.Contains(content, "secrets/") {
		t.Error("expected blocked path in prompt")
	}
}

func TestBuild_WithSkillFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a skill file
	skillFile := tmpDir + "/skill.md"
	os.WriteFile(skillFile, []byte("Do the skill task"), 0644)

	cfg := &config.Config{
		Skill:        skillFile,
		BlockedPaths: []string{".github/workflows/"},
		FooterType:   "implementation",
	}

	path, err := Build(cfg, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "Do the skill task") {
		t.Error("expected skill content in prompt")
	}
}

func TestBuild_WithAdditionalInstructions(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Prompt:                 "Fix it",
		AdditionalInstructions: "Also run linter",
		BlockedPaths:           []string{".github/workflows/"},
		FooterType:             "implementation",
	}

	path, err := Build(cfg, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "Also run linter") {
		t.Error("expected additional instructions in prompt")
	}
}

func TestBuild_CustomAssessmentCriteria(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Prompt:             "Check this",
		BlockedPaths:       []string{".github/workflows/"},
		FooterType:         "assessment",
		AssessmentCriteria: "Custom criteria here",
	}

	path, err := Build(cfg, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "Custom criteria here") {
		t.Error("expected custom assessment criteria")
	}
	if strings.Contains(content, "PROCEED if:") {
		t.Error("should not contain default criteria when custom is set")
	}
}

func TestBuild_SkillFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Skill:        "/nonexistent/skill.md",
		BlockedPaths: []string{".github/workflows/"},
		FooterType:   "implementation",
	}

	_, err := Build(cfg, tmpDir)
	if err == nil {
		t.Error("expected error for missing skill file")
	}
}

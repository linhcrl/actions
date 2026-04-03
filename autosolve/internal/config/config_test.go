package config

import (
	"os"
	"testing"
)

func TestLoadAssessConfig_RequiresPromptOrSkill(t *testing.T) {
	clearInputEnv(t)
	_, err := LoadAssessConfig()
	if err == nil {
		t.Fatal("expected error when neither prompt nor skill is set")
	}
}

func TestLoadAssessConfig_AcceptsPrompt(t *testing.T) {
	clearInputEnv(t)
	t.Setenv("INPUT_PROMPT", "fix the bug")
	t.Setenv("INPUT_MODEL", "claude-opus-4-6")
	cfg, err := LoadAssessConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Prompt != "fix the bug" {
		t.Errorf("expected prompt 'fix the bug', got %q", cfg.Prompt)
	}
	if cfg.FooterType != "assessment" {
		t.Errorf("expected footer type 'assessment', got %q", cfg.FooterType)
	}
}

func TestLoadAssessConfig_AcceptsSkill(t *testing.T) {
	clearInputEnv(t)
	t.Setenv("INPUT_SKILL", "path/to/skill.md")
	t.Setenv("INPUT_MODEL", "claude-opus-4-6")
	cfg, err := LoadAssessConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Skill != "path/to/skill.md" {
		t.Errorf("expected skill path, got %q", cfg.Skill)
	}
}

func TestLoadImplementConfig_ValidatesPR(t *testing.T) {
	clearInputEnv(t)
	t.Setenv("INPUT_PROMPT", "fix it")
	t.Setenv("INPUT_MODEL", "claude-opus-4-6")
	t.Setenv("INPUT_CREATE_PR", "true")
	// Missing fork_owner, fork_repo, etc.
	_, err := LoadImplementConfig()
	if err == nil {
		t.Fatal("expected error when PR inputs are missing")
	}
}

func TestLoadImplementConfig_NoPRCreation(t *testing.T) {
	clearInputEnv(t)
	t.Setenv("INPUT_PROMPT", "fix it")
	t.Setenv("INPUT_MODEL", "claude-opus-4-6")
	t.Setenv("INPUT_CREATE_PR", "false")
	cfg, err := LoadImplementConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CreatePR {
		t.Error("expected CreatePR to be false")
	}
}

func TestLoadImplementConfig_Defaults(t *testing.T) {
	clearInputEnv(t)
	t.Setenv("INPUT_PROMPT", "fix it")
	t.Setenv("INPUT_MODEL", "claude-opus-4-6")
	t.Setenv("INPUT_CREATE_PR", "false")
	cfg, err := LoadImplementConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", cfg.MaxRetries)
	}
	if cfg.GitUserName != "autosolve[bot]" {
		t.Errorf("expected default git user name, got %q", cfg.GitUserName)
	}
}

func TestValidateAuth_APIKey(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	if err := ValidateAuth(); err != nil {
		t.Errorf("expected no error with API key, got: %v", err)
	}
}

func TestValidateAuth_Vertex(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "1")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "my-project")
	t.Setenv("CLOUD_ML_REGION", "us-central1")
	if err := ValidateAuth(); err != nil {
		t.Errorf("expected no error with Vertex, got: %v", err)
	}
}

func TestValidateAuth_VertexMissing(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "1")
	err := ValidateAuth()
	if err == nil {
		t.Fatal("expected error when Vertex config is incomplete")
	}
}

func TestValidateAuth_None(t *testing.T) {
	clearAuthEnv(t)
	err := ValidateAuth()
	if err == nil {
		t.Fatal("expected error when no auth configured")
	}
}

func TestParseBlockedPaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty defaults", "", []string{".github/workflows/"}},
		{"single", ".github/", []string{".github/"}},
		{"multiple", ".github/workflows/, secrets/, .env", []string{".github/workflows/", "secrets/", ".env"}},
		{"with whitespace", " foo/ , bar/ ", []string{"foo/", "bar/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBlockedPaths(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len mismatch: got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestEnvBool(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		def     bool
		want    bool
		wantErr bool
	}{
		{"empty uses default true", "", true, true, false},
		{"empty uses default false", "", false, false, false},
		{"lowercase true", "true", false, true, false},
		{"uppercase TRUE", "TRUE", false, true, false},
		{"mixed case True", "True", false, true, false},
		{"lowercase false", "false", true, false, false},
		{"uppercase FALSE", "FALSE", true, false, false},
		{"invalid value", "yes", false, false, true},
		{"numeric value", "1", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_BOOL"
			if tt.value == "" {
				os.Unsetenv(key)
			} else {
				t.Setenv(key, tt.value)
			}
			got, err := envBool(key, tt.def)
			if (err != nil) != tt.wantErr {
				t.Fatalf("envBool() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("envBool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func clearInputEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"INPUT_PROMPT", "INPUT_SKILL", "INPUT_MODEL",
		"INPUT_ADDITIONAL_INSTRUCTIONS", "INPUT_ASSESSMENT_CRITERIA",
		"INPUT_BLOCKED_PATHS", "INPUT_MAX_RETRIES", "INPUT_ALLOWED_TOOLS",
		"INPUT_CREATE_PR", "INPUT_FORK_OWNER", "INPUT_FORK_REPO",
		"INPUT_FORK_PUSH_TOKEN", "INPUT_PR_CREATE_TOKEN",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func clearAuthEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ANTHROPIC_API_KEY", "CLAUDE_CODE_USE_VERTEX",
		"ANTHROPIC_VERTEX_PROJECT_ID", "CLOUD_ML_REGION",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

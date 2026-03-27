package action

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetOutput(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "output")
	os.Setenv("GITHUB_OUTPUT", tmp)
	defer os.Unsetenv("GITHUB_OUTPUT")

	SetOutput("key1", "value1")
	SetOutput("key2", "value2")

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "key1=value1") {
		t.Errorf("expected key1=value1, got: %s", content)
	}
	if !strings.Contains(content, "key2=value2") {
		t.Errorf("expected key2=value2, got: %s", content)
	}
}

func TestSetOutputMultiline(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "output")
	os.Setenv("GITHUB_OUTPUT", tmp)
	defer os.Unsetenv("GITHUB_OUTPUT")

	SetOutputMultiline("body", "line1\nline2\nline3")

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "body<<GHEOF_") {
		t.Errorf("expected heredoc delimiter, got: %s", content)
	}
	if !strings.Contains(content, "line1\nline2\nline3") {
		t.Errorf("expected multiline content, got: %s", content)
	}
}

func TestSetOutput_NoFile(t *testing.T) {
	os.Setenv("GITHUB_OUTPUT", "")
	defer os.Unsetenv("GITHUB_OUTPUT")
	// Should not panic
	SetOutput("key", "value")
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		name     string
		max      int
		input    string
		wantTail string
	}{
		{"short", 5, "a\nb\nc", ""},
		{"exact", 3, "a\nb\nc", ""},
		{"truncated", 2, "a\nb\nc\nd", "[... truncated (4 lines total, showing first 2)]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateOutput(tt.max, tt.input)
			if tt.wantTail == "" {
				if result != tt.input {
					t.Errorf("expected unchanged output, got: %s", result)
				}
			} else {
				if !strings.HasSuffix(result, tt.wantTail) {
					t.Errorf("expected suffix %q, got: %s", tt.wantTail, result)
				}
			}
		})
	}
}

func TestWriteStepSummary(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "summary")
	os.Setenv("GITHUB_STEP_SUMMARY", tmp)
	defer os.Unsetenv("GITHUB_STEP_SUMMARY")

	WriteStepSummary("## Test\nContent here")

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## Test") {
		t.Errorf("expected markdown content, got: %s", data)
	}
}

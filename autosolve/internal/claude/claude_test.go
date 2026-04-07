package claude

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestExtractResult_Success(t *testing.T) {
	f := writeJSON(t, claudeOutput{
		Type:      "result",
		Result:    "Analysis complete.\n\nIMPLEMENTATION_RESULT - SUCCESS",
		SessionID: "sess-123",
	})

	text, positive, err := ExtractResult(f, "IMPLEMENTATION_RESULT")
	if err != nil {
		t.Fatal(err)
	}
	if !positive {
		t.Error("expected positive result")
	}
	if text == "" {
		t.Error("expected non-empty text")
	}
}

func TestExtractResult_Failed(t *testing.T) {
	f := writeJSON(t, claudeOutput{
		Type:   "result",
		Result: "Could not fix it.\n\nIMPLEMENTATION_RESULT - FAILED",
	})

	text, positive, err := ExtractResult(f, "IMPLEMENTATION_RESULT")
	if err != nil {
		t.Fatal(err)
	}
	if positive {
		t.Error("expected negative result")
	}
	if text == "" {
		t.Error("expected non-empty text")
	}
}

func TestExtractResult_Proceed(t *testing.T) {
	f := writeJSON(t, claudeOutput{
		Type:   "result",
		Result: "This looks good.\n\nASSESSMENT_RESULT - PROCEED",
	})

	_, positive, err := ExtractResult(f, "ASSESSMENT_RESULT")
	if err != nil {
		t.Fatal(err)
	}
	if !positive {
		t.Error("expected positive (PROCEED)")
	}
}

func TestExtractResult_Skip(t *testing.T) {
	f := writeJSON(t, claudeOutput{
		Type:   "result",
		Result: "Too complex.\n\nASSESSMENT_RESULT - SKIP",
	})

	_, positive, err := ExtractResult(f, "ASSESSMENT_RESULT")
	if err != nil {
		t.Fatal(err)
	}
	if positive {
		t.Error("expected negative (SKIP)")
	}
}

func TestExtractResult_NoMarker(t *testing.T) {
	f := writeJSON(t, claudeOutput{
		Type:   "result",
		Result: "Some random output without a marker",
	})

	_, _, err := ExtractResult(f, "IMPLEMENTATION_RESULT")
	if err == nil {
		t.Error("expected error when marker is missing")
	}
}

func TestExtractResult_EchoedMarkerUsesLast(t *testing.T) {
	// Claude may echo the prompt instructions containing the marker before
	// producing its actual result. The last occurrence should win.
	f := writeJSON(t, claudeOutput{
		Type: "result",
		Result: "You asked me to output IMPLEMENTATION_RESULT - SUCCESS when done.\n" +
			"However, the build is broken.\n\n" +
			"IMPLEMENTATION_RESULT - FAILED",
	})

	_, positive, err := ExtractResult(f, "IMPLEMENTATION_RESULT")
	if err != nil {
		t.Fatal(err)
	}
	if positive {
		t.Error("expected negative result; echoed SUCCESS marker should not win over final FAILED")
	}
}

func TestExtractResult_EmptyResult(t *testing.T) {
	f := writeJSON(t, claudeOutput{
		Type:   "result",
		Result: "",
	})

	_, _, err := ExtractResult(f, "IMPLEMENTATION_RESULT")
	if err == nil {
		t.Error("expected error for empty result")
	}
}

func TestExtractResult_FileNotFound(t *testing.T) {
	_, _, err := ExtractResult("/nonexistent/file.json", "IMPLEMENTATION_RESULT")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestUsageTracker_RoundTrip(t *testing.T) {
	var tracker UsageTracker
	tracker.Record("assess", Usage{
		InputTokens:              6,
		OutputTokens:             792,
		CacheCreationInputTokens: 100,
		CacheReadInputTokens:     50,
		CostUSD:                  0.2636,
	})
	tracker.Record("implement (attempt 1)", Usage{
		InputTokens:  1950,
		OutputTokens: 8796,
		CostUSD:      0.8105,
	})
	tracker.Record("security review", Usage{
		InputTokens:  3,
		OutputTokens: 49,
		CostUSD:      0.0383,
	})

	md := tracker.FormatSummary()
	parsed := ParseSummary(md)

	if len(parsed) != len(tracker.Sections) {
		t.Fatalf("expected %d sections, got %d", len(tracker.Sections), len(parsed))
	}
	for i, want := range tracker.Sections {
		got := parsed[i]
		if got.Name != want.Name {
			t.Errorf("section %d: name = %q, want %q", i, got.Name, want.Name)
		}
		if got.Usage != want.Usage {
			t.Errorf("section %d (%s): usage = %+v, want %+v", i, want.Name, got.Usage, want.Usage)
		}
	}
}

func TestUsageTracker_LoadSave(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_TEMP", dir)

	// Simulate assess phase saving
	var assess UsageTracker
	assess.Record("assess", Usage{InputTokens: 100, OutputTokens: 200, CostUSD: 0.50})
	if err := assess.Save(); err != nil {
		t.Fatal(err)
	}

	// Simulate implement phase loading assess data and adding its own
	var impl UsageTracker
	impl.Record("implement (attempt 1)", Usage{InputTokens: 500, OutputTokens: 1000, CostUSD: 1.25})
	impl.Load()
	if err := impl.Save(); err != nil {
		t.Fatal(err)
	}

	// Verify the final file has both sections
	final := ParseSummary(impl.FormatSummary())
	if len(final) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(final))
	}
	if final[0].Name != "assess" {
		t.Errorf("first section = %q, want 'assess'", final[0].Name)
	}
	if final[1].Name != "implement (attempt 1)" {
		t.Errorf("second section = %q, want 'implement (attempt 1)'", final[1].Name)
	}
}

func TestParseSummary_Empty(t *testing.T) {
	sections := ParseSummary("")
	if len(sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(sections))
	}
}

func TestBuildEnv_BaselineOnly(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", "/home/test")
	t.Setenv("SECRET_TOKEN", "do-not-leak")

	env := buildEnv(nil)

	envMap := envToMap(env)
	if envMap["PATH"] != "/usr/bin" {
		t.Errorf("expected PATH in env, got %q", envMap["PATH"])
	}
	if envMap["HOME"] != "/home/test" {
		t.Errorf("expected HOME in env, got %q", envMap["HOME"])
	}
	if _, ok := envMap["SECRET_TOKEN"]; ok {
		t.Error("SECRET_TOKEN should not be in env")
	}
}

func TestBuildEnv_WithContextVars(t *testing.T) {
	t.Setenv("ISSUE_TITLE", "bug report")
	t.Setenv("ISSUE_BODY", "it's broken")
	t.Setenv("SECRET_TOKEN", "do-not-leak")

	env := buildEnv([]string{"ISSUE_TITLE", "ISSUE_BODY"})

	envMap := envToMap(env)
	if envMap["ISSUE_TITLE"] != "bug report" {
		t.Errorf("expected ISSUE_TITLE in env, got %q", envMap["ISSUE_TITLE"])
	}
	if envMap["ISSUE_BODY"] != "it's broken" {
		t.Errorf("expected ISSUE_BODY in env, got %q", envMap["ISSUE_BODY"])
	}
	if _, ok := envMap["SECRET_TOKEN"]; ok {
		t.Error("SECRET_TOKEN should not be in env")
	}
}

func TestBuildEnv_UnsetContextVar(t *testing.T) {
	// A context var that isn't set in the environment should not appear
	env := buildEnv([]string{"NONEXISTENT_VAR"})

	envMap := envToMap(env)
	if _, ok := envMap["NONEXISTENT_VAR"]; ok {
		t.Error("unset context var should not appear in env")
	}
}

func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		m[key] = value
	}
	return m
}

func writeJSON(t *testing.T, v interface{}) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp(t.TempDir(), "claude_*.json")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(data)
	f.Close()
	return f.Name()
}

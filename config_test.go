package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultConfigReferencesPlaceholders(t *testing.T) {
	cfg := defaultConfig()

	if len(cfg.Transcribe.Command) == 0 {
		t.Fatal("default transcribe command is empty")
	}
	if !contains(cfg.Transcribe.Command, "{audio}") || !contains(cfg.Transcribe.Command, "{rawdir}") {
		t.Fatalf("default transcribe command must reference {audio} and {rawdir}, got %v", cfg.Transcribe.Command)
	}
	if len(cfg.Summarize.Command) == 0 {
		t.Fatal("default summarize command is empty")
	}
	if !contains(cfg.Summarize.Command, "{out}") {
		t.Fatalf("default summarize command must reference {out}, got %v", cfg.Summarize.Command)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[transcribe]
command = ["funasr", "{audio}", "--output-format", "json"]

[summarize]
command = ["mycli", "--out", "{out}"]
prompt-file = "~/prompts/x.md"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"funasr", "{audio}", "--output-format", "json"}
	if !reflect.DeepEqual(cfg.Transcribe.Command, want) {
		t.Fatalf("transcribe command = %v, want %v", cfg.Transcribe.Command, want)
	}
	if got := cfg.Summarize.PromptFile; got != "~/prompts/x.md" {
		t.Fatalf("prompt-file = %q, want ~/prompts/x.md", got)
	}
}

func TestLoadConfigMissingFileIsError(t *testing.T) {
	_, err := LoadConfigFile("/nonexistent/vidsum/config.toml")
	if err == nil {
		t.Fatal("explicit missing config file should error")
	}
}

func TestLoadConfigParseErrorIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.toml")
	os.WriteFile(path, []byte("[transcribe\ncommand = oops"), 0o644)

	_, err := LoadConfigFile(path)
	if err == nil {
		t.Fatal("a broken config file must be an error, never silently defaulted")
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

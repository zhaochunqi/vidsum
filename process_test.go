package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExpandTemplate(t *testing.T) {
	got := ExpandTemplate(
		[]string{"mlx_whisper", "{audio}", "--output-dir", "{rawdir}"},
		map[string]string{"audio": "/tmp/a.mp3", "rawdir": "/data/raw"},
	)
	want := []string{"mlx_whisper", "/tmp/a.mp3", "--output-dir", "/data/raw"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandTemplate = %v, want %v", got, want)
	}
}

func TestExpandTemplateUnknownPlaceholderUntouched(t *testing.T) {
	got := ExpandTemplate([]string{"x", "{nope}"}, map[string]string{})
	want := []string{"x", "{nope}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandTemplate = %v, want %v", got, want)
	}
}

func TestBuildAIInput(t *testing.T) {
	got := BuildAIInput("summarize this", `{"text": "hello"}`)
	want := "summarize this\n\n---\n\n{\"text\": \"hello\"}"
	if got != want {
		t.Fatalf("BuildAIInput = %q, want %q", got, want)
	}
}

func TestDone(t *testing.T) {
	dir := t.TempDir()

	if Done(filepath.Join(dir, "missing.mp3")) {
		t.Fatal("missing file must not be done")
	}

	empty := filepath.Join(dir, "empty.mp3")
	os.WriteFile(empty, nil, 0o644)
	if Done(empty) {
		t.Fatal("empty file must not be done")
	}

	full := filepath.Join(dir, "full.mp3")
	os.WriteFile(full, []byte("x"), 0o644)
	if !Done(full) {
		t.Fatal("non-empty file must be done")
	}
}

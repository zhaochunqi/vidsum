package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathsFor(t *testing.T) {
	p := PathsFor("data", "abc123")
	if p.Audio != filepath.Join("data", "audio", "abc123.mp3") {
		t.Fatalf("Audio = %q", p.Audio)
	}
	if p.Raw != filepath.Join("data", "raw", "abc123.json") {
		t.Fatalf("Raw = %q", p.Raw)
	}
	if p.Out != filepath.Join("data", "output", "abc123.md") {
		t.Fatalf("Out = %q", p.Out)
	}
}

func TestResolveIDFromYtDlp(t *testing.T) {
	var gotCmd []string
	run := func(stage string, cmd []string, stdin string) (string, error) {
		gotCmd = cmd
		return "dQw4w9WgXcQ\n", nil
	}
	id, err := ResolveID(run, "https://youtu.be/x")
	if err != nil {
		t.Fatal(err)
	}
	if id != "dQw4w9WgXcQ" {
		t.Fatalf("id = %q", id)
	}
	if !strings.HasPrefix(gotCmd[0], "yt-dlp") || !contains(gotCmd, "--print") {
		t.Fatalf("unexpected command %v", gotCmd)
	}
}

func TestResolveIDFallsBackToHash(t *testing.T) {
	run := func(stage string, cmd []string, stdin string) (string, error) {
		return "", errors.New("boom")
	}
	id1, err := ResolveID(run, "https://example.com/video/1")
	if err != nil {
		t.Fatal(err)
	}
	if len(id1) != 12 {
		t.Fatalf("fallback id = %q, want 12 chars", id1)
	}
	id2, _ := ResolveID(run, "https://example.com/video/1")
	if id1 != id2 {
		t.Fatal("fallback must be deterministic for the same URL")
	}
}

func newTestJob(t *testing.T) (*Job, *[]call) {
	dir := t.TempDir()
	calls := &[]call{}
	job := &Job{
		DataDir: dir,
		Cfg:     defaultConfig(),
		Run: func(stage string, cmd []string, stdin string) (string, error) {
			*calls = append(*calls, call{stage, cmd, stdin})
			if stage == "download" { // simulate yt-dlp producing the audio
				p := PathsFor(dir, "v1")
				os.MkdirAll(filepath.Dir(p.Audio), 0o755)
				os.WriteFile(p.Audio, []byte("audio"), 0o644)
			}
			return "", nil
		},
	}
	return job, calls
}

type call struct {
	stage string
	cmd   []string
	stdin string
}

func TestDownloadSkipsWhenDoneAndRunsOtherwise(t *testing.T) {
	job, calls := newTestJob(t)
	p := PathsFor(job.DataDir, "v1")

	skipped, err := job.Download("https://e.com/v1", "v1")
	if err != nil || skipped {
		t.Fatalf("first run: skipped=%v err=%v", skipped, err)
	}
	if len(*calls) != 1 || (*calls)[0].stage != "download" {
		t.Fatalf("expected one download call, got %+v", *calls)
	}
	cmd := (*calls)[0].cmd
	if !contains(cmd, "-x") || !contains(cmd, "https://e.com/v1") {
		t.Fatalf("unexpected yt-dlp command %v", cmd)
	}

	os.MkdirAll(filepath.Dir(p.Audio), 0o755)
	os.WriteFile(p.Audio, []byte("audio"), 0o644)

	skipped, err = job.Download("https://e.com/v1", "v1")
	if err != nil || !skipped {
		t.Fatalf("second run should skip: skipped=%v err=%v", skipped, err)
	}
	if len(*calls) != 1 {
		t.Fatal("skip must not invoke the runner")
	}
}

func TestDownloadForceRedoes(t *testing.T) {
	job, calls := newTestJob(t)
	job.Force = true
	p := PathsFor(job.DataDir, "v1")
	os.MkdirAll(filepath.Dir(p.Audio), 0o755)
	os.WriteFile(p.Audio, []byte("audio"), 0o644)

	skipped, err := job.Download("https://e.com/v1", "v1")
	if err != nil || skipped {
		t.Fatalf("force must redo: skipped=%v err=%v", skipped, err)
	}
	if len(*calls) != 1 {
		t.Fatal("force must invoke the runner")
	}
}

func TestTranscribeCapturesStdoutJson(t *testing.T) {
	job, _ := newTestJob(t)
	createAudio(t, job.DataDir, "v1")
	job.Run = func(stage string, cmd []string, stdin string) (string, error) {
		return `{"text": "hello"}`, nil
	}

	skipped, err := job.Transcribe("v1")
	if err != nil || skipped {
		t.Fatalf("transcribe: skipped=%v err=%v", skipped, err)
	}
	raw, _ := os.ReadFile(PathsFor(job.DataDir, "v1").Raw)
	if !strings.Contains(string(raw), "hello") {
		t.Fatalf("raw = %q, want captured stdout", raw)
	}
}

func TestTranscribeAcceptsEngineWrittenRaw(t *testing.T) {
	job, calls := newTestJob(t)
	createAudio(t, job.DataDir, "v1")
	job.Run = func(stage string, cmd []string, stdin string) (string, error) {
		*calls = append(*calls, call{stage, cmd, stdin})
		// Engine writes the file itself (mlx_whisper --output-dir style).
		os.WriteFile(PathsFor(job.DataDir, "v1").Raw, []byte(`{"segments":[]}`), 0o644)
		return "progress logs", nil
	}

	if _, err := job.Transcribe("v1"); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatal("engine must run exactly once")
	}
}

func TestTranscribeFailsWithoutAnyOutput(t *testing.T) {
	job, _ := newTestJob(t)
	if _, err := job.Transcribe("v1"); err == nil {
		t.Fatal("no output anywhere must be an error")
	}
}

func TestTranscribeSkipsWhenDone(t *testing.T) {
	job, calls := newTestJob(t)
	p := PathsFor(job.DataDir, "v1")
	os.MkdirAll(filepath.Dir(p.Raw), 0o755)
	os.WriteFile(p.Raw, []byte(`{"text":"x"}`), 0o644)

	skipped, err := job.Transcribe("v1")
	if err != nil || !skipped {
		t.Fatalf("should skip: skipped=%v err=%v", skipped, err)
	}
	if len(*calls) != 0 {
		t.Fatal("skip must not invoke the runner")
	}
}

func TestSummarizeWritesInputAndRequiresOut(t *testing.T) {
	job, calls := newTestJob(t)
	p := PathsFor(job.DataDir, "v1")
	os.MkdirAll(filepath.Dir(p.Raw), 0o755)
	os.WriteFile(p.Raw, []byte(`{"text":"hello"}`), 0o644)
	job.Run = func(stage string, cmd []string, stdin string) (string, error) {
		*calls = append(*calls, call{stage, cmd, stdin})
		os.WriteFile(p.Out, []byte("# Report"), 0o644)
		return "", nil
	}

	skipped, err := job.Summarize("v1")
	if err != nil || skipped {
		t.Fatalf("summarize: skipped=%v err=%v", skipped, err)
	}

	input, _ := os.ReadFile(p.Input)
	if !strings.Contains(string(input), DefaultPrompt) || !strings.Contains(string(input), "hello") {
		t.Fatalf("input file missing prompt or transcript:\n%s", input)
	}
	if len(*calls) != 1 || (*calls)[0].stdin != string(input) {
		t.Fatalf("stdin must carry the same concatenated input, got %+v", *calls)
	}
}

func TestSummarizeFailsWithoutOutFile(t *testing.T) {
	job, _ := newTestJob(t)
	p := PathsFor(job.DataDir, "v1")
	os.MkdirAll(filepath.Dir(p.Raw), 0o755)
	os.WriteFile(p.Raw, []byte(`{"text":"hello"}`), 0o644)

	if _, err := job.Summarize("v1"); err == nil {
		t.Fatal("AI command that never writes {out} must fail")
	}
}

func TestSummarizeRequiresRaw(t *testing.T) {
	job, _ := newTestJob(t)
	if _, err := job.Summarize("v1"); err == nil {
		t.Fatal("summarize without a transcript must fail")
	}
}

func TestSummarizeSkipsWhenDone(t *testing.T) {
	job, calls := newTestJob(t)
	p := PathsFor(job.DataDir, "v1")
	os.MkdirAll(filepath.Dir(p.Out), 0o755)
	os.WriteFile(p.Out, []byte("# Report"), 0o644)

	skipped, err := job.Summarize("v1")
	if err != nil || !skipped {
		t.Fatalf("should skip: skipped=%v err=%v", skipped, err)
	}
	if len(*calls) != 0 {
		t.Fatal("skip must not invoke the runner")
	}
}

func TestForceRemovesStaleRawBeforeRerun(t *testing.T) {
	job, _ := newTestJob(t)
	job.Force = true
	createAudio(t, job.DataDir, "v1")
	p := PathsFor(job.DataDir, "v1")
	os.MkdirAll(filepath.Dir(p.Raw), 0o755)
	os.WriteFile(p.Raw, []byte(`{"text":"stale"}`), 0o644)
	job.Run = func(stage string, cmd []string, stdin string) (string, error) {
		return "", nil // engine silently produces nothing
	}

	if _, err := job.Transcribe("v1"); err == nil {
		t.Fatal("force rerun with a silent engine must fail, not pass on the stale raw file")
	}
}

func createAudio(t *testing.T, dataDir, id string) {
	t.Helper()
	p := PathsFor(dataDir, id)
	os.MkdirAll(filepath.Dir(p.Audio), 0o755)
	os.WriteFile(p.Audio, []byte("audio"), 0o644)
}

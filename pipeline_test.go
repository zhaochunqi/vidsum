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
	id, title, err := ResolveID(run, "https://youtu.be/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id != "dQw4w9WgXcQ" || title != "" {
		t.Fatalf("id=%q title=%q", id, title)
	}
	if !strings.HasPrefix(gotCmd[0], "yt-dlp") || !contains(gotCmd, "--print") {
		t.Fatalf("unexpected command %v", gotCmd)
	}
}

func TestResolveIDFallsBackToHash(t *testing.T) {
	run := func(stage string, cmd []string, stdin string) (string, error) {
		return "", errors.New("boom")
	}
	id1, _, err := ResolveID(run, "https://example.com/video/1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(id1) != 12 {
		t.Fatalf("fallback id = %q, want 12 chars", id1)
	}
	id2, _, _ := ResolveID(run, "https://example.com/video/1", nil)
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
				audio := strings.TrimSuffix(cmd[cmdIndex(cmd, "-o")+1], ".%(ext)s") + ".mp3"
				os.MkdirAll(filepath.Dir(audio), 0o755)
				os.WriteFile(audio, []byte("audio"), 0o644)
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

func TestDownloadUsesConfiguredCommand(t *testing.T) {
	job, calls := newTestJob(t)
	job.Cfg.Download.ExtraArgs = []string{"--cookies-from-browser", "chrome"}

	if _, err := job.Download("https://e.com/v1", "v1"); err != nil {
		t.Fatal(err)
	}
	cmd := (*calls)[0].cmd
	want := []string{"--cookies-from-browser", "chrome"}
	found := false
	for i := 0; i+2 <= len(cmd); i++ {
		if cmd[i] == want[0] && cmd[i+1] == want[1] {
			found = true
		}
	}
	if !found {
		t.Fatalf("extra args %v not in cmd %v", want, cmd)
	}
	if cmd[len(cmd)-1] != "https://e.com/v1" {
		t.Fatalf("url must stay last, got %v", cmd)
	}
}

func TestResolveIDPassesExtraArgs(t *testing.T) {
	var gotCmd []string
	run := func(stage string, cmd []string, stdin string) (string, error) {
		gotCmd = cmd
		return "abc\n", nil
	}
	ResolveID(run, "https://e.com/v", []string{"--cookies-from-browser", "chrome"})
	if !contains(gotCmd, "--cookies-from-browser") || gotCmd[len(gotCmd)-1] != "https://e.com/v" {
		t.Fatalf("unexpected command %v", gotCmd)
	}
}

func TestExtractURLFromDouyinShareText(t *testing.T) {
	share := "0.53 RKj:/ :9pm 04/05 P@x.Sl 92科比一年7倍实盘心法：短线赚钱的根本。# 游资心法 # 情绪周期  https://v.douyin.com/0gC4hgX8SVA/ 复制此链接，打开Dou音搜索，直接观看视频！"
	if got := ExtractURL(share); got != "https://v.douyin.com/0gC4hgX8SVA/" {
		t.Fatalf("ExtractURL = %q", got)
	}
}

func TestExtractURLPassesPlainURLThrough(t *testing.T) {
	if got := ExtractURL("https://youtu.be/x?a=1&b=2"); got != "https://youtu.be/x?a=1&b=2" {
		t.Fatalf("ExtractURL = %q", got)
	}
}

func TestBaseName(t *testing.T) {
	if got := BaseName("v1", "some/title: here"); got != "v1 - some title here" {
		t.Fatalf("BaseName sanitize = %q", got)
	}
	if got := BaseName("v1", ""); got != "v1" {
		t.Fatalf("empty title = %q", got)
	}
	long := strings.Repeat("字", 100)
	if got := BaseName("v1", long); len([]rune(got)) != len([]rune("v1"))+1+80+0 && !strings.HasPrefix(got, "v1 - ") {
		t.Fatalf("long title not capped: %q (%d runes)", got, len([]rune(got)))
	}
}

func TestResolveIDReturnsTitle(t *testing.T) {
	run := func(stage string, cmd []string, stdin string) (string, error) {
		return "abc123\n我的标题\n", nil
	}
	id, title, err := ResolveID(run, "https://e.com/v", nil)
	if err != nil || id != "abc123" || title != "我的标题" {
		t.Fatalf("id=%q title=%q err=%v", id, title, err)
	}
}

func TestStepsUseTitleInFileNames(t *testing.T) {
	job, _ := newTestJob(t)
	base := BaseName("v1", "一个 标题")

	if _, err := job.Download("https://e.com/v1", base); err != nil {
		t.Fatal(err)
	}
	job.Run = func(stage string, cmd []string, stdin string) (string, error) {
		return `{"text":"ok"}`, nil
	}
	if _, err := job.Transcribe(base); err != nil {
		t.Fatal(err)
	}
	p := PathsFor(job.DataDir, base)
	for _, f := range []string{p.Audio, p.Raw} {
		if !Done(f) {
			t.Fatalf("%s missing", f)
		}
	}
	if !strings.Contains(p.Audio, " - 一个 标题") {
		t.Fatalf("audio name lacks title: %q", p.Audio)
	}
}

func TestStandaloneStepResolvesTitledArtifactsByIdPrefix(t *testing.T) {
	job, calls := newTestJob(t)
	job.Run = func(stage string, cmd []string, stdin string) (string, error) {
		*calls = append(*calls, call{stage, cmd, stdin})
		return `{"text":"x"}`, nil
	}
	base := BaseName("v1", "标题")
	createAudioAt(t, PathsFor(job.DataDir, base).Audio)

	skipped, err := job.Transcribe("v1") // bare id, must find "v1 - 标题.mp3"
	if err != nil || skipped {
		t.Fatalf("skipped=%v err=%v", skipped, err)
	}
	if !Done(PathsFor(job.DataDir, base).Raw) {
		t.Fatal("raw must land next to the titled audio")
	}
}

func createAudioAt(t *testing.T, path string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("audio"), 0o644)
}

func cmdIndex(cmd []string, flag string) int {
	for i, c := range cmd {
		if c == flag {
			return i
		}
	}
	return -1
}

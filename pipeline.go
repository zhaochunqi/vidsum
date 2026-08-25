package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Paths are the artifact locations for one video id, rooted at DataDir.
type Paths struct {
	Audio string // data/audio/<id>.mp3
	Raw   string // data/raw/<id>.json
	Input string // data/raw/<id>.input.txt (prompt + transcript fed to the AI command)
	Out   string // data/output/<id>.md
}

func PathsFor(dataDir, id string) Paths {
	return Paths{
		Audio: filepath.Join(dataDir, "audio", id+".mp3"),
		Raw:   filepath.Join(dataDir, "raw", id+".json"),
		Input: filepath.Join(dataDir, "raw", id+".input.txt"),
		Out:   filepath.Join(dataDir, "output", id+".md"),
	}
}

// ResolveID asks yt-dlp for the canonical video id and falls back to a
// deterministic hash prefix of the URL when that fails.
// ponytail: the fallback id breaks resume identity across runs when yt-dlp
// fails intermittently (two parallel artifact trees); acceptable because the
// fallback only triggers on total yt-dlp failure, where download would fail
// anyway.
func ResolveID(run Runner, url string) (string, error) {
	out, err := run("resolve", []string{"yt-dlp", "--no-download", "--print", "id", url}, "")
	if err == nil {
		if id := strings.TrimSpace(out); id != "" && !strings.Contains(id, "\n") {
			return id, nil
		}
	}
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:])[:12], nil
}

// Job runs pipeline steps for videos against a data directory.
type Job struct {
	DataDir string
	Cfg     Config
	Run     Runner
	Force   bool
}

func (j *Job) paths(id string) Paths { return PathsFor(j.DataDir, id) }

func (j *Job) ensureDir(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return nil
}

// Download fetches the audio with yt-dlp unless it already exists.
func (j *Job) Download(url, id string) (skipped bool, err error) {
	p := j.paths(id)
	if !j.Force && Done(p.Audio) {
		return true, nil
	}
	if j.Force {
		os.Remove(p.Audio) // stale artifact must not mask a failed redo
	}
	if err := j.ensureDir(p.Audio); err != nil {
		return false, err
	}
	tmpl := ExpandTemplate(
		[]string{"yt-dlp", "-x", "--audio-format", "mp3", "-o", strings.TrimSuffix(p.Audio, ".mp3") + ".%(ext)s", url},
		nil,
	)
	if _, err := j.Run("download", tmpl, ""); err != nil {
		return false, err
	}
	if !Done(p.Audio) {
		return false, fmt.Errorf("download finished but %s is missing", p.Audio)
	}
	return false, nil
}

// Transcribe runs the configured engine over the audio. Engines either write
// the raw JSON themselves (e.g. mlx_whisper --output-dir) or print it to
// stdout, which vidsum then captures.
func (j *Job) Transcribe(id string) (skipped bool, err error) {
	p := j.paths(id)
	if !j.Force && Done(p.Raw) {
		return true, nil
	}
	if j.Force {
		os.Remove(p.Raw) // stale artifact must not mask a failed redo
	}
	if !Done(p.Audio) {
		return false, fmt.Errorf("audio %s missing; run download first", p.Audio)
	}
	rawDir := filepath.Dir(p.Raw)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return false, err
	}
	stdout, err := j.Run("transcribe", ExpandTemplate(j.Cfg.Transcribe.Command, map[string]string{
		"audio":  p.Audio,
		"raw":    p.Raw,
		"rawdir": rawDir,
	}), "")
	if err != nil {
		return false, err
	}
	if Done(p.Raw) { // engine wrote the file itself
		return false, nil
	}
	if strings.TrimSpace(stdout) == "" {
		return false, fmt.Errorf("transcribe produced no output: no %s written and empty stdout", p.Raw)
	}
	return false, os.WriteFile(p.Raw, []byte(strings.TrimSpace(stdout)+"\n"), 0o644)
}

// Summarize feeds prompt + transcript to the AI command via stdin (also
// written to <id>.input.txt for CLIs that prefer {input}) and requires it to
// produce the output markdown at {out}.
func (j *Job) Summarize(id string) (skipped bool, err error) {
	p := j.paths(id)
	if !j.Force && Done(p.Out) {
		return true, nil
	}
	if j.Force {
		os.Remove(p.Out) // stale artifact must not mask a failed redo
	}
	if !Done(p.Raw) {
		return false, fmt.Errorf("transcript %s missing; run transcribe first", p.Raw)
	}
	prompt := DefaultPrompt
	if j.Cfg.Summarize.PromptFile != "" {
		path, err := expandHome(j.Cfg.Summarize.PromptFile)
		if err != nil {
			return false, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("read prompt-file: %w", err)
		}
		prompt = strings.TrimSpace(string(data))
	}
	raw, err := os.ReadFile(p.Raw)
	if err != nil {
		return false, err
	}
	input := BuildAIInput(prompt, strings.TrimSpace(string(raw)))
	if err := j.ensureDir(p.Input); err != nil {
		return false, err
	}
	if err := os.WriteFile(p.Input, []byte(input), 0o644); err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(p.Out), 0o755); err != nil {
		return false, err
	}
	_, err = j.Run("summarize", ExpandTemplate(j.Cfg.Summarize.Command, map[string]string{
		"out":    p.Out,
		"input":  p.Input,
		"rawdir": filepath.Dir(p.Raw),
	}), input)
	if err != nil {
		return false, err
	}
	if !Done(p.Out) {
		return false, fmt.Errorf("summarize command ran but did not write %s", p.Out)
	}
	return false, nil
}

func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[2:]), nil
}

// DefaultPrompt is used when no summarize.prompt-file is configured.
const DefaultPrompt = `You are a transcript organization assistant. You may only use the transcript provided below.

Constraints:

- Do not use outside knowledge or browse the web.
- Do not add information that is not in the transcript.
- If a passage is unclear in the transcript, say so instead of guessing.

Output requirements:

- Use Markdown.
- Split the content into sections by topic.
- Stay faithful: no summaries beyond light structure, no commentary, no rewriting facts into opinion.
- Keep facts, numbers, names, and policy/project titles as they naturally appear in the transcript.`

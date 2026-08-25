package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ExpandTemplate substitutes {key} placeholders in a command template.
// Unknown placeholders are left untouched so tools with their own brace
// syntax keep working.
func ExpandTemplate(tmpl []string, vars map[string]string) []string {
	out := make([]string, len(tmpl))
	for i, arg := range tmpl {
		for k, v := range vars {
			arg = strings.ReplaceAll(arg, "{"+k+"}", v)
		}
		out[i] = arg
	}
	return out
}

// BuildAIInput concatenates the prompt and the raw transcript the same way
// cctv-news did: prompt, separator, raw.
func BuildAIInput(prompt, raw string) string {
	return prompt + "\n\n---\n\n" + raw
}

// Done reports whether an artifact exists and is non-empty. This is the
// whole resume check: no state database, just artifacts on disk.
func Done(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

// RunError describes a failed external command.
type RunError struct {
	Stage  string
	Cmd    []string
	Stdout string
	Stderr string
	Err    error
}

func (e *RunError) Error() string {
	return fmt.Sprintf("%s failed (%v)\nstdout:\n%s\nstderr:\n%s", e.Stage, e.Err, e.Stdout, e.Stderr)
}

// Runner executes a command and returns its stdout. Injections point for tests.
type Runner func(stage string, cmd []string, stdin string) (string, error)

// Exec is the real Runner backed by os/exec. Child stderr streams live to
// the terminal (progress bars/logs) while still being captured for errors.
func Exec(stage string, cmd []string, stdin string) (string, error) {
	proc := osProcess(cmd[0], cmd[1:])
	proc.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	proc.Stdout = &stdout
	proc.Stderr = io.MultiWriter(os.Stderr, &stderr)
	if err := proc.Run(); err != nil {
		return stdout.String(), &RunError{Stage: stage, Cmd: cmd, Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
	}
	return stdout.String(), nil
}

// osProcess builds an exec.Cmd; split out so tests never need os/exec.
func osProcess(name string, args []string) *exec.Cmd {
	return exec.Command(name, args...)
}

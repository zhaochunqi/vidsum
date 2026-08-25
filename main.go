package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	force := fs.Bool("force", false, "redo steps even if their output already exists")
	configPath := fs.String("config", "", "path to config.toml (default: ~/.config/vidsum/config.toml if present)")
	fs.Parse(os.Args[2:])
	args := fs.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "vidsum %s takes exactly one argument\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	arg := args[0]

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	job := &Job{
		DataDir: "data",
		Cfg:     cfg,
		Run:     Exec,
		Force:   *force,
	}

	var skipped bool
	switch os.Args[1] {
	case "download":
		err = step(job, arg)
	case "transcribe":
		skipped, err = job.Transcribe(job.FindBase(arg))
	case "summarize":
		skipped, err = job.Summarize(job.FindBase(arg))
	case "run":
		err = runAll(job, arg)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if os.Args[1] != "run" && os.Args[1] != "download" {
		report(os.Args[1], arg, skipped)
	}
}

// step runs the download step; the argument is a URL that needs id resolution.
func step(job *Job, url string) error {
	url = ExtractURL(url)
	id, title, err := ResolveID(job.Run, url, job.Cfg.Download.ExtraArgs)
	if err != nil {
		return err
	}
	skipped, err := job.Download(url, BaseName(id, title))
	if err != nil {
		return err
	}
	report("download", BaseName(id, title), skipped)
	if !skipped {
		fmt.Printf("  -> %s\n", PathsFor(job.DataDir, BaseName(id, title)).Audio)
	}
	return nil
}

func runAll(job *Job, url string) error {
	url = ExtractURL(url)
	id, title, err := ResolveID(job.Run, url, job.Cfg.Download.ExtraArgs)
	if err != nil {
		return err
	}
	base := BaseName(id, title)

	skipped, err := job.Download(url, base)
	if err != nil {
		return err
	}
	report("download", id, skipped)

	skipped, err = job.Transcribe(base)
	if err != nil {
		return err
	}
	report("transcribe", base, skipped)

	skipped, err = job.Summarize(base)
	if err != nil {
		return err
	}
	report("summarize", base, skipped)
	return nil
}

func report(stage, id string, skipped bool) {
	verb := "done"
	if skipped {
		verb = "skipped"
	}
	fmt.Printf("%s %s: %s\n", stage, id, verb)
}

// loadConfig resolves the config path and loads it. Missing optional default
// file means defaults; an existing file that fails to parse is fatal.
func loadConfig(path string) (Config, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return defaultConfig(), nil
		}
		candidate := home + "/.config/vidsum/config.toml"
		if _, err := os.Stat(candidate); err != nil {
			return defaultConfig(), nil
		}
		path = candidate
	}
	return LoadConfigFile(path)
}

func usage() {
	fmt.Fprint(os.Stderr, `vidsum — download a video's audio, transcribe it, summarize it

Usage:
  vidsum run <url>            download + transcribe + summarize
  vidsum download <url>       yt-dlp the audio into data/audio/<id>.mp3
  vidsum transcribe <id>      run the transcription engine on data/audio/<id>.mp3
  vidsum summarize <id>       summarize data/raw/<id>.json into data/output/<id>.md

Flags (place after the subcommand, before the argument):
  -force    redo steps whose output already exists
  -config   path to config.toml

Steps skip automatically when their output already exists.
`)
}

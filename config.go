package main

import (
	"fmt"
	"os"

	toml "github.com/BurntSushi/toml"
)

type TranscribeConfig struct {
	// Command template; {audio} and {rawdir} are expanded before exec.
	Command []string `toml:"command"`
}

type SummarizeConfig struct {
	// Command template; {out} and {input} are expanded before exec.
	// prompt + transcript content is piped to stdin and written to {input}.
	Command    []string `toml:"command"`
	PromptFile string   `toml:"prompt-file"`
}

type Config struct {
	Transcribe TranscribeConfig
	Summarize  SummarizeConfig
}

func defaultConfig() Config {
	return Config{
		Transcribe: TranscribeConfig{
			Command: []string{
				"mlx_whisper", "{audio}",
				"--model", "mlx-community/whisper-large-v3-turbo",
				"--output-format", "json",
				"--output-dir", "{rawdir}",
			},
		},
		Summarize: SummarizeConfig{
			Command: []string{"pi", "exec", "--output", "{out}"},
		},
	}
}

func LoadConfigFile(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

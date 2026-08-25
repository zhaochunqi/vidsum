# vidsum

Download a video's audio, transcribe it locally, and summarize it into a Markdown report — one command, any video URL.

## 🚀 Quick Install

### Recommended (macOS)

```sh
brew install --cask zhaochunqi/tap/vidsum
```

Or grab a binary from [releases](https://github.com/zhaochunqi/vidsum/releases) and build from source with `go install github.com/zhaochunqi/vidsum@latest`.

## External dependencies

vidsum only orchestrates external tools — it ships no ML models:

- **yt-dlp** — download (`brew install yt-dlp`)
- **ffmpeg** — audio extraction (installed with yt-dlp)
- **A transcription engine** — default is `mlx_whisper` on Apple Silicon (`uv tool install mlx-whisper`); anything that reads an audio file works
- **An AI CLI** — e.g. `opencode`, `pi`, or `codex`, to write the summary

## Usage

Paste a plain URL, or even a whole share text blob (douyin style) — the first link is extracted automatically:

```sh
vidsum run '0.53 RKj:/ ... 92科比一年7倍实盘心法 https://v.douyin.com/0gC4hgX8SVA/ 复制此链接，直接观看视频！'
```

Each stage can also run on its own and resumes where it left off (an existing non-empty artifact is skipped; `--force` redoes a step):

```sh
vidsum download <url>      # → data/audio/<id>.mp3
vidsum transcribe <id>     # → data/raw/<id>.json
vidsum summarize <id>      # → data/output/<id>.md
```

Artifacts land under `data/` in your current directory.

## Configuration

Optional `~/.config/vidsum/config.toml` (or `-config <path>`). Everything is a command template with placeholders:

```toml
[download]
# extra args appended to the yt-dlp call — douyin needs fresh cookies:
extra-args = ["--cookies-from-browser", "chrome"]

[transcribe]
# {audio} = input audio path, {rawdir} = output dir for the JSON.
# The engine may write data/raw/<id>.json itself OR print JSON to stdout.
command = ["mlx_whisper", "{audio}", "--model", "mlx-community/whisper-large-v3-turbo", "--output-format", "json", "--output-dir", "{rawdir}"]

[summarize]
# prompt + transcript are piped to stdin; the command must write {out}.
command = ["sh", "-c", "opencode run -m opencode-go/ox-alpha-free > \"$1\"", "vidsum", "{out}"]
prompt-file = "~/prompts/summary.md"   # optional custom prompt
```

Without a config file you get sensible defaults (mlx_whisper + `pi`).

## How it works

```
URL ──yt-dlp──▶ data/audio/<id>.mp3
   ──engine───▶ data/raw/<id>.json
   ──AI CLI───▶ data/output/<id>.md
```

Generalized from [cctv-news](https://github.com/xiaolutech/cctv-news); design notes live in the [products repo](https://github.com/xiaolutech/products/tree/main/vidsum).

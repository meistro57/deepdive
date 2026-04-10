# 🎙️ DeepDive

**AI Podcast Generator** — Transform any text into a natural-sounding two-host deep dive conversation, inspired by Google's NotebookLM Audio Overview.

## How It Works

DeepDive reverse-engineers Google's multi-stage approach:

```
Source Text
    │
    ▼
┌─────────────────────────────────────────────┐
│  Pass 1: OUTLINE                            │
│  LLM generates a producer's outline with    │
│  topic flow, emotional beats, aha moments   │
├─────────────────────────────────────────────┤
│  Pass 2: FULL SCRIPT                        │
│  LLM writes complete two-host dialogue      │
│  with reactions, tangents, banter           │
├─────────────────────────────────────────────┤
│  Pass 3: CRITIQUE                           │
│  Second LLM pass reviews for pacing,        │
│  naturalness, engagement, chemistry         │
├─────────────────────────────────────────────┤
│  Pass 4: REVISE                             │
│  Original model rewrites incorporating      │
│  editorial feedback                         │
├─────────────────────────────────────────────┤
│  Pass 5: DISFLUENCIES                       │
│  Inject "ums", interruptions, reactions,    │
│  backchannels → structured JSON dialogue    │
└─────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────┐
│  TTS RENDERING                              │
│  Concurrent per-line synthesis with          │
│  distinct voices per host                   │
│  (OpenAI / ElevenLabs / Edge TTS)           │
├─────────────────────────────────────────────┤
│  AUDIO STITCHING                            │
│  FFmpeg: silence gaps, overlap handling,     │
│  loudness normalization (EBU R128)          │
└─────────────────────────────────────────────┘
    │
    ▼
  podcast.mp3
```

## Quick Start

### Prerequisites

- **Go 1.22+**
- **FFmpeg** (for audio processing)
- **OpenRouter API key** (for LLM script generation)
- One of: **OpenAI API key**, **ElevenLabs API key**, or **edge-tts** installed

### Install

```bash
git clone https://github.com/meistro/deepdive.git
cd deepdive
go build -o deepdive ./cmd/deepdive
```

### Setup

```bash
# Create default config
./deepdive init

# Set your API keys
export OPENROUTER_API_KEY="sk-or-v1-..."
export OPENAI_API_KEY="sk-..."          # if using OpenAI TTS
# OR
export ELEVENLABS_API_KEY="..."         # if using ElevenLabs
```

### Generate a Podcast

```bash
# Full pipeline — text file to MP3
./deepdive generate article.txt

# With options
./deepdive generate research.md --output my_show.mp3 --minutes 15 --tts elevenlabs

# Script only (for tweaking before rendering)
./deepdive script-only paper.txt --output script.json

# Render a pre-generated script
./deepdive render script.json --output podcast.mp3
```

## Configuration

Edit `deepdive.json` after running `init`:

```json
{
  "openrouter_api_key": "",
  "script_model": "deepseek/deepseek-r1",
  "critique_model": "google/gemini-2.0-flash-001",
  "tts_provider": "openai",
  "host_a": {
    "name": "Alex",
    "voice_id": "nova",
    "role": "curious",
    "speed": 1.0
  },
  "host_b": {
    "name": "Jamie",
    "voice_id": "echo",
    "role": "expert",
    "speed": 1.0
  },
  "crossfade_ms": 150,
  "silence_between_ms": 200,
  "normalize_lufs": -16.0,
  "target_minutes": 10
}
```

### Model Recommendations

| Model | Best For | Notes |
|-------|----------|-------|
| `deepseek/deepseek-r1` | Script generation | Great reasoning, cheap |
| `google/gemini-2.0-flash-001` | Critique pass | Fast, good editorial eye |
| `anthropic/claude-sonnet-4` | Script generation | Excellent dialogue writing |
| `meta-llama/llama-3.3-70b` | Budget option | Decent quality, very cheap |

### TTS Voice Pairing Tips

The key to convincing dialogue is **contrasting voices**:

- **OpenAI**: `nova` (curious host) + `echo` (expert host) — good contrast
- **ElevenLabs**: Clone two voices for maximum distinctiveness
- **Edge TTS** (free): `en-US-JasonNeural` + `en-US-JennyNeural`

## Architecture

```
deepdive/
├── cmd/deepdive/          # CLI entry point
│   └── main.go
├── internal/
│   ├── config/            # Configuration management
│   ├── llm/               # OpenRouter API client
│   ├── script/            # Multi-pass script generation pipeline
│   ├── tts/               # TTS provider abstraction (OpenAI, ElevenLabs, Edge)
│   ├── audio/             # FFmpeg-based audio stitching & normalization
│   └── ui/                # Terminal progress display
├── deepdive.json          # Config file (created by `init`)
└── output/                # Generated podcasts
```

## Workflow: Script → Edit → Render

The `script-only` + `render` split lets you tweak scripts before burning TTS credits:

```bash
# Generate script
./deepdive script-only source.txt --output draft.json

# Edit draft.json — fix lines, adjust emotions, add pauses
# (the JSON format is human-readable)

# Render to audio
./deepdive render draft.json --output final.mp3
```

### Script JSON Format

```json
{
  "title": "The Future of Consciousness Research",
  "summary": "A deep dive into...",
  "lines": [
    {
      "speaker": "A",
      "text": "OK so — I've been reading about this and honestly, it kind of blew my mind.",
      "emotion": "excited",
      "overlap": false,
      "pause_ms": 0
    },
    {
      "speaker": "B",
      "text": "Right? And here's the thing most people miss...",
      "emotion": "thoughtful",
      "overlap": false,
      "pause_ms": 300
    }
  ],
  "estimated_duration_seconds": 600
}
```

## Roadmap

- [ ] URL fetching (scrape articles directly)
- [ ] PDF/EPUB source ingestion
- [ ] Bubbletea interactive TUI with live waveform
- [ ] Multiple source synthesis (like NotebookLM's multi-doc)
- [ ] Intro/outro music mixing
- [ ] SSML emotion tags for Edge TTS
- [ ] Streaming generation (play while rendering)
- [ ] WebSocket server for GUI frontends

## Credits

Built by [Meistro](https://github.com/meistro) — structural steel detailer by day, 
consciousness explorer and code architect by night.

Inspired by Google's NotebookLM Audio Overview, powered by the same kind of 
multi-pass LLM pipeline that makes those deep dives so compelling.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meistro/deepdive/internal/audio"
	"github.com/meistro/deepdive/internal/config"
	"github.com/meistro/deepdive/internal/llm"
	"github.com/meistro/deepdive/internal/script"
	"github.com/meistro/deepdive/internal/tts"
	"github.com/meistro/deepdive/internal/ui"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate", "gen":
		requireArgs(3, "deepdive generate <source-file> [--output podcast.mp3]")
		runGenerate(os.Args[2:])
	case "script-only":
		requireArgs(3, "deepdive script-only <source-file> [--output script.json]")
		runScriptOnly(os.Args[2:])
	case "render":
		requireArgs(3, "deepdive render <script.json> [--output podcast.mp3]")
		runRender(os.Args[2:])
	case "init":
		runInit()
	case "voices":
		runVoices()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func requireArgs(min int, usage string) {
	if len(os.Args) < min {
		fmt.Fprintf(os.Stderr, "Usage: %s\n", usage)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`
  🎙️  DeepDive - AI Podcast Generator

  Transforms any text into a natural-sounding two-host podcast,
  inspired by Google's NotebookLM Audio Overview.

  COMMANDS:
    generate <source>     Full pipeline: source → script → audio → MP3
    script-only <source>  Generate just the dialogue script (JSON)
    render <script.json>  Render an existing script to audio
    init                  Create a default config file
    voices                List available TTS voices

  OPTIONS:
    --config <path>       Config file (default: ./deepdive.json)
    --output <path>       Output file path
    --model <model>       Override LLM model for script generation
    --tts <provider>      Override TTS provider (openai|elevenlabs|edge)
    --minutes <n>         Target podcast length in minutes

  ENVIRONMENT:
    OPENROUTER_API_KEY    Required for script generation
    OPENAI_API_KEY        Required for OpenAI TTS
    ELEVENLABS_API_KEY    Required for ElevenLabs TTS

  EXAMPLES:
    deepdive generate article.txt
    deepdive generate research.md --output my_show.mp3 --minutes 15
    deepdive script-only paper.txt --output script.json
    deepdive render script.json --tts elevenlabs

`)
}

// ── Flag parsing ──

type flags struct {
	source  string
	output  string
	config  string
	model   string
	ttsName string
	minutes int
}

func parseFlags(args []string, defaultOutput string) flags {
	f := flags{source: args[0], output: defaultOutput, config: "deepdive.json"}
	for i := 1; i < len(args); i++ {
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch args[i] {
		case "--output", "-o":
			f.output = next()
		case "--config", "-c":
			f.config = next()
		case "--model", "-m":
			f.model = next()
		case "--tts":
			f.ttsName = next()
		case "--minutes":
			fmt.Sscanf(next(), "%d", &f.minutes)
		}
	}
	return f
}

func applyOverrides(cfg *config.Config, f flags) {
	if f.model != "" {
		cfg.ScriptModel = f.model
	}
	if f.ttsName != "" {
		cfg.TTSProvider = f.ttsName
	}
	if f.minutes > 0 {
		cfg.TargetMinutes = f.minutes
	}
}

// ── Commands ──

func runGenerate(args []string) {
	startTime := time.Now()
	f := parseFlags(args, "output/podcast.mp3")

	cfg, err := config.Load(f.config)
	die("Config", err)
	applyOverrides(cfg, f)
	die("Validation", cfg.Validate())

	sourceText, err := readSource(f.source)
	die("Reading source", err)

	progress := ui.NewProgressPrinter()
	progress.Header(f.source)

	// ── Script generation ──
	client := llm.NewClient(cfg.OpenRouterAPIKey)
	pipeline := script.NewPipeline(client, cfg.ScriptModel, cfg.CritiqueModel,
		cfg.HostA.Name, cfg.HostB.Name, cfg.TargetMinutes)

	stageIdx := map[string]int{
		"outline": 0, "draft": 1, "critique": 2, "revise": 3, "disfluency": 4,
	}

	dialogue, err := pipeline.Generate(sourceText, func(stage, detail string) {
		base := strings.TrimSuffix(stage, "_done")
		if idx, ok := stageIdx[base]; ok {
			if strings.HasSuffix(stage, "_done") {
				progress.CompleteStage(idx, detail)
			} else {
				progress.StartStage(idx, detail)
			}
		}
	})
	die("Script generation", err)

	// Save script JSON alongside audio
	scriptPath := strings.TrimSuffix(f.output, filepath.Ext(f.output)) + "_script.json"
	saveJSON(scriptPath, dialogue)

	// ── TTS rendering ──
	progress.StartStage(5, fmt.Sprintf("Using %s", cfg.TTSProvider))

	provider, err := tts.NewProvider(cfg)
	die("TTS setup", err)

	workDir := filepath.Join(os.TempDir(), fmt.Sprintf("deepdive_%d", time.Now().UnixNano()))
	os.MkdirAll(workDir, 0755)
	defer os.RemoveAll(workDir)

	rendered, err := tts.RenderAll(provider, dialogue, cfg, workDir, progress.TTSProgress)
	die("TTS rendering", err)
	progress.CompleteStage(5, fmt.Sprintf("%d clips", len(rendered)))

	// ── Audio stitching ──
	progress.StartStage(6, "Assembling final podcast...")
	os.MkdirAll(filepath.Dir(f.output), 0755)

	err = audio.Stitch(rendered, f.output, audio.StitchConfig{
		CrossfadeMs:      cfg.CrossfadeMs,
		SilenceBetweenMs: cfg.SilenceBetweenMs,
		NormalizeLUFS:    cfg.NormalizeLUFS,
		AddRoomTone:      cfg.BackgroundNoise,
	})
	die("Audio stitching", err)
	progress.CompleteStage(6, f.output)

	progress.Footer(f.output, time.Duration(dialogue.Duration)*time.Second, time.Since(startTime))
}

func runScriptOnly(args []string) {
	f := parseFlags(args, "output/script.json")

	cfg, err := config.Load(f.config)
	die("Config", err)
	applyOverrides(cfg, f)

	if cfg.OpenRouterAPIKey == "" {
		fmt.Fprintln(os.Stderr, "  ✗ OPENROUTER_API_KEY is required")
		os.Exit(1)
	}

	sourceText, err := readSource(f.source)
	die("Reading source", err)

	progress := ui.NewProgressPrinter()
	progress.Header(f.source)

	client := llm.NewClient(cfg.OpenRouterAPIKey)
	pipeline := script.NewPipeline(client, cfg.ScriptModel, cfg.CritiqueModel,
		cfg.HostA.Name, cfg.HostB.Name, cfg.TargetMinutes)

	stageIdx := map[string]int{
		"outline": 0, "draft": 1, "critique": 2, "revise": 3, "disfluency": 4,
	}

	dialogue, err := pipeline.Generate(sourceText, func(stage, detail string) {
		base := strings.TrimSuffix(stage, "_done")
		if idx, ok := stageIdx[base]; ok {
			if strings.HasSuffix(stage, "_done") {
				progress.CompleteStage(idx, detail)
			} else {
				progress.StartStage(idx, detail)
			}
		}
	})
	die("Script generation", err)

	saveJSON(f.output, dialogue)
	fmt.Printf("\n  ✅ Script saved: %s (%d lines, ~%ds)\n\n",
		f.output, len(dialogue.Lines), dialogue.Duration)
}

func runRender(args []string) {
	f := parseFlags(args, "output/podcast.mp3")

	cfg, err := config.Load(f.config)
	die("Config", err)
	applyOverrides(cfg, f)

	data, err := os.ReadFile(f.source)
	die("Reading script", err)

	var dialogue script.Dialogue
	die("Parsing script", json.Unmarshal(data, &dialogue))

	fmt.Printf("\n  🎙️  Rendering %d lines with %s...\n\n", len(dialogue.Lines), cfg.TTSProvider)

	provider, err := tts.NewProvider(cfg)
	die("TTS setup", err)

	workDir := filepath.Join(os.TempDir(), fmt.Sprintf("deepdive_%d", time.Now().UnixNano()))
	os.MkdirAll(workDir, 0755)
	defer os.RemoveAll(workDir)

	progress := ui.NewProgressPrinter()
	rendered, err := tts.RenderAll(provider, &dialogue, cfg, workDir, progress.TTSProgress)
	die("TTS rendering", err)

	os.MkdirAll(filepath.Dir(f.output), 0755)
	err = audio.Stitch(rendered, f.output, audio.StitchConfig{
		CrossfadeMs:      cfg.CrossfadeMs,
		SilenceBetweenMs: cfg.SilenceBetweenMs,
		NormalizeLUFS:    cfg.NormalizeLUFS,
		AddRoomTone:      cfg.BackgroundNoise,
	})
	die("Audio stitching", err)

	fmt.Printf("\n  🎉 Podcast saved: %s\n\n", f.output)
}

func runInit() {
	cfg := config.DefaultConfig()
	path := "deepdive.json"

	if _, err := os.Stat(path); err == nil {
		fmt.Printf("  ⚠️  %s exists. Overwrite? (y/N): ", path)
		var a string
		fmt.Scanln(&a)
		if strings.ToLower(a) != "y" {
			fmt.Println("  Aborted.")
			return
		}
	}

	die("Saving config", cfg.Save(path))
	fmt.Printf(`
  ✅ Created %s

  Next steps:
    1. export OPENROUTER_API_KEY="sk-or-..."
       export OPENAI_API_KEY="sk-..."          # for OpenAI TTS
       export ELEVENLABS_API_KEY="..."         # for ElevenLabs TTS

    2. Edit deepdive.json to customize voices & models

    3. deepdive generate your-article.txt

`, path)
}

func runVoices() {
	fmt.Print(`
  🎙️  Available TTS Voices

  ── OpenAI TTS ──────────────────────────────────────
  alloy     Neutral, balanced
  echo      Warm, conversational   ← great for expert host
  fable     Expressive, animated
  onyx      Deep, authoritative
  nova      Friendly, upbeat       ← great for curious host
  shimmer   Clear, professional

  ── ElevenLabs ──────────────────────────────────────
  Use voice IDs from your ElevenLabs dashboard.
  Tip: clone two distinct voices for best results.

  ── Edge TTS (free) ─────────────────────────────────
  en-US-GuyNeural        Male, conversational
  en-US-JennyNeural      Female, conversational
  en-US-AriaNeural       Female, expressive
  en-US-DavisNeural      Male, calm
  en-US-JasonNeural      Male, energetic
  en-US-SaraNeural       Female, warm

  Set in deepdive.json → host_a.voice_id / host_b.voice_id

`)
}

// ── Helpers ──

func readSource(path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return "", fmt.Errorf("URL fetching not yet implemented — save as a text file first")
	}
	data, err := os.ReadFile(path)
	return string(data), err
}

func saveJSON(path string, v interface{}) {
	os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  JSON save failed: %v\n", err)
		return
	}
	os.WriteFile(path, data, 0644)
}

func die(ctx string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  ✗ %s: %v\n\n", ctx, err)
		os.Exit(1)
	}
}

package tts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/meistro/deepdive/internal/config"
	"github.com/meistro/deepdive/internal/script"
)

// Provider defines the TTS interface
type Provider interface {
	Synthesize(text string, voice config.VoiceConf, emotion string) ([]byte, error)
	Name() string
}

// RenderedLine is a TTS-rendered dialogue line
type RenderedLine struct {
	Index    int
	Line     script.Line
	AudioPath string
	Duration  time.Duration
	Err      error
}

// RenderAll renders all dialogue lines using the configured TTS provider
// with concurrent workers for speed
func RenderAll(provider Provider, dialogue *script.Dialogue, cfg *config.Config, workDir string, progress func(done, total int)) ([]RenderedLine, error) {
	total := len(dialogue.Lines)
	results := make([]RenderedLine, total)
	
	// Use a semaphore to limit concurrent TTS calls (be nice to APIs)
	maxWorkers := 4
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for i, line := range dialogue.Lines {
		wg.Add(1)
		go func(idx int, l script.Line) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			// Pick voice config based on speaker
			var voice config.VoiceConf
			if l.Speaker == "A" {
				voice = cfg.HostA
			} else {
				voice = cfg.HostB
			}

			outPath := filepath.Join(workDir, fmt.Sprintf("line_%04d.mp3", idx))

			audioData, err := provider.Synthesize(l.Text, voice, l.Emotion)
			if err != nil {
				results[idx] = RenderedLine{Index: idx, Line: l, Err: fmt.Errorf("TTS line %d: %w", idx, err)}
				return
			}

			if err := os.WriteFile(outPath, audioData, 0644); err != nil {
				results[idx] = RenderedLine{Index: idx, Line: l, Err: fmt.Errorf("writing line %d: %w", idx, err)}
				return
			}

			// Get actual audio duration using ffprobe
			dur := getAudioDuration(outPath)

			results[idx] = RenderedLine{
				Index:     idx,
				Line:      l,
				AudioPath: outPath,
				Duration:  dur,
			}

			mu.Lock()
			done++
			if progress != nil {
				progress(done, total)
			}
			mu.Unlock()
		}(i, line)
	}

	wg.Wait()

	// Check for errors
	var errs []error
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, r.Err)
		}
	}
	if len(errs) > 0 {
		return results, fmt.Errorf("%d TTS errors (first: %w)", len(errs), errs[0])
	}

	return results, nil
}

// ── OpenAI TTS Provider ──

type OpenAIProvider struct {
	apiKey string
	model  string // "tts-1" or "tts-1-hd"
}

func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey: apiKey,
		model:  "tts-1-hd",
	}
}

func (p *OpenAIProvider) Name() string { return "OpenAI TTS" }

func (p *OpenAIProvider) Synthesize(text string, voice config.VoiceConf, emotion string) ([]byte, error) {
	// OpenAI TTS accepts voice + optional speed
	payload := map[string]interface{}{
		"model":           p.model,
		"input":           text,
		"voice":           voice.VoiceID, // alloy, echo, fable, onyx, nova, shimmer
		"response_format": "mp3",
		"speed":           voice.Speed,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI TTS error %d: %s", resp.StatusCode, string(errBody))
	}

	return io.ReadAll(resp.Body)
}

// ── ElevenLabs TTS Provider ──

type ElevenLabsProvider struct {
	apiKey string
}

func NewElevenLabsProvider(apiKey string) *ElevenLabsProvider {
	return &ElevenLabsProvider{apiKey: apiKey}
}

func (p *ElevenLabsProvider) Name() string { return "ElevenLabs" }

func (p *ElevenLabsProvider) Synthesize(text string, voice config.VoiceConf, emotion string) ([]byte, error) {
	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", voice.VoiceID)

	// Map emotion to ElevenLabs style settings
	stability, similarity := emotionToElevenLabsParams(emotion)

	payload := map[string]interface{}{
		"text":     text,
		"model_id": "eleven_turbo_v2_5",
		"voice_settings": map[string]interface{}{
			"stability":        stability,
			"similarity_boost": similarity,
			"style":            0.5,
			"use_speaker_boost": true,
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", p.apiKey)
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ElevenLabs error %d: %s", resp.StatusCode, string(errBody))
	}

	return io.ReadAll(resp.Body)
}

func emotionToElevenLabsParams(emotion string) (stability, similarity float64) {
	switch emotion {
	case "excited":
		return 0.3, 0.8
	case "surprised":
		return 0.25, 0.75
	case "thoughtful":
		return 0.7, 0.85
	case "serious":
		return 0.8, 0.9
	case "amused":
		return 0.35, 0.8
	case "curious":
		return 0.4, 0.8
	case "skeptical":
		return 0.6, 0.85
	default:
		return 0.5, 0.8
	}
}

// ── Edge TTS Provider (free, uses Microsoft Edge's TTS) ──

type EdgeProvider struct{}

func NewEdgeProvider() *EdgeProvider {
	return &EdgeProvider{}
}

func (p *EdgeProvider) Name() string { return "Edge TTS (free)" }

func (p *EdgeProvider) Synthesize(text string, voice config.VoiceConf, emotion string) ([]byte, error) {
	// Edge TTS is a CLI tool — we shell out to it
	// Voice IDs are like "en-US-GuyNeural", "en-US-JennyNeural"
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("deepdive_edge_%d.mp3", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	voiceID := voice.VoiceID
	if voiceID == "" {
		voiceID = "en-US-GuyNeural"
	}

	// Build SSML for better emotion control
	ssml := buildEdgeSSML(text, voiceID, emotion, voice.Speed)

	cmd := exec.Command("edge-tts", "--text", ssml, "--voice", voiceID, "--write-media", tmpFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("edge-tts failed: %w\n%s", err, string(output))
	}

	return os.ReadFile(tmpFile)
}

func buildEdgeSSML(text, voice, emotion string, speed float64) string {
	// Edge TTS handles plain text well — SSML is optional
	// For now, just return the text; can enhance later with prosody tags
	return text
}

// ── Helper ──

func getAudioDuration(path string) time.Duration {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := cmd.Output()
	if err != nil {
		return 3 * time.Second // fallback estimate
	}

	var seconds float64
	fmt.Sscanf(string(bytes.TrimSpace(out)), "%f", &seconds)
	return time.Duration(seconds * float64(time.Second))
}

// NewProvider creates the appropriate TTS provider based on config
func NewProvider(cfg *config.Config) (Provider, error) {
	switch cfg.TTSProvider {
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("OpenAI API key required for OpenAI TTS")
		}
		return NewOpenAIProvider(cfg.OpenAIAPIKey), nil
	case "elevenlabs":
		if cfg.ElevenLabsAPIKey == "" {
			return nil, fmt.Errorf("ElevenLabs API key required")
		}
		return NewElevenLabsProvider(cfg.ElevenLabsAPIKey), nil
	case "edge":
		return NewEdgeProvider(), nil
	default:
		return nil, fmt.Errorf("unknown TTS provider: %s", cfg.TTSProvider)
	}
}

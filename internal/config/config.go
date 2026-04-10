package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all application configuration
type Config struct {
	// LLM settings
	OpenRouterAPIKey string `json:"openrouter_api_key"`
	ScriptModel      string `json:"script_model"`      // e.g. "deepseek/deepseek-r1" or "google/gemini-2.0-flash-001"
	CritiqueModel    string `json:"critique_model"`     // model for critique/revision pass

	// TTS settings
	TTSProvider string    `json:"tts_provider"` // "elevenlabs", "openai", "edge", "local"
	HostA       VoiceConf `json:"host_a"`
	HostB       VoiceConf `json:"host_b"`

	// ElevenLabs specific
	ElevenLabsAPIKey string `json:"elevenlabs_api_key,omitempty"`

	// OpenAI TTS specific (also works via OpenRouter for some providers)
	OpenAIAPIKey string `json:"openai_api_key,omitempty"`

	// Audio settings
	OutputDir       string  `json:"output_dir"`
	CrossfadeMs     int     `json:"crossfade_ms"`
	SilenceBetweenMs int    `json:"silence_between_ms"`
	BackgroundNoise bool    `json:"background_noise"` // subtle room tone
	NormalizeLUFS   float64 `json:"normalize_lufs"`   // target loudness

	// Script generation
	MaxSourceTokens int `json:"max_source_tokens"` // truncate source beyond this
	TargetMinutes   int `json:"target_minutes"`    // approximate podcast length
}

// VoiceConf configures a single speaker voice
type VoiceConf struct {
	Name     string  `json:"name"`      // display name e.g. "Alex", "Jamie"
	VoiceID  string  `json:"voice_id"`  // provider-specific voice identifier
	Role     string  `json:"role"`      // "curious" or "expert"
	Speed    float64 `json:"speed"`     // speech rate multiplier
	Pitch    string  `json:"pitch"`     // provider-specific pitch adjustment
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ScriptModel:      "deepseek/deepseek-r1",
		CritiqueModel:    "google/gemini-2.0-flash-001",
		TTSProvider:      "openai",
		HostA: VoiceConf{
			Name:    "Alex",
			VoiceID: "alloy",
			Role:    "curious",
			Speed:   1.0,
		},
		HostB: VoiceConf{
			Name:    "Jamie",
			VoiceID: "echo",
			Role:    "expert",
			Speed:   1.0,
		},
		OutputDir:        "./output",
		CrossfadeMs:      150,
		SilenceBetweenMs: 200,
		BackgroundNoise:  true,
		NormalizeLUFS:    -16.0,
		MaxSourceTokens:  100000,
		TargetMinutes:    10,
	}
}

// Load reads config from file, falling back to defaults
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file — use defaults + env vars
			cfg.loadEnv()
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.loadEnv() // env vars override file
	return cfg, nil
}

// loadEnv overrides config with environment variables
func (c *Config) loadEnv() {
	if v := os.Getenv("OPENROUTER_API_KEY"); v != "" {
		c.OpenRouterAPIKey = v
	}
	if v := os.Getenv("ELEVENLABS_API_KEY"); v != "" {
		c.ElevenLabsAPIKey = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		c.OpenAIAPIKey = v
	}
	if v := os.Getenv("DEEPDIVE_MODEL"); v != "" {
		c.ScriptModel = v
	}
	if v := os.Getenv("DEEPDIVE_TTS"); v != "" {
		c.TTSProvider = v
	}
}

// Save writes config to a file
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Validate checks required fields
func (c *Config) Validate() error {
	if c.OpenRouterAPIKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY is required (set env var or config file)")
	}
	if c.TTSProvider == "elevenlabs" && c.ElevenLabsAPIKey == "" {
		return fmt.Errorf("ELEVENLABS_API_KEY required when using ElevenLabs TTS")
	}
	if c.TTSProvider == "openai" && c.OpenAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY required when using OpenAI TTS")
	}
	return nil
}

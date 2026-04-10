package audio

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/meistro/deepdive/internal/tts"
)

// StitchConfig controls how audio clips are assembled
type StitchConfig struct {
	CrossfadeMs      int     // crossfade duration for overlapping lines
	SilenceBetweenMs int     // default gap between lines
	NormalizeLUFS    float64 // target loudness (-16 is podcast standard)
	AddRoomTone      bool    // add subtle background ambience
}

// Stitch assembles rendered TTS lines into a final podcast MP3
func Stitch(lines []tts.RenderedLine, outputPath string, cfg StitchConfig) error {
	if len(lines) == 0 {
		return fmt.Errorf("no lines to stitch")
	}

	workDir := filepath.Dir(lines[0].AudioPath)

	// Build an FFmpeg filter chain that concatenates with gaps and crossfades
	// Strategy: generate silence segments between clips, then concatenate all

	// Step 1: Create a concat list file
	listPath := filepath.Join(workDir, "concat_list.txt")
	var listEntries []string

	for i, line := range lines {
		if line.Err != nil {
			continue // skip failed lines
		}

		// Add silence before this line (unless it's an overlap or first line)
		if i > 0 {
			pauseMs := line.Line.Pause
			if pauseMs == 0 {
				pauseMs = cfg.SilenceBetweenMs
			}

			// If this line overlaps, use shorter gap (or negative for true overlap)
			if line.Line.Overlap {
				pauseMs = 50 // tiny gap for interruptions
			}

			if pauseMs > 0 {
				silencePath := filepath.Join(workDir, fmt.Sprintf("silence_%04d.mp3", i))
				if err := generateSilence(silencePath, pauseMs); err != nil {
					return fmt.Errorf("generating silence %d: %w", i, err)
				}
				listEntries = append(listEntries, fmt.Sprintf("file '%s'", silencePath))
			}
		}

		listEntries = append(listEntries, fmt.Sprintf("file '%s'", line.AudioPath))
	}

	if err := os.WriteFile(listPath, []byte(strings.Join(listEntries, "\n")), 0644); err != nil {
		return fmt.Errorf("writing concat list: %w", err)
	}

	// Step 2: Concatenate all clips
	rawPath := filepath.Join(workDir, "raw_concat.mp3")
	concatCmd := exec.Command("ffmpeg", "-y", "-f", "concat", "-safe", "0",
		"-i", listPath, "-c", "copy", rawPath)
	if output, err := concatCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("concatenating: %w\n%s", err, string(output))
	}

	// Step 3: Normalize loudness to podcast standard
	normalizedPath := filepath.Join(workDir, "normalized.mp3")
	if err := normalizeLoudness(rawPath, normalizedPath, cfg.NormalizeLUFS); err != nil {
		// Fall back to unnormalized if loudnorm isn't available
		normalizedPath = rawPath
	}

	// Step 4: Copy to final output
	if err := copyFile(normalizedPath, outputPath); err != nil {
		return fmt.Errorf("copying to output: %w", err)
	}

	return nil
}

// generateSilence creates an MP3 file of silence with the given duration
func generateSilence(path string, durationMs int) error {
	duration := fmt.Sprintf("%.3f", float64(durationMs)/1000.0)
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i",
		fmt.Sprintf("anullsrc=r=44100:cl=mono,atrim=duration=%s", duration),
		"-c:a", "libmp3lame", "-b:a", "128k", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg silence: %w\n%s", err, string(output))
	}
	return nil
}

// normalizeLoudness applies EBU R128 loudness normalization
func normalizeLoudness(inputPath, outputPath string, targetLUFS float64) error {
	// Two-pass loudnorm: first measure, then normalize
	// Single-pass is usually good enough for podcasts
	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath,
		"-af", fmt.Sprintf("loudnorm=I=%.1f:TP=-1.5:LRA=11", targetLUFS),
		"-c:a", "libmp3lame", "-b:a", "192k",
		outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("loudnorm: %w\n%s", err, string(output))
	}
	return nil
}

func copyFile(src, dst string) error {
	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

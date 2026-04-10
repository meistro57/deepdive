package ui

import (
	"fmt"
	"strings"
	"time"
)

// Spinner characters for terminal animation
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Stage represents a pipeline stage
type Stage struct {
	Name    string
	Status  string // "pending", "running", "done", "error"
	Detail  string
	Started time.Time
	Elapsed time.Duration
}

// ProgressPrinter handles simple terminal progress output
// (Bubbletea version can be added later for full interactive mode)
type ProgressPrinter struct {
	stages       []*Stage
	currentStage int
	spinFrame    int
}

// NewProgressPrinter creates a new progress display
func NewProgressPrinter() *ProgressPrinter {
	return &ProgressPrinter{
		stages: []*Stage{
			{Name: "📋 Generating outline", Status: "pending"},
			{Name: "✍️  Writing full script", Status: "pending"},
			{Name: "🔍 Critiquing script", Status: "pending"},
			{Name: "📝 Revising script", Status: "pending"},
			{Name: "🗣️  Adding natural speech patterns", Status: "pending"},
			{Name: "🎙️  Rendering TTS audio", Status: "pending"},
			{Name: "🎵 Stitching final audio", Status: "pending"},
		},
	}
}

// Header prints the opening banner
func (p *ProgressPrinter) Header(source string) {
	fmt.Println()
	fmt.Println(boxStyle("🎙️  DeepDive Podcast Generator"))
	fmt.Println()

	// Truncate source preview
	preview := source
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	fmt.Printf("  📄 Source: %s\n\n", dimText(preview))
	fmt.Println(divider())
}

// StartStage marks a stage as running
func (p *ProgressPrinter) StartStage(index int, detail string) {
	if index >= len(p.stages) {
		return
	}
	p.currentStage = index
	p.stages[index].Status = "running"
	p.stages[index].Detail = detail
	p.stages[index].Started = time.Now()

	frame := spinnerFrames[p.spinFrame%len(spinnerFrames)]
	p.spinFrame++

	fmt.Printf("  %s %s %s\n", yellowText(frame), p.stages[index].Name, dimText(detail))
}

// CompleteStage marks a stage as done
func (p *ProgressPrinter) CompleteStage(index int, detail string) {
	if index >= len(p.stages) {
		return
	}
	p.stages[index].Status = "done"
	p.stages[index].Elapsed = time.Since(p.stages[index].Started)
	p.stages[index].Detail = detail

	fmt.Printf("  %s %s %s %s\n",
		greenText("✓"),
		p.stages[index].Name,
		dimText(detail),
		dimText(fmt.Sprintf("(%s)", p.stages[index].Elapsed.Round(time.Second))),
	)
}

// ErrorStage marks a stage as failed
func (p *ProgressPrinter) ErrorStage(index int, err error) {
	if index >= len(p.stages) {
		return
	}
	p.stages[index].Status = "error"
	fmt.Printf("  %s %s %s\n", redText("✗"), p.stages[index].Name, redText(err.Error()))
}

// TTSProgress shows TTS rendering progress
func (p *ProgressPrinter) TTSProgress(done, total int) {
	pct := float64(done) / float64(total) * 100
	bar := progressBar(done, total, 30)
	fmt.Printf("\r  🎙️  Rendering: %s %.0f%% (%d/%d)", bar, pct, done, total)
	if done == total {
		fmt.Println()
	}
}

// Footer prints the completion summary
func (p *ProgressPrinter) Footer(outputPath string, duration time.Duration, totalElapsed time.Duration) {
	fmt.Println()
	fmt.Println(divider())
	fmt.Println()
	fmt.Printf("  %s Podcast generated!\n", greenText("🎉"))
	fmt.Printf("  📁 Output: %s\n", boldText(outputPath))
	fmt.Printf("  ⏱️  Duration: %s\n", duration.Round(time.Second))
	fmt.Printf("  🕐 Total time: %s\n", totalElapsed.Round(time.Second))
	fmt.Println()
}

// ── Styling helpers (ANSI escape codes for simple terminal coloring) ──

func greenText(s string) string  { return "\033[32m" + s + "\033[0m" }
func yellowText(s string) string { return "\033[33m" + s + "\033[0m" }
func redText(s string) string    { return "\033[31m" + s + "\033[0m" }
func dimText(s string) string    { return "\033[2m" + s + "\033[0m" }
func boldText(s string) string   { return "\033[1m" + s + "\033[0m" }

func boxStyle(title string) string {
	width := len(title) + 4
	top := "╭" + strings.Repeat("─", width) + "╮"
	mid := "│  " + boldText(title) + "  │"
	bot := "╰" + strings.Repeat("─", width) + "╯"
	return top + "\n" + mid + "\n" + bot
}

func divider() string {
	return dimText("  " + strings.Repeat("─", 50))
}

func progressBar(done, total, width int) string {
	if total == 0 {
		return strings.Repeat("░", width)
	}
	filled := (done * width) / total
	if filled > width {
		filled = width
	}
	return greenText(strings.Repeat("█", filled)) + dimText(strings.Repeat("░", width-filled))
}

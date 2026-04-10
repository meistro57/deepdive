package script

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/meistro/deepdive/internal/llm"
)

// Dialogue represents the final structured script
type Dialogue struct {
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Lines    []Line `json:"lines"`
	Duration int    `json:"estimated_duration_seconds"`
}

// Line is a single spoken line in the dialogue
type Line struct {
	Speaker    string `json:"speaker"`     // "A" or "B"
	Text       string `json:"text"`        // what they say
	Emotion    string `json:"emotion"`     // "excited", "thoughtful", "surprised", etc.
	Overlap    bool   `json:"overlap"`     // should overlap with previous line
	Pause      int    `json:"pause_ms"`    // pause before this line in ms
}

// Pipeline holds the multi-pass generation state
type Pipeline struct {
	client        *llm.Client
	scriptModel   string
	critiqueModel string
	hostAName     string
	hostBName     string
	targetMinutes int

	// Intermediate outputs (exposed for UI progress)
	Outline  string
	Draft    string
	Critique string
	Revised  string
	Final    string
}

// NewPipeline creates a script generation pipeline
func NewPipeline(client *llm.Client, scriptModel, critiqueModel, hostA, hostB string, targetMin int) *Pipeline {
	return &Pipeline{
		client:        client,
		scriptModel:   scriptModel,
		critiqueModel: critiqueModel,
		hostAName:     hostA,
		hostBName:     hostB,
		targetMinutes: targetMin,
	}
}

// ProgressFunc is called with stage name and detail during generation
type ProgressFunc func(stage string, detail string)

// Generate runs the full multi-pass pipeline
func (p *Pipeline) Generate(sourceText string, progress ProgressFunc) (*Dialogue, error) {
	if progress == nil {
		progress = func(string, string) {}
	}

	// ── Pass 1: Generate Outline ──
	progress("outline", "Generating conversation outline...")
	outline, err := p.generateOutline(sourceText)
	if err != nil {
		return nil, fmt.Errorf("outline pass: %w", err)
	}
	p.Outline = outline
	progress("outline_done", outline)

	// ── Pass 2: Generate Full Script ──
	progress("draft", "Writing full dialogue script...")
	draft, err := p.generateDraft(sourceText, outline)
	if err != nil {
		return nil, fmt.Errorf("draft pass: %w", err)
	}
	p.Draft = draft
	progress("draft_done", fmt.Sprintf("%d chars", len(draft)))

	// ── Pass 3: Critique ──
	progress("critique", "Critiquing the script...")
	critique, err := p.critiqueScript(draft)
	if err != nil {
		return nil, fmt.Errorf("critique pass: %w", err)
	}
	p.Critique = critique
	progress("critique_done", critique)

	// ── Pass 4: Revise Based on Critique ──
	progress("revise", "Revising script based on critique...")
	revised, err := p.reviseScript(sourceText, draft, critique)
	if err != nil {
		return nil, fmt.Errorf("revision pass: %w", err)
	}
	p.Revised = revised
	progress("revise_done", fmt.Sprintf("%d chars", len(revised)))

	// ── Pass 5: Inject Disfluencies & Structure as JSON ──
	progress("disfluency", "Adding natural speech patterns and structuring output...")
	final, err := p.injectDisfluencies(revised)
	if err != nil {
		return nil, fmt.Errorf("disfluency pass: %w", err)
	}
	p.Final = final
	progress("disfluency_done", "Parsing structured dialogue...")

	// Parse the final JSON dialogue
	dialogue, err := parseDialogue(final)
	if err != nil {
		return nil, fmt.Errorf("parsing final dialogue: %w", err)
	}

	progress("complete", fmt.Sprintf("%d lines, ~%ds", len(dialogue.Lines), dialogue.Duration))
	return dialogue, nil
}

func (p *Pipeline) generateOutline(source string) (string, error) {
	system := fmt.Sprintf(`You are a podcast producer creating an outline for a deep-dive conversation 
between two hosts: %s (the curious, enthusiastic one who asks great questions) and 
%s (the knowledgeable expert who explains things clearly with great analogies).

Target length: approximately %d minutes of conversation.

Create a detailed outline with:
1. A compelling hook/opening that draws listeners in immediately
2. 3-5 major topic sections, each with key points and planned "aha moments"  
3. Natural transition points between topics
4. A satisfying conclusion that ties themes together
5. Places where genuine surprise, humor, or debate would feel natural

The outline should read like a producer's notes — detailed enough that a scriptwriter 
could create a natural-sounding conversation from it. Include notes on emotional beats 
and where the conversation should feel most energetic vs reflective.`, p.hostAName, p.hostBName, p.targetMinutes)

	user := fmt.Sprintf("Here is the source material to build the podcast outline from:\n\n---\n%s\n---\n\nCreate the detailed producer's outline.", source)

	return p.client.CompleteWithSystem(p.scriptModel, system, user, 4096, 0.7)
}

func (p *Pipeline) generateDraft(source, outline string) (string, error) {
	system := fmt.Sprintf(`You are a world-class podcast scriptwriter. Write a complete dialogue script 
between %s and %s based on the provided outline and source material.

CRITICAL RULES:
- Write ONLY dialogue. No stage directions, no narration, no "[laughs]" markers yet.
- Each line should be prefixed with the speaker name followed by a colon.
- Make the conversation feel REAL — hosts should react to each other, build on points, 
  occasionally go on tangents before coming back, and genuinely engage.
- %s asks probing questions, makes connections to everyday life, and gets genuinely 
  excited about interesting points. They occasionally push back or play devil's advocate.
- %s explains complex ideas with vivid analogies, shares relevant context, and builds 
  excitement through their depth of knowledge. They sometimes pause to think.
- Include moments of genuine discovery — where a host realizes something new mid-conversation.
- The dialogue should feel like two smart friends geeking out, not a formal interview.
- Vary line length: some lines should be long explanations, others should be quick reactions 
  like "Wait, really?" or "That's wild" or "OK so let me make sure I understand this."
- Target approximately %d minutes of spoken dialogue (roughly 150 words per minute).`, 
		p.hostAName, p.hostBName, p.hostAName, p.hostBName, p.targetMinutes)

	user := fmt.Sprintf(`SOURCE MATERIAL:
---
%s
---

PRODUCER'S OUTLINE:
---
%s
---

Write the complete dialogue script now. Remember: natural, engaging, real conversation between two genuinely interested people.`, source, outline)

	return p.client.CompleteWithSystem(p.scriptModel, system, user, 8192, 0.8)
}

func (p *Pipeline) critiqueScript(draft string) (string, error) {
	system := `You are an experienced podcast editor reviewing a script. Provide specific, 
actionable feedback on:

1. NATURAL FLOW: Does it sound like a real conversation or does it feel scripted? 
   Point out any lines that feel wooden or forced.
2. PACING: Are there sections that drag or feel rushed? Where should we slow down or speed up?
3. ENGAGEMENT: Are there enough "hooks" — moments of surprise, humor, or revelation 
   that keep a listener engaged?
4. CHEMISTRY: Do the hosts feel like they're genuinely reacting to each other, or just 
   taking turns reading paragraphs?
5. CLARITY: Are complex topics explained well enough? Any jargon that needs unpacking?
6. OPENING: Does it grab attention in the first 30 seconds?
7. CLOSING: Does it end satisfyingly or just trail off?

Be brutally honest. This script needs to compete with real podcasts. Give specific line-level 
feedback where possible, and suggest concrete improvements.`

	user := fmt.Sprintf("Review this podcast script:\n\n---\n%s\n---", draft)

	return p.client.CompleteWithSystem(p.critiqueModel, system, user, 3000, 0.6)
}

func (p *Pipeline) reviseScript(source, draft, critique string) (string, error) {
	system := fmt.Sprintf(`You are a podcast scriptwriter revising a dialogue based on editorial feedback.
Rewrite the COMPLETE script incorporating the critique. Don't just patch — if structural 
changes are needed, make them. The hosts are %s and %s.

Output the full revised dialogue script with the same format (Speaker: dialogue).
Make every line count. If a line doesn't add energy, information, or emotion — cut it.`, 
		p.hostAName, p.hostBName)

	user := fmt.Sprintf(`ORIGINAL SCRIPT:
---
%s
---

EDITORIAL CRITIQUE:
---
%s
---

SOURCE MATERIAL (for fact-checking):
---
%s
---

Rewrite the complete revised script now.`, draft, critique, truncate(source, 50000))

	return p.client.CompleteWithSystem(p.scriptModel, system, user, 8192, 0.75)
}

func (p *Pipeline) injectDisfluencies(script string) (string, error) {
	system := fmt.Sprintf(`You are a dialogue naturalizer. Take this podcast script and:

1. Add natural disfluencies: "um", "uh", "like", "you know", "I mean", "right?", 
   "so basically", "wait wait wait", trailing off mid-sentence, self-corrections 
   ("the first — well actually the second"), false starts.
2. Add reactions and backchannels: "mm-hmm", "right", "totally", "oh wow", "huh", 
   "no way", "that's crazy", "OK OK OK".
3. Add interruptions where natural — sometimes a host is so excited they cut in.
4. Add moments of thinking: "let me think about that...", "how do I explain this...", 
   "OK so imagine..."
5. Vary the energy — some moments should feel rapid-fire, others contemplative.

IMPORTANT: Don't overdo it. Real people use maybe 1-2 disfluencies per sentence, not every sentence. 
Some lines should stay clean and impactful.

Output as a JSON array with this exact structure:
{
  "title": "Episode title",
  "summary": "One-line summary",
  "lines": [
    {
      "speaker": "A",
      "text": "The actual dialogue with disfluencies",
      "emotion": "excited|thoughtful|surprised|amused|serious|curious|skeptical",
      "overlap": false,
      "pause_ms": 0
    }
  ],
  "estimated_duration_seconds": 600
}

Speaker "A" = %s (the curious one), Speaker "B" = %s (the expert).
Use pause_ms to add beats: 0 for quick back-and-forth, 300-500 for thinking pauses, 
800-1200 for dramatic beats or topic transitions.
Set overlap=true when a host interrupts or talks over the end of the previous line.

Return ONLY valid JSON. No markdown fences, no commentary.`, p.hostAName, p.hostBName)

	user := fmt.Sprintf("Naturalize this script and output as JSON:\n\n---\n%s\n---", script)

	return p.client.CompleteWithSystem(p.critiqueModel, system, user, 12000, 0.7)
}

// parseDialogue extracts structured dialogue from LLM JSON output
func parseDialogue(raw string) (*Dialogue, error) {
	// Clean up common LLM quirks
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var d Dialogue
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		// Try to find JSON in the response
		start := strings.Index(raw, "{")
		end := strings.LastIndex(raw, "}")
		if start >= 0 && end > start {
			if err2 := json.Unmarshal([]byte(raw[start:end+1]), &d); err2 != nil {
				return nil, fmt.Errorf("could not parse dialogue JSON: %w\nRaw output:\n%s", err2, raw[:min(500, len(raw))])
			}
		} else {
			return nil, fmt.Errorf("no JSON found in output: %w", err)
		}
	}

	if len(d.Lines) == 0 {
		return nil, fmt.Errorf("parsed dialogue has no lines")
	}

	// Estimate duration if not provided (roughly 150 wpm)
	if d.Duration == 0 {
		wordCount := 0
		for _, line := range d.Lines {
			wordCount += len(strings.Fields(line.Text))
		}
		d.Duration = (wordCount * 60) / 150
	}

	return &d, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n\n[... truncated for length ...]"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

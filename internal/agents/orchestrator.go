package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/arinorr/shinobi/internal/gh"
)

const previewMaxBytes = 500

// Feedback is the structured output from a single agent review.
type Feedback struct {
	Role     string   `json:"role"`
	Findings []Finding `json:"findings"`
}

// Finding is a single observation from an agent.
type Finding struct {
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Severity string `json:"severity"` // info, warning, critical
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
}

// ReviewResult is the synthesized output from all agents.
type ReviewResult struct {
	Summary     string
	Suggestions []gh.Suggestion
}

// Options controls orchestrator behavior.
type Options struct {
	Verbose bool
	DryRun  bool
}

// Orchestrator manages the multi-agent review process.
type Orchestrator struct {
	roles  []Role
	opts   Options
	skills map[string]string // immutable after construction
}

// NewOrchestrator creates a new orchestrator with the given roles.
// All skill files are loaded eagerly so the map is immutable during review.
func NewOrchestrator(roles []Role, opts Options) (*Orchestrator, error) {
	exeDir := ""
	if exePath, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exePath)
	}

	skills := make(map[string]string, len(roles))
	for _, r := range roles {
		data, err := readSkillFile(r.SkillFile, exeDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load skill for %s: %w", r.Name, err)
		}
		skills[r.Slug] = string(data)
	}

	return &Orchestrator{
		roles:  roles,
		opts:   opts,
		skills: skills,
	}, nil
}

// readSkillFile tries to read a skill file, falling back to exe-relative path.
func readSkillFile(path string, exeDir string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil && exeDir != "" {
		data, err = os.ReadFile(filepath.Join(exeDir, path))
	}
	return data, err
}

// Review runs all agents in parallel and synthesizes their feedback.
func (o *Orchestrator) Review(pr *gh.PR) (*ReviewResult, error) {
	if o.opts.DryRun {
		return o.dryRun(pr)
	}

	// Phase 1: Dispatch all agents in parallel.
	feedbacks, err := o.dispatchAgents(pr)
	if err != nil {
		return nil, err
	}

	// Phase 2: Synthesize feedback.
	return o.synthesize(pr, feedbacks)
}

func (o *Orchestrator) dryRun(pr *gh.PR) (*ReviewResult, error) {
	if len(o.roles) == 0 {
		return nil, fmt.Errorf("no roles selected")
	}

	fmt.Println("🏜️  DRY RUN — no agents will be called")
	fmt.Printf("Diff size: %d bytes\n\n", len(pr.Diff))

	fmt.Printf("Agents that would run (%d):\n", len(o.roles))
	for _, r := range o.roles {
		fmt.Printf("   • %s — %s\n", r.Name, r.Description)
		if o.opts.Verbose {
			fmt.Printf("     Skill file: %s (%d bytes)\n", r.SkillFile, len(o.skill(r)))
		}
	}

	fmt.Printf("\nSample prompt (for %s):\n", o.roles[0].Name)
	fmt.Println("───────────────────────────────────────")
	prompt := buildAgentPrompt(o.roles[0], pr)
	const previewMaxBytes = 500
	if len(prompt) > previewMaxBytes {
		fmt.Printf("%s\n... (%d bytes total)\n", truncateUTF8(prompt, previewMaxBytes), len(prompt))
	} else {
		fmt.Println(prompt)
	}
	fmt.Println("───────────────────────────────────────")

	return &ReviewResult{
		Summary: "[dry run — no review performed]",
	}, nil
}

func (o *Orchestrator) dispatchAgents(pr *gh.PR) ([]Feedback, error) {
	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		feedbacks []Feedback
		errs      []error
	)

	// agentLog collects output per-agent to avoid interleaved printing.
	type agentLog struct {
		role   string
		output string
	}
	var logs []agentLog

	for _, role := range o.roles {
		wg.Add(1)
		go func(r Role) {
			defer wg.Done()

			var buf strings.Builder
			fmt.Fprintf(&buf, "   🔍 [%s] reviewing...\n", r.Name)
			fb, err := o.runAgent(r, pr, &buf)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("[%s] %w", r.Name, err))
				fmt.Fprintf(&buf, "   ⚠️  [%s] %v\n", r.Name, err)
			} else {
				feedbacks = append(feedbacks, *fb)
				fmt.Fprintf(&buf, "   ✅ [%s] found %d findings\n", r.Name, len(fb.Findings))
			}
			logs = append(logs, agentLog{role: r.Name, output: buf.String()})
		}(role)
	}

	wg.Wait()

	// Print all agent output atomically, one agent at a time.
	for _, l := range logs {
		fmt.Print(l.output)
	}

	if len(feedbacks) == 0 {
		return nil, fmt.Errorf("all agents failed")
	}

	return feedbacks, nil
}

func (o *Orchestrator) runAgent(role Role, pr *gh.PR, log *strings.Builder) (*Feedback, error) {
	prompt := buildAgentPrompt(role, pr)
	skill := o.skill(role)

	if o.opts.Verbose {
		fmt.Fprintf(log, "   📝 [%s] prompt: %d bytes, skill: %d bytes\n", role.Name, len(prompt), len(skill))
	}

	// Run claude with the role's skill and structured output.
	start := time.Now()
	cmd := exec.Command("claude",
		"--print",
		"--output-format", "json",
		"--append-system-prompt", skill,
		"-p", prompt,
	)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	elapsed := time.Since(start)

	if err != nil {
		if o.opts.Verbose {
			fmt.Fprintf(log, "   ❌ [%s] failed in %s: %v\n", role.Name, elapsed.Round(time.Millisecond), err)
			if s := stderr.String(); s != "" {
				fmt.Fprintf(log, "   📋 [%s] stderr: %s\n", role.Name, truncateUTF8(s, 300))
			}
		}
		return nil, fmt.Errorf("claude command failed: %w", err)
	}

	if o.opts.Verbose {
		fmt.Fprintf(log, "   ⏱️  [%s] completed in %s (%d bytes response)\n", role.Name, elapsed.Round(time.Millisecond), len(out))
	}

	// Parse the agent's JSON response.
	var response struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		if o.opts.Verbose {
			fmt.Fprintf(log, "   🔬 [%s] raw response: %s\n", role.Name, truncateUTF8(string(out), previewMaxBytes))
		}
		return nil, fmt.Errorf("failed to parse claude response: %w", err)
	}

	// Extract the structured feedback from the response.
	fb, err := parseFeedback(role.Slug, response.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse feedback: %w", err)
	}

	return fb, nil
}

func (o *Orchestrator) synthesize(pr *gh.PR, feedbacks []Feedback) (*ReviewResult, error) {
	synthesisPrompt := buildSynthesisPrompt(pr, feedbacks)

	cmd := exec.Command("claude",
		"--print",
		"-p", synthesisPrompt,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("synthesis failed: %w", err)
	}

	// Parse synthesis response.
	var response struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		// If not JSON, use raw output.
		return &ReviewResult{
			Summary: string(out),
		}, nil
	}

	return &ReviewResult{
		Summary: response.Result,
	}, nil
}

func buildAgentPrompt(role Role, pr *gh.PR) string {
	return fmt.Sprintf(`Review the following pull request changes through your specialized lens.

PR Title: %s
PR Description: %s

Diff:
%s

Respond with a JSON array of findings. Each finding should have:
- "file": the file path
- "line": the line number (0 if not applicable)
- "severity": "info", "warning", or "critical"
- "summary": a brief one-line summary
- "detail": a detailed explanation with suggested improvement

Output ONLY valid JSON in this format:
{"findings": [...]}`, pr.Title, pr.Body, pr.Diff)
}

func buildSynthesisPrompt(pr *gh.PR, feedbacks []Feedback) string {
	var parts []string
	for _, fb := range feedbacks {
		data, _ := json.Marshal(fb)
		parts = append(parts, string(data))
	}

	return fmt.Sprintf(`You are the Shinobi Council Synthesizer. Multiple specialist agents have reviewed a PR. Your job is to:

1. Review all feedback from each specialist
2. Identify the most important suggestions
3. Resolve any conflicts between agents (weigh pros and cons)
4. Produce a clear, actionable summary

If agents disagree, explain the tradeoff rather than picking a side (unless one is clearly correct).

Group suggestions by file, then by priority (critical > warning > info).

PR Title: %s

Agent Feedback:
%s

Produce a well-formatted markdown summary with:
- An overall assessment (1-2 sentences)
- Grouped suggestions by file
- Any tradeoffs or conflicts noted
- A final verdict (approve with suggestions, request changes, or needs discussion)`,
		pr.Title, strings.Join(parts, "\n\n---\n\n"))
}

func (o *Orchestrator) skill(role Role) string {
	return o.skills[role.Slug]
}

func parseFeedback(role string, response string) (*Feedback, error) {
	response = strings.TrimSpace(response)

	// Try direct parse first.
	var result struct {
		Findings []Finding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(response), &result); err == nil {
		return &Feedback{Role: role, Findings: result.Findings}, nil
	}

	// Extract JSON from markdown code blocks.
	if idx := strings.Index(response, "```"); idx != -1 {
		lines := strings.Split(response, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				if inBlock {
					// End of block — try to parse what we collected.
					candidate := strings.Join(jsonLines, "\n")
					if err := json.Unmarshal([]byte(candidate), &result); err == nil {
						return &Feedback{Role: role, Findings: result.Findings}, nil
					}
					jsonLines = nil
				}
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
	}

	// Last resort: find {"findings" marker and extract from there to the last }.
	if start := strings.Index(response, `{"findings"`); start != -1 {
		if end := strings.LastIndex(response, "}"); end > start {
			candidate := response[start : end+1]
			if err := json.Unmarshal([]byte(candidate), &result); err == nil {
				return &Feedback{Role: role, Findings: result.Findings}, nil
			}
		}
	}

	return nil, fmt.Errorf("could not extract JSON from response\nRaw: %s", truncateUTF8(response, 200))
}

// truncateUTF8 truncates s to at most maxBytes without splitting a UTF-8 character.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk back from maxBytes to avoid splitting a multi-byte rune.
	for maxBytes > 0 && maxBytes < len(s) && s[maxBytes]&0xC0 == 0x80 {
		maxBytes--
	}
	return s[:maxBytes]
}

package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/arinorr/shinobi/internal/gh"
)

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

// Orchestrator manages the multi-agent review process.
type Orchestrator struct {
	roles []Role
}

// NewOrchestrator creates a new orchestrator with the given roles.
func NewOrchestrator(roles []Role) *Orchestrator {
	return &Orchestrator{roles: roles}
}

// Review runs all agents in parallel and synthesizes their feedback.
func (o *Orchestrator) Review(pr *gh.PR) (*ReviewResult, error) {
	// Phase 1: Dispatch all agents in parallel.
	feedbacks, err := o.dispatchAgents(pr)
	if err != nil {
		return nil, err
	}

	// Phase 2: Synthesize feedback.
	return o.synthesize(pr, feedbacks)
}

func (o *Orchestrator) dispatchAgents(pr *gh.PR) ([]Feedback, error) {
	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		feedbacks []Feedback
		errs      []error
	)

	for _, role := range o.roles {
		wg.Add(1)
		go func(r Role) {
			defer wg.Done()

			fmt.Printf("   🔍 [%s] reviewing...\n", r.Name)
			fb, err := o.runAgent(r, pr)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("[%s] %w", r.Name, err))
				return
			}
			feedbacks = append(feedbacks, *fb)
			fmt.Printf("   ✅ [%s] found %d findings\n", r.Name, len(fb.Findings))
		}(role)
	}

	wg.Wait()

	if len(errs) > 0 {
		// Report errors but continue with whatever feedback we got.
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "   ⚠️  %v\n", e)
		}
	}

	if len(feedbacks) == 0 {
		return nil, fmt.Errorf("all agents failed")
	}

	return feedbacks, nil
}

func (o *Orchestrator) runAgent(role Role, pr *gh.PR) (*Feedback, error) {
	prompt := buildAgentPrompt(role, pr)

	// Run claude with the role's skill and structured output.
	cmd := exec.Command("claude",
		"--print",
		"--output-format", "json",
		"--append-system-prompt", loadSkill(role),
		"-p", prompt,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("claude command failed: %w", err)
	}

	// Parse the agent's JSON response.
	var response struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
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
		data, _ := json.MarshalIndent(fb, "", "  ")
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

func loadSkill(role Role) string {
	// Load the skill content from the embedded skill file.
	// For now, return inline skill descriptions.
	skills := map[string]string{
		"know-it-all": "You are the Know-It-All reviewer. You are obsessive about best practices, language idioms, and code smells. You know every linting rule, every style guide, and every anti-pattern. Your job is to find code that doesn't follow established best practices for the language it's written in.",
		"architect":   "You are the Architect reviewer. You think in systems, patterns, and abstractions. You evaluate whether code changes fit the overall system architecture, whether abstractions are clean, whether the code scales, and whether it introduces technical debt. You care about separation of concerns, dependency direction, and API boundaries.",
		"solver":      "You are the Solver reviewer. You focus on whether the PR actually solves the problem it claims to solve. You check edge cases, error handling, boundary conditions, and whether the solution is complete. You ask: does this cover all the bases? What could go wrong? What was missed?",
		"editor":      "You are the Editor reviewer. You care about readability, clarity, and simplicity. Code should be easy to follow, well-named, and not unnecessarily complex. Some duplication is acceptable if it aids readability, but excessive copy-paste is a smell. You want code that a new team member could understand quickly.",
		"optimizer":   "You are the Optimizer reviewer. You focus on performance, algorithmic complexity, and resource usage. You analyze Big-O complexity, look for unnecessary allocations, N+1 queries, missing indexes, and opportunities to improve performance. You consider whether optimizations are worth the complexity tradeoff.",
		"sentinel":    "You are the Sentinel reviewer. You are a security specialist. You hunt for vulnerabilities, dangerous code patterns, and potential attack vectors. You look for injection flaws, auth/authz gaps, secrets in code, unsafe deserialization, SSRF, path traversal, and anything from the OWASP Top 10. You flag code that could be exploited and suggest secure alternatives.",
	}
	return skills[role.Slug]
}

func parseFeedback(role string, response string) (*Feedback, error) {
	// Try to extract JSON from the response.
	response = strings.TrimSpace(response)

	// Handle cases where the response is wrapped in markdown code blocks.
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		response = strings.Join(jsonLines, "\n")
	}

	var result struct {
		Findings []Finding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w\nRaw: %s", err, response[:min(len(response), 200)])
	}

	return &Feedback{
		Role:     role,
		Findings: result.Findings,
	}, nil
}

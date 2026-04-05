# Agent Blind Spots: Why 7 Agents Missed What html/template Already Solves

## The Incident

PR #18 added a `sanitize.ForHTML()` function to escape HTML characters in LLM
response output, attempting to prevent XSS in HTML reports. Over 3 review
iterations, the agents kept flagging security issues with the sanitization —
incomplete escaping, wrong rendering context, inconsistent application — each
time prompting fixes that introduced new findings.

After 3 rounds of churn, the actual answer was: **`html/template` already
auto-escapes all template variables.** The sanitize package was solving a
problem that didn't exist, and every "fix" made things worse.

None of the 7 agents — including the Know-It-All (best practices), the
Sentinel (security), and the Architect (design) — identified this.

## Why It Happened

### 1. Agents review the diff, not the architecture

Each agent sees the changed lines but not the full data flow. They saw
`sanitize.ForHTML()` being added and asked "is this sanitization complete?"
but couldn't see that the HTML report template (in a different file, not in
the diff) uses `html/template` which auto-escapes all variables.

The data flow is:

```
LLM response (untrusted)
  → parseFeedback() → Finding struct
    → report.HTML() → html/template (AUTO-ESCAPES everything)
    → report.Markdown() → plain text (no HTML execution)
    → report.JSON() → json.Marshal (encodes properly)
    → stderr logging → terminal (not an HTML context)
```

The HTML rendering is already safe. The sanitize package was adding escaping
to terminal/error paths where it produced confusing `&lt;` output, while
the actual HTML path was already protected by the template engine.

### 2. The Know-It-All doesn't know Go-specific security idioms

The Know-It-All's Go skill module doesn't mention that `html/template`
auto-escapes — one of Go's most well-known security features and a key
difference from `text/template`. If the skill file included this knowledge,
the agent might have reasoned: "they're adding HTML escaping, but the
reports use html/template which already does this."

### 3. No agent challenges findings

All 7 agents are advocates — they look for problems. Nobody asks the
meta-question: "Is this finding actually correct? Does the codebase already
handle this?" This is like having 5 code reviewers who each leave a
"you should do X" comment without checking if X is already done.

### 4. No "full picture" agent

No agent can read beyond the diff to understand the full system. An agent
that traced the data flow from LLM output through parseFeedback through the
report templates would see that html/template is the sanitizer. Current
agents can't do this — they only see the diff.

## Proposed Solutions

### Short-term: Update agent skill files

Add Go-specific security knowledge to the Know-It-All's Go skill module:

- `html/template` auto-escapes all variables (unlike `text/template`)
- Don't add manual HTML escaping when html/template is the renderer
- Sanitize at the rendering boundary, not at data creation
- Error messages flow to terminals, not HTML — don't HTML-escape them

This is worth doing but fragile — you can't enumerate every framework-level
guarantee across every language in skill files.

### Medium-term: Verification Agent (Fact Checker + Codebase Expert)

The originally planned "Fact Checker" and "Codebase Expert" roles collapse
into a single agent: a **Verification Agent** that reads the actual codebase
to validate findings. A Fact Checker that can't read the code is just another
opinion-generator. A Codebase Expert that doesn't verify claims is just
another advocate. They're the same role.

This agent runs after the 7 specialists produce findings. For each finding,
it reads the relevant code and checks whether the claim holds:

- "Finding says 'output flows unsanitized to HTML' — is html/template
  used for rendering? Yes → finding is incorrect"
- "Finding says 'add validation here' — is validation already done
  upstream? Check the call chain"
- "Finding says 'this could XSS' — trace the data flow to the actual
  rendering context"

#### Preventing hallucinated verification

An LLM saying "I checked and html/template is used" is worthless if it
didn't actually look. The Verification Agent must output **specific
evidence** — file path, line number, the actual code — so a human can
spot-check the verification, not just the conclusion:

```
Finding: "LLM output flows unsanitized to HTML reports"
Verdict: INCORRECT
Evidence:
  - internal/report/report.go:8 imports html/template (not text/template)
  - html/template auto-escapes all {{.Variable}} expressions
  - PR uses html/template for all HTML rendering (line 325)
```

A human can verify that evidence in 2 seconds. If the Verification Agent
can't produce concrete file:line evidence, the original finding stands
uncontested.

### Long-term: Data flow annotations

The data flow diagram in section 1 should live somewhere agents can
reference — not just in a post-mortem. If security-sensitive data flows
were annotated in the codebase (e.g., a `SECURITY.md` or inline comments
in a specific format), agents could check claims structurally rather than
reasoning from the diff alone.

Example annotation format:

```
// TRUST-BOUNDARY: LLM response → parseFeedback → Finding struct
// SANITIZATION: html/template auto-escapes all Finding fields in report.HTML()
// SAFE-CONTEXTS: HTML report (auto-escaped), Markdown (plain text), JSON (encoded)
// UNSAFE-IF: text/template is used instead of html/template
```

This shifts security verification from "does the LLM know this framework?"
to "can the agent read an annotation?" — a much more reliable operation.

## Lessons Learned

1. **More review iterations don't converge on correctness** if the agents
   have the same blind spot. Three rounds produced progressively worse code
   because each round "fixed" findings that were themselves incorrect.

2. **Advocacy without verification is dangerous.** Seven agents unanimously
   agreeing on a finding doesn't make it correct — it means they all have
   the same information gap.

3. **The diff is not enough context for security findings.** Security
   requires understanding data flows end-to-end, not just the changed lines.

4. **Standard library knowledge matters.** `html/template` auto-escaping is
   fundamental Go knowledge. Agent skill files should encode language-specific
   security properties, not just coding style preferences.

5. **Verification > more opinions.** Adding a Fact Checker that validates
   findings against actual code is fundamentally more useful than adding
   more specialist agents that generate more opinions from the same limited
   context.

6. **Collapsed roles are clearer.** The Fact Checker and Codebase Expert
   are the same agent — one that reads the codebase to verify claims. Two
   separate roles would have overlapping concerns and unclear boundaries.

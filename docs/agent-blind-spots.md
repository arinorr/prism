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

The planned Codebase Expert role (backlog) would read beyond the diff to
understand the full system. It would trace the data flow from LLM output
through parseFeedback through the report templates and see that
html/template is the sanitizer. Current agents can't do this — they only
see the diff.

## Proposed Solutions

### Short-term: Update agent skill files

Add Go-specific security knowledge to the Know-It-All's Go skill module:

- `html/template` auto-escapes all variables (unlike `text/template`)
- Don't add manual HTML escaping when html/template is the renderer
- Sanitize at the rendering boundary, not at data creation
- Error messages flow to terminals, not HTML — don't HTML-escape them

### Medium-term: Add a Fact Checker agent

A post-review agent that runs after the 7 specialists produce findings.
For each finding, it checks whether the claim holds against the actual code:

- "Finding says 'output flows unsanitized to HTML' — is html/template
  used for rendering? Yes → finding is incorrect"
- "Finding says 'add validation here' — is validation already done
  upstream? Check the call chain"
- "Finding says 'this could XSS' — trace the data flow to the actual
  rendering context"

This agent would have access to the findings AND the codebase (not just
the diff), allowing it to verify claims rather than just generate them.

### Long-term: Codebase Expert role

An agent that reads the wider codebase to understand data flows, existing
patterns, and architectural decisions. This agent could catch cases where:

- A security mechanism already exists (html/template, bluemonday, etc.)
- A proposed fix conflicts with an existing design decision
- A finding describes a symptom but misidentifies the root cause

This requires the Agent SDK migration (backlog item) since it needs to
read files beyond the diff.

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

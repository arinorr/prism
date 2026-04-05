// Package sanitize provides functions for cleaning untrusted data before
// it reaches output contexts like HTML reports, GitHub comments, or logs.
//
// All LLM-generated content is untrusted — agents can produce arbitrary
// text including HTML tags, script injections, or misleading formatting.
// Use these functions at every boundary where untrusted data flows into
// a rendering context.
package sanitize

import "html"

// ForHTML escapes HTML-significant characters (&, <, >, ", ') so the
// string is safe to include in HTML reports, GitHub comments, or any
// context where HTML is interpreted. Uses the standard library's
// html.EscapeString which handles all five characters in the correct
// order (& first to prevent double-encoding).
func ForHTML(s string) string {
	return html.EscapeString(s)
}

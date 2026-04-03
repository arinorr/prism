// Package sizecheck provides advisory warnings for large PR diffs.
package sizecheck

import "fmt"

// Result describes the outcome of a diff size check.
type Result struct {
	DiffBytes    int
	Warn         bool
	SuggestChunk bool
	Message      string
}

// Check evaluates a diff size against the given thresholds.
// A threshold of 0 disables that check. The check is advisory only
// and never blocks execution.
func Check(diffBytes, warnThreshold, chunkThreshold int) Result {
	r := Result{DiffBytes: diffBytes}

	if chunkThreshold > 0 && diffBytes >= chunkThreshold {
		r.Warn = true
		r.SuggestChunk = true
		r.Message = fmt.Sprintf(
			"Large diff (%d KB). Consider splitting this PR into smaller pieces for more effective review.",
			diffBytes/1024)
		return r
	}

	if warnThreshold > 0 && diffBytes >= warnThreshold {
		r.Warn = true
		r.Message = fmt.Sprintf(
			"Diff is %d KB — review quality may be reduced for very large diffs.",
			diffBytes/1024)
		return r
	}

	return r
}

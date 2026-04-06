package sizecheck

import "testing"

func TestCheck_BelowThreshold(t *testing.T) {
	t.Parallel()
	r := Check(100000, 150000, 300000)
	if r.Warn || r.SuggestChunk {
		t.Error("expected no warning below threshold")
	}
}

func TestCheck_BetweenThresholds(t *testing.T) {
	t.Parallel()
	r := Check(200000, 150000, 300000)
	if !r.Warn {
		t.Error("expected warning between thresholds")
	}
	if r.SuggestChunk {
		t.Error("should not suggest chunk between thresholds")
	}
	if r.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCheck_AboveBothThresholds(t *testing.T) {
	t.Parallel()
	r := Check(400000, 150000, 300000)
	if !r.Warn {
		t.Error("expected warning above both thresholds")
	}
	if !r.SuggestChunk {
		t.Error("expected chunk suggestion above chunk threshold")
	}
}

func TestCheck_ZeroThresholds(t *testing.T) {
	t.Parallel()
	r := Check(999999, 0, 0)
	if r.Warn || r.SuggestChunk {
		t.Error("zero thresholds should disable checks")
	}
}

func TestCheck_ExactWarnBoundary(t *testing.T) {
	t.Parallel()
	r := Check(150000, 150000, 300000)
	if !r.Warn {
		t.Error("expected warning at exact boundary")
	}
}

func TestCheck_ExactChunkBoundary(t *testing.T) {
	t.Parallel()
	r := Check(300000, 150000, 300000)
	if !r.SuggestChunk {
		t.Error("expected chunk suggestion at exact boundary")
	}
}

func TestCheck_DiffBytesStored(t *testing.T) {
	t.Parallel()
	r := Check(12345, 0, 0)
	if r.DiffBytes != 12345 {
		t.Errorf("expected DiffBytes 12345, got %d", r.DiffBytes)
	}
}

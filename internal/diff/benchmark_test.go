package diff

import (
	"fmt"
	"strings"
	"testing"
)

// buildDiff creates a synthetic diff with the given number of files and
// lines per file. Produces realistic unified diff format.
func buildDiff(files, linesPerFile int) string {
	var b strings.Builder
	for f := 0; f < files; f++ {
		name := fmt.Sprintf("pkg/module%d/handler.go", f)
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", name, name)
		fmt.Fprintf(&b, "--- a/%s\n", name)
		fmt.Fprintf(&b, "+++ b/%s\n", name)
		fmt.Fprintf(&b, "@@ -1,%d +1,%d @@\n", linesPerFile, linesPerFile+2)
		for l := 0; l < linesPerFile; l++ {
			switch {
			case l%5 == 0:
				fmt.Fprintf(&b, "+\tnewLine%d := process(input)\n", l)
			case l%7 == 0:
				fmt.Fprintf(&b, "-\toldLine%d := legacy(input)\n", l)
			default:
				fmt.Fprintf(&b, " \texistingLine%d()\n", l)
			}
		}
	}
	return b.String()
}

func BenchmarkCompress_SmallDiff(b *testing.B) {
	diff := buildDiff(5, 20) // ~100 lines across 5 files
	opts := DefaultOptions()
	b.ResetTimer()
	for range b.N {
		Compress(diff, opts)
	}
}

func BenchmarkCompress_MediumDiff(b *testing.B) {
	diff := buildDiff(30, 50) // ~1500 lines across 30 files
	opts := DefaultOptions()
	b.ResetTimer()
	for range b.N {
		Compress(diff, opts)
	}
}

func BenchmarkCompress_LargeDiff(b *testing.B) {
	diff := buildDiff(100, 100) // ~10000 lines across 100 files
	opts := DefaultOptions()
	b.ResetTimer()
	for range b.N {
		Compress(diff, opts)
	}
}

func BenchmarkCompress_WithLockFiles(b *testing.B) {
	// Mix of real files and lock files that should be stripped.
	var sb strings.Builder
	sb.WriteString(buildDiff(10, 30))
	// Add lock file diffs.
	for _, name := range []string{"go.sum", "package-lock.json", "yarn.lock", "Cargo.lock"} {
		fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", name, name)
		fmt.Fprintf(&sb, "--- a/%s\n", name)
		fmt.Fprintf(&sb, "+++ b/%s\n", name)
		fmt.Fprintf(&sb, "@@ -1,100 +1,120 @@\n")
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&sb, "+dependency-hash-%d\n", i)
		}
	}
	diff := sb.String()
	opts := DefaultOptions()
	b.ResetTimer()
	for range b.N {
		Compress(diff, opts)
	}
}

package parse

import (
	"fmt"
	"strings"
	"testing"
)

// Parser benchmarks measuring throughput, allocations, and correctness.
// Standard parser metrics: bytes/sec, allocs/op, and symbol extraction accuracy.

// generateTSSource creates a realistic TypeScript file with known declaration counts.
func generateTSSource(numFunctions, numClasses, numInterfaces int) []byte {
	var b strings.Builder

	b.WriteString("// Generated TypeScript source for benchmarking.\n\n")

	for i := 0; i < numInterfaces; i++ {
		fmt.Fprintf(&b, "export interface Config%d {\n", i)
		fmt.Fprintf(&b, "  host: string;\n")
		fmt.Fprintf(&b, "  port: number;\n")
		fmt.Fprintf(&b, "  debug?: boolean;\n")
		fmt.Fprintf(&b, "}\n\n")
	}

	for i := 0; i < numFunctions; i++ {
		fmt.Fprintf(&b, "export async function handleRequest%d(req: Request): Promise<Response> {\n", i)
		fmt.Fprintf(&b, "  const data = await req.json();\n")
		fmt.Fprintf(&b, "  if (!data) {\n")
		fmt.Fprintf(&b, "    return new Response('bad request', { status: 400 });\n")
		fmt.Fprintf(&b, "  }\n")
		fmt.Fprintf(&b, "  return new Response(JSON.stringify(data));\n")
		fmt.Fprintf(&b, "}\n\n")
	}

	for i := 0; i < numClasses; i++ {
		fmt.Fprintf(&b, "export class Service%d {\n", i)
		fmt.Fprintf(&b, "  private db: Database;\n\n")
		fmt.Fprintf(&b, "  constructor(db: Database) {\n")
		fmt.Fprintf(&b, "    this.db = db;\n")
		fmt.Fprintf(&b, "  }\n\n")
		fmt.Fprintf(&b, "  async findById(id: string): Promise<Entity> {\n")
		fmt.Fprintf(&b, "    return this.db.query(id);\n")
		fmt.Fprintf(&b, "  }\n\n")
		fmt.Fprintf(&b, "  async delete(id: string): Promise<void> {\n")
		fmt.Fprintf(&b, "    await this.db.remove(id);\n")
		fmt.Fprintf(&b, "  }\n")
		fmt.Fprintf(&b, "}\n\n")
	}

	return []byte(b.String())
}

// --- Throughput benchmarks (bytes/sec). ---

// BenchmarkTSScanner_Throughput_1KB measures parse throughput on a ~1KB file.
func BenchmarkTSScanner_Throughput_1KB(b *testing.B) {
	src := generateTSSource(5, 1, 2) // ~1KB.
	scanner := NewTSScanner()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanner.Scan("file.ts", src)
	}
}

// BenchmarkTSScanner_Throughput_10KB measures parse throughput on a ~10KB file.
func BenchmarkTSScanner_Throughput_10KB(b *testing.B) {
	src := generateTSSource(30, 10, 15) // ~10KB.
	scanner := NewTSScanner()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanner.Scan("file.ts", src)
	}
}

// BenchmarkTSScanner_Throughput_50KB measures parse throughput on a ~50KB file.
func BenchmarkTSScanner_Throughput_50KB(b *testing.B) {
	src := generateTSSource(150, 40, 60) // ~50KB.
	scanner := NewTSScanner()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanner.Scan("file.ts", src)
	}
}

// BenchmarkTSScanner_Throughput_100KB measures parse throughput on a ~100KB file.
func BenchmarkTSScanner_Throughput_100KB(b *testing.B) {
	src := generateTSSource(300, 80, 120) // ~100KB.
	scanner := NewTSScanner()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanner.Scan("file.ts", src)
	}
}

// --- Go scanner comparison (for context). ---

// BenchmarkGoScanner_Throughput_10KB provides a reference point.
func BenchmarkGoScanner_Throughput_10KB(b *testing.B) {
	var buf strings.Builder
	buf.WriteString("package main\n\n")
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&buf, "func Handler%d() error { return nil }\n\n", i)
	}
	for i := 0; i < 15; i++ {
		fmt.Fprintf(&buf, "type Service%d struct{}\n", i)
		fmt.Fprintf(&buf, "func (s *Service%d) Find() {}\n", i)
		fmt.Fprintf(&buf, "func (s *Service%d) Delete() {}\n\n", i)
	}
	src := []byte(buf.String())
	scanner := GoScanner{}
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanner.Scan("main.go", src)
	}
}

// --- Correctness verification benchmark. ---

// BenchmarkTSScanner_Correctness verifies symbol count matches expected declarations.
func BenchmarkTSScanner_Correctness(b *testing.B) {
	nFunc, nClass, nIface := 50, 15, 20
	src := generateTSSource(nFunc, nClass, nIface)
	scanner := NewTSScanner()
	symbols := scanner.Scan("correctness.ts", src)

	// Expected: functions + classes + methods (findById + delete per class) + interfaces.
	// Constructors are skipped.
	expectedFuncs := nFunc
	expectedClasses := nClass
	expectedMethods := nClass * 2 // findById + delete.
	expectedIfaces := nIface
	expectedTotal := expectedFuncs + expectedClasses + expectedMethods + expectedIfaces

	if len(symbols) != expectedTotal {
		b.Errorf("symbol count mismatch: got %d, want %d (funcs=%d, classes=%d, methods=%d, ifaces=%d)",
			len(symbols), expectedTotal, expectedFuncs, expectedClasses, expectedMethods, expectedIfaces)

		// Report breakdown for debugging.
		counts := map[SymbolKind]int{}
		for _, s := range symbols {
			counts[s.Kind]++
		}
		b.Logf("breakdown: %v", counts)
	}
}

// --- Error recovery benchmark. ---

// BenchmarkTSScanner_ErrorRecovery measures parse time on broken code.
func BenchmarkTSScanner_ErrorRecovery(b *testing.B) {
	// Intentionally broken TS — missing braces, incomplete expressions.
	src := []byte(`
export function broken(x: string) {
  if (x) {
    const y = x.

export class Incomplete {
  method() {
    return this.

interface MissingBrace {
  field: string
`)
	scanner := NewTSScanner()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanner.Scan("broken.ts", src)
	}
}

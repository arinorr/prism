package index

import "testing"

func TestGoScanner(t *testing.T) {
	src := []byte(`package example

import "fmt"

// Config holds server configuration.
type Config struct {
	Host string
	Port int
}

// Handler is the request handler interface.
type Handler interface {
	ServeHTTP(w Writer, r *Request)
}

// Alias is a simple type alias.
type Alias = string

func NewConfig() *Config {
	return &Config{Host: "localhost", Port: 8080}
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c Config) String() string {
	return c.Addr()
}
`)

	scanner := GoScanner{}
	symbols := scanner.Scan("example.go", src)

	// Expect: Config (struct), Handler (interface), Alias (type),
	// NewConfig (func), Addr (method on *Config), String (method on Config)
	if len(symbols) != 6 {
		t.Fatalf("expected 6 symbols, got %d: %+v", len(symbols), symbols)
	}

	tests := []struct {
		name     string
		kind     SymbolKind
		receiver string
	}{
		{"Config", KindType, ""},
		{"Handler", KindInterface, ""},
		{"Alias", KindType, ""},
		{"NewConfig", KindFunc, ""},
		{"Addr", KindMethod, "Config"},
		{"String", KindMethod, "Config"},
	}

	for i, tt := range tests {
		sym := symbols[i]
		if sym.Name != tt.name {
			t.Errorf("symbol[%d]: name = %q, want %q", i, sym.Name, tt.name)
		}
		if sym.Kind != tt.kind {
			t.Errorf("symbol[%d] %s: kind = %q, want %q", i, tt.name, sym.Kind, tt.kind)
		}
		if sym.Receiver != tt.receiver {
			t.Errorf("symbol[%d] %s: receiver = %q, want %q", i, tt.name, sym.Receiver, tt.receiver)
		}
		if sym.File != "example.go" {
			t.Errorf("symbol[%d] %s: file = %q, want %q", i, tt.name, sym.File, "example.go")
		}
		if sym.StartLine == 0 || sym.EndLine == 0 {
			t.Errorf("symbol[%d] %s: line range not set (start=%d, end=%d)", i, tt.name, sym.StartLine, sym.EndLine)
		}
		if sym.EndLine < sym.StartLine {
			t.Errorf("symbol[%d] %s: end (%d) < start (%d)", i, tt.name, sym.EndLine, sym.StartLine)
		}
	}
}

func TestGoScanner_LineRanges(t *testing.T) {
	src := []byte(`package example

func Short() {}

func Long(x int) int {
	if x > 0 {
		return x
	}
	return -x
}
`)

	scanner := GoScanner{}
	symbols := scanner.Scan("ranges.go", src)

	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}

	short := symbols[0]
	if short.Name != "Short" || short.StartLine != short.EndLine {
		t.Errorf("Short: expected single-line, got start=%d end=%d", short.StartLine, short.EndLine)
	}

	long := symbols[1]
	if long.Name != "Long" || long.EndLine <= long.StartLine {
		t.Errorf("Long: expected multi-line, got start=%d end=%d", long.StartLine, long.EndLine)
	}
}

func TestGoScanner_InvalidSource(t *testing.T) {
	src := []byte(`this is not valid go code`)
	scanner := GoScanner{}
	symbols := scanner.Scan("invalid.go", src)
	// Should not panic; may return partial results or nil.
	_ = symbols
}

package parse

import "testing"

func TestGoScanner_GenericReceiver(t *testing.T) {
	src := []byte(`package example

type Cache[K comparable, V any] struct {
	items map[K]V
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	v, ok := c.items[key]
	return v, ok
}

func (c *Cache[K, V]) Set(key K, value V) {
	c.items[key] = value
}
`)

	scanner := GoScanner{}
	symbols := scanner.Scan("cache.go", src)

	// Expect: Cache (type), Get (method), Set (method)
	if len(symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d: %+v", len(symbols), symbols)
	}

	if symbols[0].Name != "Cache" || symbols[0].Kind != KindType {
		t.Errorf("expected Cache type, got %s %s", symbols[0].Name, symbols[0].Kind)
	}

	if symbols[1].Name != "Get" || symbols[1].Kind != KindMethod || symbols[1].Receiver != "Cache" {
		t.Errorf("Get: kind=%s, receiver=%s, want method on Cache", symbols[1].Kind, symbols[1].Receiver)
	}

	if symbols[2].Name != "Set" || symbols[2].Kind != KindMethod || symbols[2].Receiver != "Cache" {
		t.Errorf("Set: kind=%s, receiver=%s, want method on Cache", symbols[2].Kind, symbols[2].Receiver)
	}
}

func TestGoScanner_EmptyFile(t *testing.T) {
	scanner := GoScanner{}
	symbols := scanner.Scan("empty.go", []byte("package main\n"))
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols from empty file, got %d", len(symbols))
	}
}

func TestGoScanner_MultipleTypesInOneBlock(t *testing.T) {
	src := []byte(`package example

type (
	Foo struct{ X int }
	Bar interface{ Do() }
	Baz = string
)
`)

	scanner := GoScanner{}
	symbols := scanner.Scan("types.go", src)

	if len(symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d", len(symbols))
	}

	tests := []struct {
		name string
		kind SymbolKind
	}{
		{"Foo", KindType},
		{"Bar", KindInterface},
		{"Baz", KindType},
	}

	for i, tt := range tests {
		if symbols[i].Name != tt.name || symbols[i].Kind != tt.kind {
			t.Errorf("symbol[%d]: got %s (%s), want %s (%s)", i, symbols[i].Name, symbols[i].Kind, tt.name, tt.kind)
		}
	}
}

func TestGoScanner_PointerReceiver(t *testing.T) {
	src := []byte(`package example

type Server struct{}

func (s *Server) Start() error { return nil }
func (s Server) Name() string { return "srv" }
`)

	scanner := GoScanner{}
	symbols := scanner.Scan("server.go", src)

	// Both methods should have Receiver = "Server" regardless of pointer
	for _, sym := range symbols {
		if sym.Kind == KindMethod && sym.Receiver != "Server" {
			t.Errorf("%s: receiver = %q, want Server", sym.Name, sym.Receiver)
		}
	}
}

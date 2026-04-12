package parse

import "testing"

func TestTSScanner_NestedClasses(t *testing.T) {
	src := []byte(`export class Outer {
  private inner: Inner;

  constructor() {
    this.inner = new Inner();
  }

  getInner() {
    return this.inner;
  }
}

export class Inner {
  value: number = 0;

  getValue() {
    return this.value;
  }
}
`)

	scanner := TSScanner{}
	symbols := scanner.Scan("nested.ts", src)

	names := map[string]bool{}
	for _, s := range symbols {
		names[s.Name] = true
	}

	for _, want := range []string{"Outer", "Inner", "getInner", "getValue"} {
		if !names[want] {
			t.Errorf("expected symbol %q", want)
		}
	}
}

func TestTSScanner_ArrowFunctionVariants(t *testing.T) {
	src := []byte(`export const simple = () => {};
export const withParams = (a: string, b: number) => {
  return a + b;
};
const noExport = (x: number) => x * 2;
export let mutable = () => {
  console.log("hi");
};
`)

	scanner := TSScanner{}
	symbols := scanner.Scan("arrows.ts", src)

	expected := []string{"simple", "withParams", "noExport", "mutable"}
	if len(symbols) != len(expected) {
		t.Fatalf("expected %d symbols, got %d", len(expected), len(symbols))
	}
	for i, want := range expected {
		if symbols[i].Name != want {
			t.Errorf("symbol[%d] = %q, want %q", i, symbols[i].Name, want)
		}
		if symbols[i].Kind != KindFunc {
			t.Errorf("symbol[%d] %s: kind = %q, want func", i, want, symbols[i].Kind)
		}
	}
}

func TestTSScanner_AbstractClass(t *testing.T) {
	src := []byte(`export abstract class BaseService {
  abstract handle(req: Request): Response;

  log(msg: string) {
    console.log(msg);
  }
}
`)

	scanner := TSScanner{}
	symbols := scanner.Scan("abstract.ts", src)

	if len(symbols) < 1 {
		t.Fatal("expected at least 1 symbol")
	}
	if symbols[0].Name != "BaseService" || symbols[0].Kind != KindClass {
		t.Errorf("expected BaseService class, got %s %s", symbols[0].Name, symbols[0].Kind)
	}
}

func TestTSScanner_MultiLineType(t *testing.T) {
	src := []byte(`export type Result =
  | { success: true; data: string }
  | { success: false; error: Error };

export function process(): Result {
  return { success: true, data: "ok" };
}
`)

	scanner := TSScanner{}
	symbols := scanner.Scan("multiline.ts", src)

	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}
	if symbols[0].Name != "Result" || symbols[0].Kind != KindType {
		t.Errorf("expected Result type, got %s %s", symbols[0].Name, symbols[0].Kind)
	}
	if symbols[1].Name != "process" || symbols[1].Kind != KindFunc {
		t.Errorf("expected process func, got %s %s", symbols[1].Name, symbols[1].Kind)
	}
}

func TestTSScanner_StringsWithBraces(t *testing.T) {
	// Braces inside strings should not confuse brace counting.
	src := []byte(`function render() {
  const template = "Hello {name}!";
  return template;
}

function other() {
  return 42;
}
`)

	scanner := TSScanner{}
	symbols := scanner.Scan("strings.ts", src)

	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}
	if symbols[0].EndLine != 4 {
		t.Errorf("render EndLine = %d, want 4", symbols[0].EndLine)
	}
}

func TestTSScanner_EmptyFile(t *testing.T) {
	scanner := TSScanner{}
	symbols := scanner.Scan("empty.ts", []byte("// just a comment\n"))
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(symbols))
	}
}

func TestTSScanner_ConstructorSkipped(t *testing.T) {
	src := []byte(`class Foo {
  constructor(private x: number) {}
  bar() { return this.x; }
}
`)

	scanner := TSScanner{}
	symbols := scanner.Scan("ctor.ts", src)

	for _, s := range symbols {
		if s.Name == "constructor" {
			t.Error("constructor should be skipped")
		}
	}
}

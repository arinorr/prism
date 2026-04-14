package parse

import (
	"testing"
)

// Comprehensive tree-sitter TS/JS parser tests.
// Covers every declaration form to ensure correctness.

// --- Function declarations. ---

func TestTS_FunctionDeclaration(t *testing.T) {
	t.Parallel()
	src := []byte(`function greet(name: string): string {
  return "hello " + name;
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "greet", KindFunc, 1, 3)
}

func TestTS_AsyncFunctionDeclaration(t *testing.T) {
	t.Parallel()
	src := []byte(`async function fetchData(url: string): Promise<Data> {
  const res = await fetch(url);
  return res.json();
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "fetchData", KindFunc, 1, 4)
}

func TestTS_ExportedFunction(t *testing.T) {
	t.Parallel()
	src := []byte(`export function handleRequest(req: Request): Response {
  return new Response("ok");
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "handleRequest", KindFunc, 1, 3)
}

func TestTS_ExportDefaultFunction(t *testing.T) {
	t.Parallel()
	src := []byte(`export default function createApp() {
  return {};
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "createApp", KindFunc, 1, 3)
}

func TestTS_GeneratorFunction(t *testing.T) {
	t.Parallel()
	src := []byte(`function* range(start: number, end: number) {
  for (let i = start; i < end; i++) {
    yield i;
  }
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "range", KindFunc, 1, 5)
}

// --- Arrow functions. ---

func TestTS_ConstArrowFunction(t *testing.T) {
	t.Parallel()
	src := []byte(`const processItems = (items: Item[]) => {
  return items.map(i => i.value);
};`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "processItems", KindFunc, 1, 3)
}

func TestTS_ExportConstArrowFunction(t *testing.T) {
	t.Parallel()
	src := []byte(`export const handler = async (req: Request) => {
  return new Response("ok");
};`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "handler", KindFunc, 1, 3)
}

func TestTS_LetArrowFunction(t *testing.T) {
	t.Parallel()
	src := []byte(`let callback = () => {
  console.log("called");
};`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "callback", KindFunc, 1, 3)
}

func TestTS_ArrowFunctionSingleExpression(t *testing.T) {
	t.Parallel()
	src := []byte(`const double = (x: number) => x * 2;`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "double", KindFunc, 1, 1)
}

func TestTS_ArrowFunctionNoParens(t *testing.T) {
	t.Parallel()
	src := []byte(`const identity = x => x;`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "identity", KindFunc, 1, 1)
}

// --- Class declarations. ---

func TestTS_ClassDeclaration(t *testing.T) {
	t.Parallel()
	src := []byte(`class UserService {
  private db: Database;

  constructor(db: Database) {
    this.db = db;
  }

  async findById(id: string): Promise<User> {
    return this.db.query(id);
  }

  delete(id: string): void {
    this.db.remove(id);
  }
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "UserService", KindClass)
	assertSymbolExists(t, symbols, "findById", KindMethod)
	assertSymbolExists(t, symbols, "delete", KindMethod)
	assertConstructorSkipped(t, symbols)
}

func TestTS_ExportDefaultClass(t *testing.T) {
	t.Parallel()
	src := []byte(`export default class App {
  run() { console.log("running"); }
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "App", KindClass)
	assertSymbolExists(t, symbols, "run", KindMethod)
}

func TestTS_AbstractClass(t *testing.T) {
	t.Parallel()
	src := []byte(`export abstract class BaseService {
  abstract handle(req: Request): Response;

  log(msg: string) {
    console.log(msg);
  }
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "BaseService", KindClass)
	assertSymbolExists(t, symbols, "log", KindMethod)
}

func TestTS_ClassWithStaticMethods(t *testing.T) {
	t.Parallel()
	src := []byte(`class Config {
  static fromEnv(): Config {
    return new Config();
  }

  static default(): Config {
    return new Config();
  }
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "Config", KindClass)
	assertSymbolExists(t, symbols, "fromEnv", KindMethod)
}

func TestTS_ClassWithGetterSetter(t *testing.T) {
	t.Parallel()
	src := []byte(`class Person {
  private _name: string = "";

  get name(): string {
    return this._name;
  }

  set name(value: string) {
    this._name = value;
  }
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "Person", KindClass)
	// Getters/setters are method_definitions in tree-sitter.
}

func TestTS_ClassExtendsImplements(t *testing.T) {
	t.Parallel()
	src := []byte(`class ApiController extends BaseController implements Loggable {
  handle(req: Request): Response {
    return new Response("ok");
  }
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "ApiController", KindClass)
	assertSymbolExists(t, symbols, "handle", KindMethod)
}

// --- Interface declarations. ---

func TestTS_InterfaceDeclaration(t *testing.T) {
	t.Parallel()
	src := []byte(`interface Config {
  host: string;
  port: number;
  debug?: boolean;
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "Config", KindInterface, 1, 5)
}

func TestTS_ExportedInterface(t *testing.T) {
	t.Parallel()
	src := []byte(`export interface ApiResponse<T> {
  data: T;
  error?: string;
  status: number;
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "ApiResponse", KindInterface)
}

func TestTS_InterfaceExtends(t *testing.T) {
	t.Parallel()
	src := []byte(`interface AdminUser extends User {
  permissions: string[];
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "AdminUser", KindInterface)
}

// --- Type alias declarations. ---

func TestTS_TypeAlias(t *testing.T) {
	t.Parallel()
	src := []byte(`type UserID = string;`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbol(t, symbols, "UserID", KindType, 1, 1)
}

func TestTS_UnionType(t *testing.T) {
	t.Parallel()
	src := []byte(`type Result =
  | { success: true; data: string }
  | { success: false; error: Error };`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "Result", KindType)
}

func TestTS_GenericType(t *testing.T) {
	t.Parallel()
	src := []byte(`type AsyncResult<T, E = Error> = Promise<Result<T, E>>;`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "AsyncResult", KindType)
}

func TestTS_MappedType(t *testing.T) {
	t.Parallel()
	src := []byte(`type Readonly<T> = {
  readonly [P in keyof T]: T[P];
};`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "Readonly", KindType)
}

func TestTS_ConditionalType(t *testing.T) {
	t.Parallel()
	src := []byte(`type NonNullable<T> = T extends null | undefined ? never : T;`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "NonNullable", KindType)
}

func TestTS_TemplateLiteralType(t *testing.T) {
	t.Parallel()
	src := []byte("type EventName = `on${string}`;")
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "EventName", KindType)
}

// --- Export variations. ---

func TestTS_NamedExportFunction(t *testing.T) {
	t.Parallel()
	src := []byte(`export function foo() {}
export function bar() {}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "foo", KindFunc)
	assertSymbolExists(t, symbols, "bar", KindFunc)
}

func TestTS_ExportDefaultAnonymousClass(t *testing.T) {
	t.Parallel()
	// Anonymous default export — no name to extract.
	src := []byte(`export default class {
  run() {}
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	// We can't extract a name from anonymous export. Methods might still be found.
	// This tests that we don't crash.
	_ = symbols
}

// --- Nested and complex structures. ---

func TestTS_NestedFunctions(t *testing.T) {
	t.Parallel()
	src := []byte(`function outer() {
  function inner() {
    return 42;
  }
  return inner();
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "outer", KindFunc)
	assertSymbolExists(t, symbols, "inner", KindFunc)
}

func TestTS_FunctionInsideIfBlock(t *testing.T) {
	t.Parallel()
	src := []byte(`if (process.env.NODE_ENV === "development") {
  function debugLog(msg: string) {
    console.log("[DEBUG]", msg);
  }
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "debugLog", KindFunc)
}

func TestTS_MultipleClassesInOneFile(t *testing.T) {
	t.Parallel()
	src := []byte(`class Foo {
  doFoo() {}
}

class Bar {
  doBar() {}
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	assertSymbolExists(t, symbols, "Foo", KindClass)
	assertSymbolExists(t, symbols, "doFoo", KindMethod)
	assertSymbolExists(t, symbols, "Bar", KindClass)
	assertSymbolExists(t, symbols, "doBar", KindMethod)
}

func TestTS_MethodReceiverIsCorrectClass(t *testing.T) {
	t.Parallel()
	src := []byte(`class Alpha {
  alphaMethod() {}
}

class Beta {
  betaMethod() {}
}`)
	symbols := NewTSScanner().Scan("test.ts", src)
	for _, s := range symbols {
		if s.Name == "alphaMethod" && s.Receiver != "Alpha" {
			t.Errorf("alphaMethod receiver = %q, want Alpha", s.Receiver)
		}
		if s.Name == "betaMethod" && s.Receiver != "Beta" {
			t.Errorf("betaMethod receiver = %q, want Beta", s.Receiver)
		}
	}
}

// --- JavaScript-specific. ---

func TestTS_JSFile(t *testing.T) {
	t.Parallel()
	src := []byte(`function hello() {
  return "world";
}

class App {
  start() {}
}`)
	symbols := NewTSScanner().Scan("app.js", src)
	assertSymbolExists(t, symbols, "hello", KindFunc)
	assertSymbolExists(t, symbols, "App", KindClass)
	assertSymbolExists(t, symbols, "start", KindMethod)
}

func TestTS_JSXFile(t *testing.T) {
	t.Parallel()
	src := []byte(`function Component(props) {
  return <div>{props.children}</div>;
}`)
	// JSX uses JS parser.
	symbols := NewTSScanner().Scan("Component.jsx", src)
	assertSymbolExists(t, symbols, "Component", KindFunc)
}

func TestTS_MJSFile(t *testing.T) {
	t.Parallel()
	src := []byte(`export function helper() { return 1; }`)
	symbols := NewTSScanner().Scan("util.mjs", src)
	assertSymbolExists(t, symbols, "helper", KindFunc)
}

func TestTS_CJSFile(t *testing.T) {
	t.Parallel()
	src := []byte(`function legacyHelper() { return 1; }`)
	symbols := NewTSScanner().Scan("util.cjs", src)
	assertSymbolExists(t, symbols, "legacyHelper", KindFunc)
}

// --- Edge cases and error tolerance. ---

func TestTS_EmptyFile(t *testing.T) {
	t.Parallel()
	symbols := NewTSScanner().Scan("empty.ts", []byte(""))
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(symbols))
	}
}

func TestTS_CommentOnlyFile(t *testing.T) {
	t.Parallel()
	symbols := NewTSScanner().Scan("comments.ts", []byte("// just a comment\n/* block */\n"))
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(symbols))
	}
}

func TestTS_BrokenSyntax(t *testing.T) {
	t.Parallel()
	// Incomplete code — tree-sitter should handle gracefully.
	src := []byte(`function incomplete(x: string) {
  if (x) {
    // missing closing braces`)
	symbols := NewTSScanner().Scan("broken.ts", src)
	// Should not panic. May or may not find the function.
	_ = symbols
}

func TestTS_NilInput(t *testing.T) {
	t.Parallel()
	symbols := NewTSScanner().Scan("nil.ts", nil)
	if symbols != nil && len(symbols) != 0 {
		t.Errorf("expected nil or empty, got %d", len(symbols))
	}
}

func TestTS_VeryLargeFile(t *testing.T) {
	t.Parallel()
	// Generate a file with 100 functions.
	var src []byte
	for i := 0; i < 100; i++ {
		src = append(src, []byte("function func"+string(rune('A'+i%26))+string(rune('0'+i/26))+"() { return "+string(rune('0'+i%10))+"; }\n")...)
	}
	symbols := NewTSScanner().Scan("large.ts", src)
	if len(symbols) < 26 { // at least 26 unique functions (a-z).
		t.Errorf("expected at least 26 symbols from large file, got %d", len(symbols))
	}
}

func TestTS_UnicodeIdentifiers(t *testing.T) {
	t.Parallel()
	src := []byte(`function grüße() { return "hello"; }`)
	symbols := NewTSScanner().Scan("unicode.ts", src)
	// Tree-sitter handles Unicode — should find the function.
	assertSymbolExists(t, symbols, "grüße", KindFunc)
}

// --- Const declarations that are NOT arrow functions. ---

func TestTS_ConstValueNotArrow(t *testing.T) {
	t.Parallel()
	src := []byte(`const MAX_RETRIES = 3;
const DEFAULT_PORT = 8080;
const handler = (req: Request) => { return new Response("ok"); };`)
	symbols := NewTSScanner().Scan("test.ts", src)
	// Only handler should be a symbol (arrow function).
	// MAX_RETRIES and DEFAULT_PORT are constants, not functions.
	assertSymbolExists(t, symbols, "handler", KindFunc)
	for _, s := range symbols {
		if s.Name == "MAX_RETRIES" || s.Name == "DEFAULT_PORT" {
			t.Errorf("constant %q should not be extracted as a symbol", s.Name)
		}
	}
}

// --- Test helpers. ---

func assertSymbol(t *testing.T, symbols []Symbol, name string, kind SymbolKind, startLine, endLine int) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == name {
			if s.Kind != kind {
				t.Errorf("%s: kind = %q, want %q", name, s.Kind, kind)
			}
			if s.StartLine != startLine {
				t.Errorf("%s: StartLine = %d, want %d", name, s.StartLine, startLine)
			}
			if s.EndLine != endLine {
				t.Errorf("%s: EndLine = %d, want %d", name, s.EndLine, endLine)
			}
			return
		}
	}
	t.Errorf("symbol %q not found. Got: %v", name, symbolNames(symbols))
}

func assertSymbolExists(t *testing.T, symbols []Symbol, name string, kind SymbolKind) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == name {
			if s.Kind != kind {
				t.Errorf("%s: kind = %q, want %q", name, s.Kind, kind)
			}
			return
		}
	}
	t.Errorf("symbol %q (%s) not found. Got: %v", name, kind, symbolNames(symbols))
}

func assertConstructorSkipped(t *testing.T, symbols []Symbol) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == "constructor" {
			t.Error("constructor should be skipped")
		}
	}
}

func symbolNames(symbols []Symbol) []string {
	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Name + "(" + string(s.Kind) + ")"
	}
	return names
}

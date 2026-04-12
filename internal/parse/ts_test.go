package parse

import "testing"

func TestTSScanner_Declarations(t *testing.T) {
	src := []byte(`import { Request } from 'express';

export function handleRequest(req: Request): Response {
  const data = req.body;
  return new Response(data);
}

export class UserService {
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
}

export interface Config {
  host: string;
  port: number;
}

export type UserID = string;

const processItems = (items: Item[]) => {
  return items.map(i => i.value);
}

export async function fetchData(url: string): Promise<Data> {
  const response = await fetch(url);
  return response.json();
}
`)

	scanner := TSScanner{}
	symbols := scanner.Scan("app.ts", src)

	// We expect: handleRequest, UserService, findById, delete,
	// Config, UserID, processItems, fetchData
	expected := []struct {
		name     string
		kind     SymbolKind
		receiver string
	}{
		{"handleRequest", KindFunc, ""},
		{"UserService", KindClass, ""},
		{"findById", KindMethod, "UserService"},
		{"delete", KindMethod, "UserService"},
		{"Config", KindInterface, ""},
		{"UserID", KindType, ""},
		{"processItems", KindFunc, ""},
		{"fetchData", KindFunc, ""},
	}

	if len(symbols) != len(expected) {
		t.Fatalf("expected %d symbols, got %d:\n", len(expected), len(symbols))
		for _, s := range symbols {
			t.Logf("  %s (%s) at %d-%d", s.Name, s.Kind, s.StartLine, s.EndLine)
		}
	}

	for i, tt := range expected {
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
		if sym.StartLine == 0 {
			t.Errorf("symbol[%d] %s: StartLine not set", i, tt.name)
		}
	}
}

func TestTSScanner_EndLines(t *testing.T) {
	src := []byte(`function simple() {
  return 1;
}

function multi() {
  if (true) {
    return 2;
  }
  return 3;
}
`)

	scanner := TSScanner{}
	symbols := scanner.Scan("end.ts", src)

	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}

	simple := symbols[0]
	if simple.EndLine != 3 {
		t.Errorf("simple: EndLine = %d, want 3", simple.EndLine)
	}

	multi := symbols[1]
	if multi.EndLine != 10 {
		t.Errorf("multi: EndLine = %d, want 10", multi.EndLine)
	}
}

func TestTSScanner_BlockComments(t *testing.T) {
	src := []byte(`/*
function notReal() {
  return false;
}
*/

function real() {
  return true;
}
`)

	scanner := TSScanner{}
	symbols := scanner.Scan("comments.ts", src)

	if len(symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(symbols))
	}
	if symbols[0].Name != "real" {
		t.Errorf("expected 'real', got %q", symbols[0].Name)
	}
}

func TestTSScanner_DefaultExports(t *testing.T) {
	src := []byte(`export default function createApp() {
  return {};
}

export default class App {
  run() {
    console.log("running");
  }
}
`)

	scanner := TSScanner{}
	symbols := scanner.Scan("default.ts", src)

	if len(symbols) < 2 {
		t.Fatalf("expected at least 2 symbols, got %d", len(symbols))
	}
	if symbols[0].Name != "createApp" {
		t.Errorf("expected 'createApp', got %q", symbols[0].Name)
	}
	if symbols[1].Name != "App" {
		t.Errorf("expected 'App', got %q", symbols[1].Name)
	}
}

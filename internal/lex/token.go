// Package lex provides a lightweight source code tokenizer for extracting
// symbol references from diff text. Based on Thorsten Ball's lexer design
// from "Writing an Interpreter in Go."
//
// The lexer tokenizes individual source lines (diff prefixes already stripped)
// and identifies function calls, type references, and declarations by matching
// token patterns. It is language-agnostic for v1, unioning Go and TypeScript
// keywords into one set (they don't conflict).
//
// Unicode identifiers are a known v1 limitation — the lexer uses byte-level
// scanning. Go identifiers require ASCII uppercase for exports, and Unicode
// identifiers are extremely rare in TypeScript.
package lex

// TokenType identifies the kind of a lexer token.
type TokenType int

const (
	TokenIdent   TokenType = iota // identifier: function name, type name, variable
	TokenLParen                   // (
	TokenRParen                   // )
	TokenColon                    // :
	TokenDot                      // .
	TokenLBrace                   // {
	TokenString                   // entire string literal (contents skipped)
	TokenComment                  // entire comment (contents skipped)
	TokenKeyword                  // func, type, class, function, interface, etc.
	TokenOther                    // everything else (operators, numbers, etc.)
	TokenEOL                      // end of input
)

// Token is a single lexer token with its type and literal text.
type Token struct {
	Type    TokenType
	Literal string
}

// keywords is the union of Go and TypeScript keywords relevant to
// declaration and reference detection. Go keywords (func, type, var, const)
// don't appear in TS; TS keywords (function, class, interface, export, async,
// new) are valid but rare in Go. Unioning is safe for v1.
var keywords = map[string]bool{
	// Go
	"func": true, "type": true, "var": true, "const": true,
	"package": true, "import": true, "return": true, "if": true,
	"else": true, "for": true, "range": true, "switch": true,
	"case": true, "default": true, "break": true, "continue": true,
	"go": true, "defer": true, "select": true, "chan": true,
	"map": true, "struct": true, "interface": true,
	// TypeScript / JavaScript
	"function": true, "class": true, "export": true, "async": true,
	"await": true, "new": true, "let": true, "this": true,
	"extends": true, "implements": true, "abstract": true,
	"public": true, "private": true, "protected": true, "static": true,
	"readonly": true, "declare": true, "enum": true, "namespace": true,
	"module": true, "yield": true, "throw": true, "try": true,
	"catch": true, "finally": true, "delete": true, "typeof": true,
	"instanceof": true, "void": true, "super": true, "with": true,
	"as": true, "from": true, "of": true, "in": true, "is": true,
	// Shared / common
	"nil": true, "null": true, "undefined": true, "true": true,
	"false": true,
}

// IsKeyword returns true if the identifier is a known keyword.
func IsKeyword(s string) bool {
	return keywords[s]
}

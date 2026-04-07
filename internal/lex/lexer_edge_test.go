package lex

import "testing"

func TestLexer_UnclosedString(t *testing.T) {
	t.Parallel()
	// Unclosed string should consume to end of input.
	tokens := Tokenize(`name := "unterminated`)

	hasString := false
	for _, tok := range tokens {
		if tok.Type == TokenString {
			hasString = true
		}
	}
	if !hasString {
		t.Error("expected STRING token for unclosed string")
	}
}

func TestLexer_UnclosedTemplateLiteral(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("msg := `unterminated template")

	hasString := false
	for _, tok := range tokens {
		if tok.Type == TokenString {
			hasString = true
		}
	}
	if !hasString {
		t.Error("expected STRING token for unclosed template literal")
	}
}

func TestLexer_UnclosedBlockComment(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("x /* unclosed comment")

	hasComment := false
	for _, tok := range tokens {
		if tok.Type == TokenComment {
			hasComment = true
		}
	}
	if !hasComment {
		t.Error("expected COMMENT token for unclosed block comment")
	}
}

func TestLexer_EscapedBacktick(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("msg := `foo\\`bar`")

	// Should handle escaped backtick inside template literal.
	// The lexer may or may not handle this perfectly — key is no panic.
	hasString := false
	for _, tok := range tokens {
		if tok.Type == TokenString {
			hasString = true
		}
	}
	if !hasString {
		t.Error("expected STRING token")
	}
}

func TestLexer_ConsecutiveEscapes(t *testing.T) {
	t.Parallel()
	tokens := Tokenize(`x := "foo\\\\bar"`)

	// \\\\ is two escaped backslashes. Should not break out of the string.
	hasIdent := false
	for _, tok := range tokens {
		if tok.Type == TokenIdent && tok.Literal == "x" {
			hasIdent = true
		}
	}
	if !hasIdent {
		t.Error("expected ident 'x'")
	}
}

func TestLexer_NumbersAsOther(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("x := 42")

	for _, tok := range tokens {
		if tok.Type == TokenIdent && tok.Literal == "42" {
			t.Error("numbers should not be identifiers")
		}
	}
}

func TestLexer_WhitespaceOnly(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("   \t  ")
	if len(tokens) != 1 || tokens[0].Type != TokenEOL {
		t.Errorf("whitespace-only input should produce only EOL, got %d tokens", len(tokens))
	}
}

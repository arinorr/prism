package lex

import "testing"

func TestLexer_BasicTokens(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("result := ProcessBatch(data)")

	// Expected: IDENT(result) COLON OTHER(=) IDENT(ProcessBatch) LPAREN IDENT(data) RPAREN EOL
	// Note: := is two tokens — COLON then OTHER(=)
	wantTypes := []TokenType{TokenIdent, TokenColon, TokenOther, TokenIdent, TokenLParen, TokenIdent, TokenRParen, TokenEOL}
	if len(tokens) != len(wantTypes) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(wantTypes))
	}
	for i, wt := range wantTypes {
		if tokens[i].Type != wt {
			t.Errorf("token[%d] type = %d, want %d (literal: %q)", i, tokens[i].Type, wt, tokens[i].Literal)
		}
	}
}

func TestLexer_Keywords(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("func HandleRequest() {")

	if tokens[0].Type != TokenKeyword || tokens[0].Literal != "func" {
		t.Errorf("expected keyword 'func', got type=%d literal=%q", tokens[0].Type, tokens[0].Literal)
	}
	if tokens[1].Type != TokenIdent || tokens[1].Literal != "HandleRequest" {
		t.Errorf("expected ident 'HandleRequest', got type=%d literal=%q", tokens[1].Type, tokens[1].Literal)
	}
}

func TestLexer_StringSkipped(t *testing.T) {
	t.Parallel()
	tokens := Tokenize(`name := "ProcessBatch"`)

	for _, tok := range tokens {
		if tok.Type == TokenIdent && tok.Literal == "ProcessBatch" {
			t.Error("ProcessBatch inside string should not be an ident token")
		}
	}
	// Should have a STRING token containing the quoted literal.
	hasString := false
	for _, tok := range tokens {
		if tok.Type == TokenString {
			hasString = true
		}
	}
	if !hasString {
		t.Error("expected a STRING token for the quoted literal")
	}
}

func TestLexer_SingleQuoteString(t *testing.T) {
	t.Parallel()
	tokens := Tokenize(`const x = 'hello'`)

	hasString := false
	for _, tok := range tokens {
		if tok.Type == TokenString {
			hasString = true
		}
	}
	if !hasString {
		t.Error("expected STRING token for single-quoted literal")
	}
}

func TestLexer_TemplateLiteral(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("const msg = `hello ${name}`")

	hasString := false
	for _, tok := range tokens {
		if tok.Type == TokenString {
			hasString = true
		}
	}
	if !hasString {
		t.Error("expected STRING token for template literal")
	}
}

func TestLexer_LineComment(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("// calls ProcessBatch")

	if tokens[0].Type != TokenComment {
		t.Errorf("expected COMMENT, got type=%d", tokens[0].Type)
	}
	// Should be only COMMENT + EOL.
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens (comment + EOL), got %d", len(tokens))
	}
}

func TestLexer_BlockComment(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("x /* skip this */ = y")

	hasComment := false
	for _, tok := range tokens {
		if tok.Type == TokenComment {
			hasComment = true
		}
	}
	if !hasComment {
		t.Error("expected COMMENT token for block comment")
	}
	// 'y' should still be parsed after the comment.
	hasY := false
	for _, tok := range tokens {
		if tok.Type == TokenIdent && tok.Literal == "y" {
			hasY = true
		}
	}
	if !hasY {
		t.Error("expected ident 'y' after block comment")
	}
}

func TestLexer_DotAccess(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("cfg.Port")

	wantTypes := []TokenType{TokenIdent, TokenDot, TokenIdent, TokenEOL}
	if len(tokens) != len(wantTypes) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(wantTypes))
	}
	for i, wt := range wantTypes {
		if tokens[i].Type != wt {
			t.Errorf("token[%d] type = %d, want %d", i, tokens[i].Type, wt)
		}
	}
}

func TestLexer_EscapedString(t *testing.T) {
	t.Parallel()
	tokens := Tokenize(`msg := "say \"hello\""`)

	// Should not break on escaped quotes.
	hasIdent := false
	for _, tok := range tokens {
		if tok.Type == TokenIdent && tok.Literal == "msg" {
			hasIdent = true
		}
	}
	if !hasIdent {
		t.Error("expected ident 'msg'")
	}
}

func TestLexer_EmptyInput(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("")
	if len(tokens) != 1 || tokens[0].Type != TokenEOL {
		t.Errorf("empty input should produce only EOL, got %d tokens", len(tokens))
	}
}

func TestLexer_TypeAnnotation(t *testing.T) {
	t.Parallel()
	tokens := Tokenize("const x: SomeType = value")

	// Should have COLON followed by IDENT(SomeType).
	for i, tok := range tokens {
		if tok.Type == TokenColon {
			if i+1 < len(tokens) && tokens[i+1].Type == TokenIdent && tokens[i+1].Literal == "SomeType" {
				return // found it
			}
		}
	}
	t.Error("expected COLON followed by IDENT 'SomeType'")
}

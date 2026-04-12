package lex

// Lexer tokenizes a single line of source code. Based on Thorsten Ball's
// lexer design: read character, switch on it, emit token.
//
// The caller is responsible for stripping diff prefixes (+/-/space) before
// passing the line to the lexer.
type Lexer struct {
	input   string
	pos     int  // current position (points to ch)
	readPos int  // next read position
	ch      byte // current character
}

// New creates a Lexer for the given input line.
func New(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

// NextToken returns the next token from the input.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if l.ch == 0 {
		return Token{Type: TokenEOL, Literal: ""}
	}

	switch {
	case l.ch == '(':
		tok := Token{Type: TokenLParen, Literal: "("}
		l.readChar()
		return tok
	case l.ch == ')':
		tok := Token{Type: TokenRParen, Literal: ")"}
		l.readChar()
		return tok
	case l.ch == ':':
		tok := Token{Type: TokenColon, Literal: ":"}
		l.readChar()
		return tok
	case l.ch == '.':
		tok := Token{Type: TokenDot, Literal: "."}
		l.readChar()
		return tok
	case l.ch == '{':
		tok := Token{Type: TokenLBrace, Literal: "{"}
		l.readChar()
		return tok

	// String literals — skip contents entirely.
	case l.ch == '"':
		return l.readStringLiteral('"')
	case l.ch == '\'':
		return l.readStringLiteral('\'')
	case l.ch == '`':
		return l.readTemplateLiteral()

	// Comments.
	case l.ch == '/' && l.peekChar() == '/':
		lit := l.input[l.pos:]
		l.pos = len(l.input)
		l.readPos = len(l.input)
		l.ch = 0
		return Token{Type: TokenComment, Literal: lit}
	case l.ch == '/' && l.peekChar() == '*':
		return l.readBlockComment()

	// Identifiers and keywords.
	case isIdentStart(l.ch):
		lit := l.readIdentifier()
		if keywords[lit] {
			return Token{Type: TokenKeyword, Literal: lit}
		}
		return Token{Type: TokenIdent, Literal: lit}

	default:
		tok := Token{Type: TokenOther, Literal: string(l.ch)}
		l.readChar()
		return tok
	}
}

func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
}

func (l *Lexer) peekChar() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) readIdentifier() string {
	start := l.pos
	for isIdentPart(l.ch) {
		l.readChar()
	}
	return l.input[start:l.pos]
}

func (l *Lexer) readStringLiteral(quote byte) Token {
	start := l.pos
	l.readChar() // skip opening quote
	for l.ch != 0 && l.ch != quote {
		if l.ch == '\\' {
			l.readChar() // skip escape
		}
		l.readChar()
	}
	if l.ch == quote {
		l.readChar() // skip closing quote
	}
	return Token{Type: TokenString, Literal: l.input[start:l.pos]}
}

func (l *Lexer) readTemplateLiteral() Token {
	// Template literals can span lines. For a single-line lexer, consume
	// everything from ` to the next ` or end of input.
	start := l.pos
	l.readChar() // skip opening `
	for l.ch != 0 && l.ch != '`' {
		if l.ch == '\\' {
			l.readChar()
		}
		l.readChar()
	}
	if l.ch == '`' {
		l.readChar()
	}
	return Token{Type: TokenString, Literal: l.input[start:l.pos]}
}

func (l *Lexer) readBlockComment() Token {
	start := l.pos
	l.readChar() // skip /
	l.readChar() // skip *
	for l.ch != 0 {
		if l.ch == '*' && l.peekChar() == '/' {
			l.readChar() // skip *
			l.readChar() // skip /
			break
		}
		l.readChar()
	}
	return Token{Type: TokenComment, Literal: l.input[start:l.pos]}
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

// Tokenize scans a full line and returns all tokens.
func Tokenize(line string) []Token {
	l := New(line)
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOL {
			break
		}
	}
	return tokens
}

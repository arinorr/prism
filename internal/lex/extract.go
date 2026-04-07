package lex

import "strings"

// RefKind classifies how a symbol is referenced.
type RefKind string

const (
	RefCall        RefKind = "call"        // Name( or pkg.Name(
	RefType        RefKind = "type"        // : Name
	RefDeclaration RefKind = "declaration" // func Name, type Name, class Name
)

// SymbolReference represents a symbol found in source text.
type SymbolReference struct {
	Name     string
	Kind     RefKind
	InChange bool // true if from a +/- line (not context)
}

// declarationKeywords are keywords that introduce a named declaration.
var declarationKeywords = map[string]bool{
	"func": true, "type": true,
	"function": true, "class": true, "interface": true,
}

// ExtractCandidates scans diff text for symbol references and declarations.
// Strips diff prefixes (+/-/space), skips hunk headers and diff metadata,
// tokenizes each clean source line, and matches token patterns.
//
// Returns candidates WITHOUT index filtering — the caller filters separately.
func ExtractCandidates(diffText string) []SymbolReference {
	var refs []SymbolReference
	seen := make(map[string]bool)

	for _, line := range strings.Split(diffText, "\n") {
		if shouldSkipDiffLine(line) {
			continue
		}

		inChange := len(line) > 0 && (line[0] == '+' || line[0] == '-')
		clean := stripDiffPrefix(line)
		if clean == "" {
			continue
		}

		tokens := Tokenize(clean)
		lineRefs := matchPatterns(tokens, inChange)

		for _, ref := range lineRefs {
			key := ref.Name + ":" + string(ref.Kind)
			if seen[key] {
				continue
			}
			seen[key] = true
			refs = append(refs, ref)
		}
	}

	return refs
}

// shouldSkipDiffLine returns true for lines that are diff metadata,
// not source code.
func shouldSkipDiffLine(line string) bool {
	if line == "" {
		return true
	}
	// Hunk headers.
	if strings.HasPrefix(line, "@@") {
		return true
	}
	// File headers.
	if strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
		return true
	}
	// "No newline at end of file" marker.
	if strings.HasPrefix(line, `\`) {
		return true
	}
	// Index and mode lines.
	if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "old mode") || strings.HasPrefix(line, "new mode") {
		return true
	}
	return false
}

// stripDiffPrefix removes the +/-/space diff prefix from a line.
func stripDiffPrefix(line string) string {
	if len(line) == 0 {
		return ""
	}
	if line[0] == '+' || line[0] == '-' || line[0] == ' ' {
		return line[1:]
	}
	return line
}

// matchPatterns scans a token sequence for symbol references and declarations.
func matchPatterns(tokens []Token, inChange bool) []SymbolReference {
	var refs []SymbolReference

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]

		// Pattern: KEYWORD(func/type/class/...) IDENT → declaration
		if tok.Type == TokenKeyword && declarationKeywords[tok.Literal] {
			if next := lookAhead(tokens, i+1); next.Type == TokenIdent {
				refs = append(refs, SymbolReference{
					Name:     next.Literal,
					Kind:     RefDeclaration,
					InChange: inChange,
				})
				i++ // skip the ident
				continue
			}
		}

		// Pattern: IDENT LPAREN → function call
		if tok.Type == TokenIdent {
			next := lookAhead(tokens, i+1)
			if next.Type == TokenLParen {
				refs = append(refs, SymbolReference{
					Name:     tok.Literal,
					Kind:     RefCall,
					InChange: inChange,
				})
				continue
			}

			// Pattern: IDENT DOT IDENT LPAREN → qualified call (use the method name)
			if next.Type == TokenDot {
				method := lookAhead(tokens, i+2)
				paren := lookAhead(tokens, i+3)
				if method.Type == TokenIdent && paren.Type == TokenLParen {
					refs = append(refs, SymbolReference{
						Name:     method.Literal,
						Kind:     RefCall,
						InChange: inChange,
					})
					i += 2 // skip dot + method
					continue
				}
			}
		}

		// Pattern: COLON IDENT → type annotation (TypeScript)
		if tok.Type == TokenColon {
			next := lookAhead(tokens, i+1)
			if next.Type == TokenIdent {
				refs = append(refs, SymbolReference{
					Name:     next.Literal,
					Kind:     RefType,
					InChange: inChange,
				})
				i++ // skip the ident
				continue
			}
		}

		// Pattern: KEYWORD(new) IDENT → constructor call
		if tok.Type == TokenKeyword && tok.Literal == "new" {
			next := lookAhead(tokens, i+1)
			if next.Type == TokenIdent {
				refs = append(refs, SymbolReference{
					Name:     next.Literal,
					Kind:     RefCall,
					InChange: inChange,
				})
				i++
				continue
			}
		}
	}

	return refs
}

func lookAhead(tokens []Token, i int) Token {
	if i >= len(tokens) {
		return Token{Type: TokenEOL}
	}
	return tokens[i]
}

package index

import (
	"regexp"
	"strings"
)

// TSScanner extracts symbol declarations from TypeScript and JavaScript files
// using line-by-line regex matching with brace counting for end-line detection.
// It is intentionally approximate: false negatives are acceptable (missing some
// symbols), false positives are not (wrong line ranges).
type TSScanner struct{}

var (
	// Top-level declarations.
	reFuncDecl  = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+(\w+)`)
	reClassDecl = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+(\w+)`)
	reIfaceDecl = regexp.MustCompile(`^(?:export\s+)?interface\s+(\w+)`)
	reTypeDecl  = regexp.MustCompile(`^(?:export\s+)?type\s+(\w+)\s*[=<]`)
	reArrowDecl = regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:\([^)]*\)|[^=])*\s*=>`)
	// Method declarations inside a class body.
	reMethodDecl = regexp.MustCompile(`^\s+(?:public\s+|private\s+|protected\s+)?(?:static\s+)?(?:async\s+)?(\w+)\s*\(`)
)

func (TSScanner) Scan(filename string, src []byte) []Symbol {
	lines := strings.Split(string(src), "\n")
	var symbols []Symbol

	inBlockComment := false
	var classStack []int // indices into symbols for open class declarations

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Track block comments to avoid matching braces inside them.
		if inBlockComment {
			if idx := strings.Index(trimmed, "*/"); idx >= 0 {
				inBlockComment = false
				trimmed = trimmed[idx+2:]
			} else {
				continue
			}
		}
		if idx := strings.Index(trimmed, "/*"); idx >= 0 {
			if !strings.Contains(trimmed[idx:], "*/") {
				inBlockComment = true
			}
		}

		// Skip single-line comments.
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Try matching declarations (order matters: class before method).
		if m := reFuncDecl.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{
				Name:      m[1],
				File:      filename,
				StartLine: lineNum,
				EndLine:   lineNum, // updated by brace counting
				Kind:      KindFunc,
			})
		} else if m := reClassDecl.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{
				Name:      m[1],
				File:      filename,
				StartLine: lineNum,
				EndLine:   lineNum,
				Kind:      KindClass,
			})
			classStack = append(classStack, len(symbols)-1)
		} else if m := reIfaceDecl.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{
				Name:      m[1],
				File:      filename,
				StartLine: lineNum,
				EndLine:   lineNum,
				Kind:      KindInterface,
			})
		} else if m := reTypeDecl.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{
				Name:      m[1],
				File:      filename,
				StartLine: lineNum,
				EndLine:   lineNum,
				Kind:      KindType,
			})
		} else if m := reArrowDecl.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{
				Name:      m[1],
				File:      filename,
				StartLine: lineNum,
				EndLine:   lineNum,
				Kind:      KindFunc,
			})
		} else if len(classStack) > 0 {
			// Inside a class: check for method declarations.
			if m := reMethodDecl.FindStringSubmatch(line); m != nil {
				name := m[1]
				// Skip keywords that look like methods.
				if name != "if" && name != "for" && name != "while" && name != "switch" && name != "catch" && name != "constructor" {
					classIdx := classStack[len(classStack)-1]
					symbols = append(symbols, Symbol{
						Name:      m[1],
						File:      filename,
						StartLine: lineNum,
						EndLine:   lineNum,
						Kind:      KindMethod,
						Receiver:  symbols[classIdx].Name,
					})
				}
			}
		}
	}

	// Second pass: use brace counting to find EndLine for each symbol.
	resolveEndLines(lines, symbols)

	return symbols
}

// resolveEndLines uses brace counting to determine the EndLine for each symbol.
// For each symbol, it starts scanning from its StartLine and tracks brace depth.
// EndLine is set when the depth returns to the pre-declaration level.
func resolveEndLines(lines []string, symbols []Symbol) {
	for i := range symbols {
		sym := &symbols[i]
		if sym.Kind == KindType {
			// Type aliases are typically single-line; scan for semicolon or end of line.
			sym.EndLine = findTypeEnd(lines, sym.StartLine-1)
			continue
		}

		depth := 0
		started := false
		inBlockComment := false

		for j := sym.StartLine - 1; j < len(lines); j++ {
			line := lines[j]

			for k := 0; k < len(line); k++ {
				if inBlockComment {
					if k+1 < len(line) && line[k] == '*' && line[k+1] == '/' {
						inBlockComment = false
						k++ // skip '/'
					}
					continue
				}

				ch := line[k]
				switch {
				case k+1 < len(line) && ch == '/' && line[k+1] == '/':
					// Rest of line is comment.
					k = len(line)
				case k+1 < len(line) && ch == '/' && line[k+1] == '*':
					inBlockComment = true
					k++ // skip '*'
				case ch == '\'' || ch == '"' || ch == '`':
					// Skip string literals.
					k = skipString(line, k, ch)
				case ch == '{':
					depth++
					started = true
				case ch == '}':
					depth--
					if started && depth <= 0 {
						sym.EndLine = j + 1
						goto nextSymbol
					}
				}
			}
		}
	nextSymbol:
	}
}

// findTypeEnd finds the end line for a type alias declaration.
// It scans forward from the start line looking for the end of the type
// expression (handles multi-line union/intersection types).
func findTypeEnd(lines []string, startIdx int) int {
	depth := 0
	for i := startIdx; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		for _, ch := range line {
			switch ch {
			case '{', '(':
				depth++
			case '}', ')':
				depth--
			}
		}
		// Type ends when we're back to depth 0 and the line doesn't end with | or &.
		if depth <= 0 && !strings.HasSuffix(line, "|") && !strings.HasSuffix(line, "&") {
			return i + 1
		}
	}
	return startIdx + 1
}

// skipString advances past a string literal starting at position start.
func skipString(line string, start int, quote byte) int {
	if quote == '`' {
		// Template literals can span lines; just skip to end of current line.
		return len(line) - 1
	}
	for i := start + 1; i < len(line); i++ {
		if line[i] == '\\' {
			i++ // skip escaped character
			continue
		}
		if line[i] == quote {
			return i
		}
	}
	return len(line) - 1
}

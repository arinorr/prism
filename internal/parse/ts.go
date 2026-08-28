package parse

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// TSScanner extracts symbol declarations from TypeScript and JavaScript files
// using tree-sitter for proper AST-based parsing. This replaces the previous
// regex + brace counting approach which missed arrow functions in objects,
// nested declarations, destructured exports, and more.
type TSScanner struct {
	tsParser *sitter.Parser
	jsParser *sitter.Parser
}

// NewTSScanner creates a TSScanner with tree-sitter parsers for TypeScript and JavaScript.
func NewTSScanner() *TSScanner {
	ts := sitter.NewParser()
	ts.SetLanguage(typescript.GetLanguage())
	js := sitter.NewParser()
	js.SetLanguage(javascript.GetLanguage())
	return &TSScanner{tsParser: ts, jsParser: js}
}

func (s *TSScanner) Scan(filename string, src []byte) []Symbol {
	parser := s.tsParser
	if isJS(filename) {
		parser = s.jsParser
	}

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil || tree == nil {
		return nil
	}
	root := tree.RootNode()
	if root == nil {
		return nil
	}

	var symbols []Symbol
	walkTSNode(root, src, filename, &symbols, "")
	return symbols
}

// isJS returns true if the filename has a JavaScript extension.
func isJS(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".js", ".jsx", ".mjs", ".cjs":
		return true
	}
	return false
}

// walkTSNode recursively walks the tree-sitter AST and extracts symbol declarations.
func walkTSNode(node *sitter.Node, src []byte, file string, symbols *[]Symbol, currentClass string) {
	switch node.Type() {
	case "function_declaration", "generator_function_declaration":
		if name := nodeFieldContent(node, "name", src); name != "" {
			*symbols = append(*symbols, Symbol{
				Name:      name,
				File:      file,
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Kind:      KindFunc,
			})
		}

	case "class_declaration", "abstract_class_declaration":
		name := nodeFieldContent(node, "name", src)
		if name != "" {
			*symbols = append(*symbols, Symbol{
				Name:      name,
				File:      file,
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Kind:      KindClass,
			})
		}
		// Walk children with class context for method detection.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			walkTSNode(node.NamedChild(i), src, file, symbols, name)
		}
		return // don't walk children again below

	case "interface_declaration":
		if name := nodeFieldContent(node, "name", src); name != "" {
			*symbols = append(*symbols, Symbol{
				Name:      name,
				File:      file,
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Kind:      KindInterface,
			})
		}

	case "type_alias_declaration":
		if name := nodeFieldContent(node, "name", src); name != "" {
			*symbols = append(*symbols, Symbol{
				Name:      name,
				File:      file,
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Kind:      KindType,
			})
		}

	case "method_definition":
		if name := nodeFieldContent(node, "name", src); name != "" {
			// Skip constructor.
			if name != "constructor" {
				*symbols = append(*symbols, Symbol{
					Name:      name,
					File:      file,
					StartLine: int(node.StartPoint().Row) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
					Kind:      KindMethod,
					Receiver:  currentClass,
				})
			}
		}

	case "lexical_declaration":
		// Handle: const/let/var name = (...) => { ... }
		// These are arrow function declarations.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			decl := node.NamedChild(i)
			if decl.Type() == "variable_declarator" {
				nameNode := decl.ChildByFieldName("name")
				valueNode := decl.ChildByFieldName("value")
				if nameNode != nil && valueNode != nil && valueNode.Type() == "arrow_function" {
					*symbols = append(*symbols, Symbol{
						Name:      nameNode.Content(src),
						File:      file,
						StartLine: int(node.StartPoint().Row) + 1,
						EndLine:   int(node.EndPoint().Row) + 1,
						Kind:      KindFunc,
					})
				}
			}
		}

	case "export_statement":
		// Walk into export to find the actual declaration.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			walkTSNode(child, src, file, symbols, currentClass)
		}
		return // don't walk children again below
	}

	// Recurse into children.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkTSNode(node.NamedChild(i), src, file, symbols, currentClass)
	}
}

// nodeFieldContent returns the text content of a named field on a node,
// or empty string if the field doesn't exist.
func nodeFieldContent(node *sitter.Node, field string, src []byte) string {
	child := node.ChildByFieldName(field)
	if child == nil {
		return ""
	}
	return child.Content(src)
}

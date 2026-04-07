package index

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// GoScanner extracts symbol declarations from Go source files
// using the standard library's go/ast package.
type GoScanner struct{}

func (GoScanner) Scan(filename string, src []byte) []Symbol {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.AllErrors)
	if err != nil || file == nil {
		return nil
	}

	var symbols []Symbol

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := Symbol{
				Name:      d.Name.Name,
				File:      filename,
				StartLine: fset.Position(d.Pos()).Line,
				EndLine:   fset.Position(d.End()).Line,
				Kind:      KindFunc,
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				sym.Kind = KindMethod
				sym.Receiver = receiverTypeName(d.Recv.List[0].Type)
			}
			symbols = append(symbols, sym)

		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				sym := Symbol{
					Name:      ts.Name.Name,
					File:      filename,
					StartLine: fset.Position(spec.Pos()).Line,
					EndLine:   fset.Position(spec.End()).Line,
					Kind:      KindType,
				}
				switch ts.Type.(type) {
				case *ast.InterfaceType:
					sym.Kind = KindInterface
				}
				symbols = append(symbols, sym)
			}
		}
	}

	return symbols
}

// receiverTypeName extracts the type name from a method receiver expression,
// handling both value receivers (T) and pointer receivers (*T).
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverTypeName(t.X) // recurse: *T, *T[P], *T[P, Q]
	case *ast.IndexExpr: // generic receiver T[P]
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.IndexListExpr: // generic receiver T[P, Q]
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

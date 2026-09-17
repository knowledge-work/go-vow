package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"unicode/utf8"
)

// declaredIdents parses the given Go files (syntax only, no type
// checking) and returns the set of identifiers a caller in another
// package could reference: top-level type, function, method, var,
// and const names, plus struct field and interface method names
// declared anywhere inside a top-level declaration — type bodies,
// function signatures, and var/const type annotations alike, so
// members of anonymous types are covered too. The blank identifier
// is dropped because it can never be referenced.
func declaredIdents(files []string) (map[string]struct{}, error) {
	fset := token.NewFileSet()
	idents := make(map[string]struct{})
	add := func(name string) {
		if name != "_" {
			idents[name] = struct{}{}
		}
	}
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				add(d.Name.Name)
				collectMemberNames(d.Type, add)
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						add(s.Name.Name)
						collectMemberNames(s.Type, add)
					case *ast.ValueSpec:
						for _, name := range s.Names {
							add(name.Name)
						}
						if s.Type != nil {
							collectMemberNames(s.Type, add)
						}
					}
				}
			}
		}
	}
	return idents, nil
}

// collectMemberNames walks a type expression and feeds every struct
// field name and interface method name to add. Nested composite
// types (a struct inside a struct, an interface inside a struct
// field) are included because their members are referenced by the
// same selector syntax as top-level ones. An embedded field
// contributes its implicit name — the base identifier of its type —
// because a selector like w.Embedded references that name directly.
func collectMemberNames(typeExpr ast.Expr, add func(string)) {
	ast.Inspect(typeExpr, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.StructType:
			for _, field := range t.Fields.List {
				if len(field.Names) == 0 {
					add(embeddedFieldName(field.Type))
					continue
				}
				for _, name := range field.Names {
					add(name.Name)
				}
			}
		case *ast.InterfaceType:
			for _, method := range t.Methods.List {
				for _, name := range method.Names {
					add(name.Name)
				}
			}
		}
		return true
	})
}

// embeddedFieldName resolves the implicit field name of an embedded
// struct field: the base identifier of its type with any pointer,
// qualifier, and type-argument decoration stripped. Type expressions
// that cannot appear as embedded fields resolve to the blank
// identifier, which the caller discards.
func embeddedFieldName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return embeddedFieldName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return embeddedFieldName(t.X)
	case *ast.IndexListExpr:
		return embeddedFieldName(t.X)
	}
	return "_"
}

// fileMentionsAny reports whether the file contains any of the
// identifiers as a whole word. Words are maximal runs of identifier
// bytes (ASCII letters, digits, underscore, and any non-ASCII byte,
// matching Go's identifier syntax at byte granularity), so a match
// never fires on a substring of a longer identifier. Occurrences
// inside comments or strings match too; that over-approximation is
// deliberate — the resolver's contract is to never miss a real
// reference.
func fileMentionsAny(path string, idents map[string]struct{}) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	start := -1
	for i := 0; i <= len(content); i++ {
		var c byte
		if i < len(content) {
			c = content[i]
		}
		if isIdentByte(c) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			if _, ok := idents[string(content[start:i])]; ok {
				return true, nil
			}
			start = -1
		}
	}
	return false, nil
}

// isIdentByte reports whether c can be part of a Go identifier when
// scanning at byte granularity; every byte of a multi-byte rune
// counts so multi-byte identifiers stay glued to their word.
func isIdentByte(c byte) bool {
	return c == '_' ||
		'a' <= c && c <= 'z' ||
		'A' <= c && c <= 'Z' ||
		'0' <= c && c <= '9' ||
		c >= utf8.RuneSelf
}

package analysis

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateFieldNilAssign reports a statically-nil value written into a
// struct field declared non-nil (`!`). A pointer receiver and a value
// receiver resolve to the same field Object, so one recogniser covers
// both assignment forms. Values whose nil-ness needs dataflow belong to
// validateFieldNilAssignFlow.
//
// Diagnostics anchor at the nil expression rather than at the enclosing
// statement or literal, so both shapes point at the token an author
// edits.
func validateFieldNilAssign(pass *analysis.Pass, state *passState) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				reportFieldNilAssignStmt(pass, state, node)
			case *ast.CompositeLit:
				reportFieldNilCompositeLit(pass, state, node)
			}
			return true
		})
	}
}

// reportFieldNilAssignStmt emits a diagnostic for every left-hand side
// of assign that selects a `!`-declared field and whose index-aligned
// right-hand side is statically nil.
func reportFieldNilAssignStmt(pass *analysis.Pass, state *passState, assign *ast.AssignStmt) {
	for i, lhs := range assign.Lhs {
		sel, ok := ast.Unparen(lhs).(*ast.SelectorExpr)
		if !ok {
			continue
		}
		if i >= len(assign.Rhs) {
			continue
		}
		if !argumentIsStaticallyNil(pass, assign.Rhs[i]) {
			continue
		}
		name, ok := nonNilFieldName(pass, state, pass.TypesInfo.ObjectOf(sel.Sel))
		if !ok {
			continue
		}
		reportFieldAssignedNil(pass, assign.Rhs[i], name)
	}
}

// reportFieldNilCompositeLit emits a diagnostic for every element of
// lit that supplies a `!`-declared field with a statically-nil
// value. Keyed elements resolve the field through the key
// identifier's Object; positional elements resolve it through the
// literal's struct type at the element's index. A literal whose type
// is not a struct (a slice, map, or array literal) contributes no
// field write of its own and returns early; a struct literal nested
// inside it is a separate node carrying its own field writes.
func reportFieldNilCompositeLit(pass *analysis.Pass, state *passState, lit *ast.CompositeLit) {
	st := structTypeOfCompositeLit(pass, lit)
	if st == nil {
		return
	}
	for i, elt := range lit.Elts {
		value, obj := compositeLitFieldWrite(pass, st, i, elt)
		if value == nil {
			continue
		}
		if !argumentIsStaticallyNil(pass, value) {
			continue
		}
		name, ok := nonNilFieldName(pass, state, obj)
		if !ok {
			continue
		}
		reportFieldAssignedNil(pass, value, name)
	}
}

// structTypeOfCompositeLit returns the struct type lit constructs,
// or nil when lit builds a non-struct value. A pointer element type
// inside an outer literal (`[]*T{{...}}`) reaches the inner literal
// as `*T`, so the resolver steps through one pointer layer before
// reading the underlying struct.
func structTypeOfCompositeLit(pass *analysis.Pass, lit *ast.CompositeLit) *types.Struct {
	typ := pass.TypesInfo.TypeOf(lit)
	if typ == nil {
		return nil
	}
	if ptr, ok := typ.Underlying().(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	st, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	return st
}

// compositeLitFieldWrite resolves the field an element of a struct
// composite literal writes, returning the written expression and the
// field's Object. A keyed element (`Ref: nil`) reads the field off
// the key identifier; a positional element reads it off the struct
// type at index i. Both forms return a nil expression when the
// element does not resolve to a field of st — a keyed element whose
// key is not a field identifier, or a positional index beyond the
// struct's field count.
func compositeLitFieldWrite(pass *analysis.Pass, st *types.Struct, i int, elt ast.Expr) (ast.Expr, types.Object) {
	if kv, ok := elt.(*ast.KeyValueExpr); ok {
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return nil, nil
		}
		return kv.Value, pass.TypesInfo.ObjectOf(key)
	}
	if i >= st.NumFields() {
		return nil, nil
	}
	return elt, st.Field(i)
}

// nonNilFieldName returns obj's declared name when obj is a struct
// field whose `vow:nil` decl pins it as non-nil (`!`). Fields at
// platform, fields declared `?`, and fields carrying a nest-only
// decl (no top-level nullness token) return ok=false so the pass
// stays silent on every position the declaration left
// unconstrained. The lookup runs through fieldNilForField so a
// field declared in an imported package resolves through its
// fieldNilFact under the same predicate as a same-package one.
func nonNilFieldName(pass *analysis.Pass, state *passState, obj types.Object) (string, bool) {
	if obj == nil {
		return "", false
	}
	decl := fieldNilForField(pass, state, obj)
	if decl == nil || decl.Nullness != dsl.NullnessNonNil {
		return "", false
	}
	return obj.Name(), true
}

// reportFieldAssignedNil emits the shared diagnostic for a nil write
// into a non-nil-declared field, anchored at the nil expression.
func reportFieldAssignedNil(pass *analysis.Pass, value ast.Expr, fieldName string) {
	vowReportNilSafetyf(
		pass,
		value.Pos(),
		"vow[nil-safety]: field %s assigned nil; %s declared ! on this field",
		fieldName,
		nilDeclMarker,
	)
}

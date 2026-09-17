package analysis

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateFieldNilDeref reports a dereference of a `?`-declared field
// that no nil guard covers.
//
// A guard discharges the site through the same field-location matcher
// the caller-side argument check uses, so what counts as a guard, and
// what ends its proof, is decided in one place for both surfaces.
//
// At most one site is reported per field reached through one base, so a
// field read repeatedly under one missing guard yields one diagnostic.
func validateFieldNilDeref(pass *analysis.Pass, state *passState) {
	srcFuncs := ssaSourceFunctions(pass)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Body == nil {
				continue
			}
			reportUnguardedFieldDerefs(pass, state, fn, findSSAFunction(srcFuncs, fn))
		}
	}
}

// fieldDerefKey identifies one field reached through one base
// identifier, which is the granularity a guard applies at and so the
// granularity a diagnostic is reported at.
type fieldDerefKey struct {
	base  types.Object
	field types.Object
}

// reportUnguardedFieldDerefs walks fn's body for dereferences of
// `?`-declared fields and reports the first unguarded one per field and
// base. ssaFn may be nil when the SSA form is unavailable, in which
// case no guard can be recognised and every deref site reports.
func reportUnguardedFieldDerefs(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function) {
	reported := map[fieldDerefKey]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		site, sel, ok := fieldDerefSite(pass, n)
		if !ok {
			return true
		}
		key, ok := fieldDerefKeyFor(pass, sel)
		if !ok || reported[key] {
			return true
		}
		decl := fieldNilForField(pass, state, key.field)
		if decl == nil || decl.Nullness != dsl.NullnessNillable {
			return true
		}
		if fieldDerefNarrowed(pass, ssaFn, site, sel) {
			return true
		}
		reported[key] = true
		vowReportNilSafetyf(
			pass,
			site.Pos(),
			"vow[nil-safety]: field %s is dereferenced without a nil guard; %s declared ? on this field",
			key.field.Name(),
			nilDeclMarker,
		)
		return true
	})
}

// fieldDerefSite reports whether n dereferences the value a field
// selector holds, returning the dereferencing expression together with
// the field selector it reads through. Two shapes qualify:
//
//   - `*h.Maybe` — the explicit dereference operator.
//   - `h.Maybe.X` where reaching X steps through the pointer the field
//     holds, as selectionDerefsPointer decides.
func fieldDerefSite(pass *analysis.Pass, n ast.Node) (ast.Expr, *ast.SelectorExpr, bool) {
	switch node := n.(type) {
	case *ast.StarExpr:
		if sel, ok := ast.Unparen(node.X).(*ast.SelectorExpr); ok {
			return node, sel, true
		}
	case *ast.SelectorExpr:
		inner, ok := ast.Unparen(node.X).(*ast.SelectorExpr)
		if !ok {
			return nil, nil, false
		}
		if selectionDerefsPointer(pass, node) {
			return node, inner, true
		}
	}
	return nil, nil, false
}

// selectionDerefsPointer reports whether reaching sel's member requires
// dereferencing a pointer the receiver expression holds.
//
// A field of the pointee is reached through the pointer, so it
// dereferences. A method resolves on its receiver: a value receiver has
// to be built from the pointee and dereferences, while a pointer
// receiver takes the pointer as it stands and does not. types.Selection
// exposes an Indirect predicate, but it answers a wider question —
// whether any indirection was involved anywhere in reaching the member
// — and reports true for the pointer-receiver case this check has to
// leave alone.
func selectionDerefsPointer(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	selection, ok := pass.TypesInfo.Selections[sel]
	if !ok {
		return false
	}
	if _, isPointer := selection.Recv().Underlying().(*types.Pointer); !isPointer {
		return false
	}
	switch selection.Kind() {
	case types.FieldVal:
		return true
	case types.MethodVal:
		return !methodTakesPointerReceiver(selection.Obj())
	}
	return false
}

// methodTakesPointerReceiver reports whether obj is a method declared
// on a pointer receiver.
func methodTakesPointerReceiver(obj types.Object) bool {
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	_, isPointer := sig.Recv().Type().(*types.Pointer)
	return isPointer
}

// fieldDerefKeyFor resolves the base identifier and the field a
// selector reads, or ok=false when the receiver is not a bare
// identifier or the selection does not name a struct field.
func fieldDerefKeyFor(pass *analysis.Pass, sel *ast.SelectorExpr) (fieldDerefKey, bool) {
	ident, ok := ast.Unparen(sel.X).(*ast.Ident)
	if !ok {
		return fieldDerefKey{}, false
	}
	base := pass.TypesInfo.ObjectOf(ident)
	field, isVar := pass.TypesInfo.ObjectOf(sel.Sel).(*types.Var)
	if base == nil || !isVar || !field.IsField() {
		return fieldDerefKey{}, false
	}
	return fieldDerefKey{base: base, field: field}, true
}

// fieldDerefNarrowed reports whether a guard proves the field non-nil
// where the dereference runs, over the field location the caller-side
// argument check also narrows.
func fieldDerefNarrowed(pass *analysis.Pass, ssaFn *ssa.Function, site ast.Expr, sel *ast.SelectorExpr) bool {
	if ssaFn == nil {
		return false
	}
	subject, ok := fieldSubjectAt(pass, ssaFn, sel)
	if !ok {
		return false
	}
	block, index, ok := ssaFieldReadPosition(ssaFn, sel.Sel.Pos())
	if !ok {
		return false
	}
	subject.useBlock = block
	subject.useIndex = index
	return blockProvesFact(block, subject, factNil, false)
}

// ssaFieldReadPosition locates the field address computed at fieldPos
// and returns its block together with its index inside that block, so
// the proof can be bounded by the instructions that run before the
// read. The walk recurses into function literals so a dereference
// inside a closure resolves against the closure's own graph.
func ssaFieldReadPosition(ssaFn *ssa.Function, fieldPos token.Pos) (*ssa.BasicBlock, int, bool) {
	if ssaFn == nil || fieldPos == token.NoPos {
		return nil, 0, false
	}
	for _, block := range ssaFn.Blocks {
		for i, instr := range block.Instrs {
			fa, ok := instr.(*ssa.FieldAddr)
			if ok && fa.Pos() == fieldPos {
				return block, i, true
			}
		}
	}
	for _, anon := range ssaFn.AnonFuncs {
		if block, i, ok := ssaFieldReadPosition(anon, fieldPos); ok {
			return block, i, true
		}
	}
	return nil, 0, false
}

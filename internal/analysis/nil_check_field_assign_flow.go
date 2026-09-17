package analysis

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// validateFieldNilAssignFlow reports a write into a `!`-declared field
// whose right-hand side the resolver classifies as nillable; the
// literal-nil pass owns the nil spelled in the source.
//
// A write the guard matcher cannot be anchored at goes unreported rather
// than reported with no way out — a diagnostic surviving the guard the
// declaration asks for would refuse its own remedy.
func validateFieldNilAssignFlow(pass *analysis.Pass, state *passState) {
	srcFuncs := ssaSourceFunctions(pass)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Body == nil {
				continue
			}
			reportFieldNilFlowWrites(pass, state, fn, findSSAFunction(srcFuncs, fn))
		}
	}
}

// reportFieldNilFlowWrites walks fn's body for the two write shapes and
// judges each one's right-hand side through the flow resolver.
func reportFieldNilFlowWrites(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function) {
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				sel, ok := ast.Unparen(lhs).(*ast.SelectorExpr)
				if !ok || i >= len(node.Rhs) {
					continue
				}
				judgeFieldNilFlowWrite(pass, state, fn, ssaFn, fieldNilFlowWrite{
					field:  pass.TypesInfo.ObjectOf(sel.Sel),
					value:  node.Rhs[i],
					anchor: sel,
				})
			}
		case *ast.CompositeLit:
			st := structTypeOfCompositeLit(pass, node)
			if st == nil {
				return true
			}
			for i, elt := range node.Elts {
				value, obj := compositeLitFieldWrite(pass, st, i, elt)
				if value == nil {
					continue
				}
				judgeFieldNilFlowWrite(pass, state, fn, ssaFn, fieldNilFlowWrite{
					field: obj,
					value: value,
				})
			}
		}
		return true
	})
}

// fieldNilFlowWrite names one write of value into field. anchor is the
// left-hand selector, nil for a composite-literal element, and is what
// gives the guard matcher a position inside the writing block.
type fieldNilFlowWrite struct {
	field  types.Object
	value  ast.Expr
	anchor *ast.SelectorExpr
}

// judgeFieldNilFlowWrite reports the write when the field is declared
// `!`, the resolver classifies the value as nillable, and no guard
// proves it non-nil where the write runs.
func judgeFieldNilFlowWrite(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, write fieldNilFlowWrite) {
	if argumentIsStaticallyNil(pass, write.value) {
		// The literal-nil pass reports this one; judging it here as well
		// would double up on the same write.
		return
	}
	name, ok := nonNilFieldName(pass, state, write.field)
	if !ok {
		return
	}
	if resolveArgNilness(pass, state, fn, ssaFn, write.value) != nilnessNillable {
		return
	}
	narrowed, anchored := fieldNilFlowWriteNarrowed(pass, ssaFn, write)
	if !anchored || narrowed {
		return
	}
	vowReportNilSafetyf(
		pass,
		write.value.Pos(),
		"vow[nil-safety]: field %s may be assigned nil through flow; %s declared ! on this field",
		name,
		nilDeclMarker,
	)
}

// fieldNilFlowWriteNarrowed reports whether a guard proves the written
// value non-nil, and whether the matcher could be anchored at all;
// anchored=false means "do not report".
//
// A field read needs its own location because the guard's load and the
// write's load are separate SSA values. Everything else is narrowed by
// the value the store receives — which a composite-literal element never
// performs, so a non-field element cannot be anchored.
func fieldNilFlowWriteNarrowed(pass *analysis.Pass, ssaFn *ssa.Function, write fieldNilFlowWrite) (narrowed, anchored bool) {
	if ssaFn == nil {
		return false, false
	}
	if sel, ok := ast.Unparen(write.value).(*ast.SelectorExpr); ok {
		subject, ok := fieldSubjectAt(pass, ssaFn, sel)
		if !ok {
			return false, false
		}
		block, index, ok := ssaFieldReadPosition(ssaFn, sel.Sel.Pos())
		if !ok {
			return false, false
		}
		subject.useBlock = block
		subject.useIndex = index
		return blockProvesFact(block, subject, factNil, false), true
	}
	return storedValueNarrowed(ssaFn, write)
}

// storedValueNarrowed narrows by the value the store receives rather than
// by resolving the expression, so parameter, alias and call result need
// no per-shape case.
func storedValueNarrowed(ssaFn *ssa.Function, write fieldNilFlowWrite) (narrowed, anchored bool) {
	if write.anchor == nil {
		return false, false
	}
	store, block, _, ok := ssaFieldStore(ssaFn, write.anchor.Sel.Pos())
	if !ok {
		return false, false
	}
	return blockProvesFact(block, valueSubject{value: store.Val}, factNil, false), true
}

// ssaFieldStore locates the store writing through the FieldAddr at
// fieldPos, with its block and index. Matching on the address rather than
// on the store's own position keeps this off whatever the builder
// attaches to a store.
func ssaFieldStore(ssaFn *ssa.Function, fieldPos token.Pos) (*ssa.Store, *ssa.BasicBlock, int, bool) {
	if ssaFn == nil || fieldPos == token.NoPos {
		return nil, nil, 0, false
	}
	for _, block := range ssaFn.Blocks {
		for i, instr := range block.Instrs {
			fa, isAddr := instr.(*ssa.FieldAddr)
			if !isAddr || fa.Pos() != fieldPos {
				continue
			}
			for j := i + 1; j < len(block.Instrs); j++ {
				if store, isStore := block.Instrs[j].(*ssa.Store); isStore && store.Addr == fa {
					return store, block, j, true
				}
			}
		}
	}
	for _, anon := range ssaFn.AnonFuncs {
		if store, block, i, ok := ssaFieldStore(anon, fieldPos); ok {
			return store, block, i, true
		}
	}
	return nil, nil, 0, false
}

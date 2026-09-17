package analysis

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// ssaNarrowedPathNilCheck refines a nil-check rule operand whose
// reference carries a postfix path (field, slice index, map key)
// at a call or return site. The walker resolves the path's head to
// the bound argument or receiver expression and dispatches across
// two narrowing tiers:
//
//   - The dominator-guard tier walks the dominator chain at block
//     and matches each terminating `if <expr> != nil` / `if <expr>
//     == nil` whose condition is the SSA composition of the rule's
//     path applied to the head SSA value. A match decides the rule
//     operand for the dominating side; equality and inequality
//     reasoning follow the same flip already used by the bare-
//     identifier nil-check refinement.
//   - The declarative tier walks the path through the head's Go
//     type and reads the field-nil registry at each `StepField`.
//     The terminal field's declared nullness, or the nest body's
//     element / value layer when the terminal step is an index,
//     commits the operand without consulting the SSA. The walker
//     stays conservative — unknown shapes report decided=false so
//     the under-approximation keeps false positives out of the
//     diagnostic.
//
// The decision returns (held, decided). decided=false leaves the
// rule unevaluated at this site so the caller drops it silently.
func ssaNarrowedPathNilCheck(pass *analysis.Pass, state *passState, ssaFn *ssa.Function, headExpr ast.Expr, path []dsl.PathStep, op dsl.CompareOp, block *ssa.BasicBlock) (bool, bool) {
	if op != dsl.OpEq && op != dsl.OpNeq {
		return false, false
	}
	if len(path) == 0 {
		return false, false
	}
	if held, decided := dominatorPathNilCheck(pass, ssaFn, headExpr, path, op, block); decided {
		return held, true
	}
	return declarativePathNilCheck(pass, state, headExpr, path, op)
}

// dominatorPathNilCheck walks the dominator chain at block looking
// for a guard whose condition matches the path-composed SSA value
// rooted at the head's SSA representation. The match is structural:
// the comparison's non-nil operand must reduce through the path
// steps to the head SSA value via the standard Load / FieldAddr /
// IndexAddr / Lookup chain the Go SSA builder emits for an
// expression of the form `head.Field[Key]`.
func dominatorPathNilCheck(pass *analysis.Pass, ssaFn *ssa.Function, headExpr ast.Expr, path []dsl.PathStep, op dsl.CompareOp, block *ssa.BasicBlock) (bool, bool) {
	if ssaFn == nil || block == nil {
		return false, false
	}
	headValue := ssaValueForHeadExpr(pass, ssaFn, headExpr)
	if headValue == nil {
		return false, false
	}
	for cur := block; cur != nil; cur = cur.Idom() {
		idom := cur.Idom()
		if idom == nil {
			return false, false
		}
		ifInstr, ok := terminatingIf(idom)
		if !ok {
			continue
		}
		side, ok := dominantSide(ifInstr, cur)
		if !ok {
			continue
		}
		if held, decided := ifGuardDecidesPathNilOp(ifInstr.Cond, headValue, path, side, op); decided {
			return held, true
		}
	}
	return false, false
}

// ifGuardDecidesPathNilOp reports whether the terminating If
// condition proves the path-composed nil check on the dominating
// side. The condition must be `BinOp <expr> OP nil` where `<expr>`
// reduces to the path applied to headValue; equality and
// inequality flip the sense in the same way the bare-identifier
// helper does.
func ifGuardDecidesPathNilOp(cond ssa.Value, headValue ssa.Value, path []dsl.PathStep, then bool, op dsl.CompareOp) (bool, bool) {
	bop, ok := cond.(*ssa.BinOp)
	if !ok {
		return false, false
	}
	if bop.Op != token.EQL && bop.Op != token.NEQ {
		return false, false
	}
	operand, ok := nonNilOperand(bop)
	if !ok {
		return false, false
	}
	if !ssaValueMatchesPath(operand, headValue, path) {
		return false, false
	}
	var sideIsNil bool
	switch {
	case bop.Op == token.EQL && then:
		sideIsNil = true
	case bop.Op == token.NEQ && then:
		sideIsNil = false
	case bop.Op == token.EQL && !then:
		sideIsNil = false
	case bop.Op == token.NEQ && !then:
		sideIsNil = true
	}
	switch op {
	case dsl.OpEq:
		return sideIsNil, true
	case dsl.OpNeq:
		return !sideIsNil, true
	default:
		return false, false
	}
}

// nonNilOperand returns the non-nil-const operand of a BinOp that
// compares one operand against the nil constant. The helper accepts
// either operand orientation so the caller does not need to know
// the SSA builder's choice.
func nonNilOperand(bop *ssa.BinOp) (ssa.Value, bool) {
	switch {
	case isNilSSAConst(bop.X):
		return bop.Y, true
	case isNilSSAConst(bop.Y):
		return bop.X, true
	}
	return nil, false
}

// ssaValueMatchesPath reports whether value reduces to headValue
// after walking the path steps in reverse through the SSA chain
// the Go builder emits for a `head.Field[Key]`-shaped read. A
// trailing UnOp(MUL) load is unwrapped before each step; FieldAddr
// pins the StepField match, IndexAddr pins integer-index matches,
// Lookup pins string-key matches.
func ssaValueMatchesPath(value ssa.Value, headValue ssa.Value, path []dsl.PathStep) bool {
	for i := len(path) - 1; i >= 0; i-- {
		step := path[i]
		value = unwrapLoad(value)
		next, ok := consumePathStep(value, step)
		if !ok {
			return false
		}
		value = next
	}
	value = unwrapLoad(value)
	return value == headValue
}

// consumePathStep peels one path step off value and returns the
// SSA value the step's container reads from. StepField expects a
// FieldAddr whose Field index resolves to the named field on the
// container's struct type; StepIndex expects an IndexAddr whose
// constant index matches; StepStringKey expects a Lookup whose
// constant key matches; StepIdentKey reports false because the
// matched key is a dynamic SSA value the structural walker cannot
// pin without a constant.
func consumePathStep(value ssa.Value, step dsl.PathStep) (ssa.Value, bool) {
	switch step.Kind {
	case dsl.StepField:
		fa, ok := value.(*ssa.FieldAddr)
		if !ok {
			return nil, false
		}
		if !fieldAddrNames(fa, step.Name) {
			return nil, false
		}
		return fa.X, true
	case dsl.StepIndex:
		ia, ok := value.(*ssa.IndexAddr)
		if !ok {
			return nil, false
		}
		idx, ok := intSSAConst(ia.Index)
		if !ok || idx != int64(step.Index) {
			return nil, false
		}
		return ia.X, true
	case dsl.StepStringKey:
		lk, ok := value.(*ssa.Lookup)
		if !ok {
			return nil, false
		}
		c, ok := lk.Index.(*ssa.Const)
		if !ok || c.Value == nil {
			return nil, false
		}
		if constant.StringVal(c.Value) != step.Name {
			return nil, false
		}
		return lk.X, true
	}
	return nil, false
}

// fieldAddrNames reports whether the FieldAddr's selected struct
// field carries the given name. The struct type is resolved
// through the pointer the FieldAddr addresses; non-struct shapes
// report false so the walker drops the step.
func fieldAddrNames(fa *ssa.FieldAddr, name string) bool {
	ptr, ok := fa.X.Type().Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	st, ok := ptr.Elem().Underlying().(*types.Struct)
	if !ok {
		return false
	}
	if fa.Field < 0 || fa.Field >= st.NumFields() {
		return false
	}
	return st.Field(fa.Field).Name() == name
}

// unwrapLoad peels every trailing UnOp(MUL) off value. The Go SSA
// builder lowers `*p` reads of an addressable selector to a
// UnOp(MUL) on the FieldAddr / IndexAddr; a pointer-to-aggregate
// intermediate (`*[]T`, `*map[K]V`) stacks two such loads in a row,
// so the structural walker peels the chain until a non-load
// instruction surfaces.
func unwrapLoad(value ssa.Value) ssa.Value {
	for {
		un, ok := value.(*ssa.UnOp)
		if !ok || un.Op != token.MUL {
			return value
		}
		value = un.X
	}
}

// ssaValueForHeadExpr resolves headExpr to the SSA value that
// represents it inside ssaFn. The head is a bare identifier that
// resolves to a parameter, receiver, or named return slot at the
// signature level; locals fall through unsupported because the
// path narrowing only matches receivers and parameters.
func ssaValueForHeadExpr(pass *analysis.Pass, ssaFn *ssa.Function, headExpr ast.Expr) ssa.Value {
	ident, ok := ast.Unparen(headExpr).(*ast.Ident)
	if !ok {
		return nil
	}
	return ssaParameterForIdent(pass, ssaFn, ident)
}

// declarativePathNilCheck walks path through the head's Go type
// and reads the field-nil registry to commit on a declared
// nullness. The walker stops at the first step that does not
// resolve through the registry and reports decided=false so the
// caller keeps the under-approximation in place. A terminal
// StepField commits on the field's top-level declared nullness;
// a terminal index step commits on the field's nest body element
// or value layer when the field declares one. Only `NullnessNonNil`
// proves the operand; `NullnessNillable` admits nil so the rule
// stays unresolved at this site.
func declarativePathNilCheck(pass *analysis.Pass, state *passState, headExpr ast.Expr, path []dsl.PathStep, op dsl.CompareOp) (bool, bool) {
	headType := pass.TypesInfo.TypeOf(headExpr)
	if headType == nil {
		return false, false
	}
	nullness, ok := walkPathForDeclaredNullness(pass, state, headType, path)
	if !ok || nullness != dsl.NullnessNonNil {
		return false, false
	}
	return op == dsl.OpNeq, true
}

// walkPathForDeclaredNullness walks path through typ one step at a
// time. StepField unwraps pointers, looks up the field on the
// resulting struct, and consults the field-nil registry; an
// intermediate field may carry a `?` decl that does not by itself
// decide a downstream operand, so the walker continues with the
// field's type rather than committing at the intermediate step.
// The terminal index step (StepIndex / StepStringKey / StepIdentKey)
// commits on the field's nest body element / value layer when the
// preceding StepField field declares one.
func walkPathForDeclaredNullness(pass *analysis.Pass, state *passState, typ types.Type, path []dsl.PathStep) (dsl.Nullness, bool) {
	var lastField *types.Var
	for i, step := range path {
		switch step.Kind {
		case dsl.StepField:
			field, ok := lookupStructField(typ, step.Name)
			if !ok {
				return 0, false
			}
			lastField = field
			typ = field.Type()
			if i == len(path)-1 {
				return terminalFieldNullness(pass, state, field)
			}
		case dsl.StepIndex, dsl.StepStringKey, dsl.StepIdentKey:
			if lastField == nil {
				return 0, false
			}
			if i != len(path)-1 {
				elem, ok := indexableElementType(typ)
				if !ok {
					return 0, false
				}
				typ = elem
				lastField = nil
				continue
			}
			return terminalNestNullness(pass, state, lastField)
		default:
			return 0, false
		}
	}
	return 0, false
}

// terminalFieldNullness reads the field-nil registry for a
// terminal StepField and returns the field's declared top-level
// nullness when present. A field whose decl carries `NullnessNone`
// (a nest-only decl that pins the element layer without naming the
// top-level state) leaves the operand undecided so the walker
// stays sound.
func terminalFieldNullness(pass *analysis.Pass, state *passState, field *types.Var) (dsl.Nullness, bool) {
	decl := fieldNilForField(pass, state, field)
	if decl == nil || decl.Nullness == dsl.NullnessNone {
		return 0, false
	}
	return decl.Nullness, true
}

// terminalNestNullness reads the field-nil registry for a terminal
// index step and returns the nest body's element or value layer
// nullness when the preceding field carries a nest decl. The
// NestDecl exposes a single `Value` slot that covers both the slice
// element and the map value layer, so the walker reads it without
// branching on the step kind. Fields without a nest decl, or
// layers the nest body leaves unconstrained, report ok=false.
func terminalNestNullness(pass *analysis.Pass, state *passState, field *types.Var) (dsl.Nullness, bool) {
	decl := fieldNilForField(pass, state, field)
	if decl == nil || decl.Nest == nil {
		return 0, false
	}
	value := decl.Nest.Value
	if value == nil || value.Nullness == dsl.NullnessNone {
		return 0, false
	}
	return value.Nullness, true
}

package analysis

import (
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// validateZeroStructEscape reports a struct value that is still its zero
// value where it escapes, for every `!`-declared field the zero leaves
// nil. `Box{}` and `var b Box; return b` compile to the same zero
// constant, so both are reached here without the syntactic form
// mattering.
//
// Escapes are read off the instruction rather than off a list of source
// positions: a return, a call operand, a store, a channel send, a map
// update. A comparison is not an escape and is left out deliberately.
//
// A store into storage the escape pass judges is the SSA form of
// assigning to a local, which keeps the value in reach of a later field
// write and so is not an escape here. That covers a member of a local as
// much as the local itself: `h.Inner = Box{}` leaves `h.Inner.Ref` open
// to the next line.
func validateZeroStructEscape(pass *analysis.Pass, state *passState) {
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok || result == nil {
		return
	}
	for _, fn := range result.SrcFuncs {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				if leftToTheEscapePass(instr) {
					continue
				}
				for _, operand := range escapingOperands(instr) {
					reportZeroStructOperand(pass, state, instr, operand)
				}
			}
		}
	}
}

// escapingOperands returns the operands instr lets out of the function's
// own control. Anything not listed keeps its operands to itself.
func escapingOperands(instr ssa.Instruction) []ssa.Value {
	switch node := instr.(type) {
	case *ssa.Return:
		return node.Results
	case *ssa.Store:
		if _, isLocal := node.Addr.(*ssa.Alloc); isLocal {
			// A local slot keeps the value in reach of a later field write.
			return nil
		}
		return []ssa.Value{node.Val}
	case *ssa.Send:
		return []ssa.Value{node.X}
	case *ssa.MapUpdate:
		// A map element is not addressable, so the field can never be
		// filled after this.
		return []ssa.Value{node.Value}
	case ssa.CallInstruction:
		common := node.Common()
		if common == nil {
			return nil
		}
		return append([]ssa.Value{common.Value}, common.Args...)
	}
	return nil
}

// leftToTheEscapePass reports whether the escape pass answers for what
// instr writes. A store into storage that pass carries to a judgement is
// the SSA form of assigning to a local: the field stays in reach of a
// later write, and the value is judged where that storage escapes. Only
// that pass can say whether the later write arrives, so a store it does
// not carry is judged here instead.
func leftToTheEscapePass(instr ssa.Instruction) bool {
	store, ok := instr.(*ssa.Store)
	return ok && addressesJudgedStorage(store.Addr)
}

// addressesJudgedStorage reports whether addr names storage the escape
// pass judges, or a member of such storage.
func addressesJudgedStorage(addr ssa.Value) bool {
	alloc, ok := rootAllocation(addr)
	return ok && allocEscapeIsJudged(alloc)
}

// constantIndexInto reports whether index addresses an element of an
// allocation directly and at a position known at build time. An element
// of an array held inside another one, or inside a struct member, is
// reached through a further address, and the fields-by-value walk does
// not descend into an array to find the field there.
func constantIndexInto(index *ssa.IndexAddr) bool {
	if _, isAlloc := index.X.(*ssa.Alloc); !isAlloc {
		return false
	}
	konst, isConst := index.Index.(*ssa.Const)
	return isConst && konst.Value != nil
}

// reportZeroStructOperand emits one diagnostic per `!`-declared field a
// zero-valued struct operand leaves nil.
func reportZeroStructOperand(pass *analysis.Pass, state *passState, instr ssa.Instruction, operand ssa.Value) {
	konst, ok := operand.(*ssa.Const)
	if !ok || konst.Value != nil {
		return
	}
	st, ok := konst.Type().Underlying().(*types.Struct)
	if !ok {
		return
	}
	for _, field := range nonNilFields(pass, state, st, nil, map[*types.Struct]bool{}) {
		vowReportNilSafetyf(
			pass,
			instr.Pos(),
			"vow[nil-safety]: field %s left nil where this value escapes; %s declared ! on this field",
			field.name,
			nilDeclMarker,
		)
	}
}

// nonNilField is a `!`-declared field reachable from a struct by value,
// carrying the field indices that lead to it so a caller holding the
// storage can ask whether that field was written.
type nonNilField struct {
	name string
	path []int
}

// nonNilFields returns st's `!`-declared fields in declaration order,
// descending into struct-typed fields so a field nested inside an outer
// value is reached. chain drops out of a type that reaches itself, which
// only malformed input can produce, and unwinds so two sibling fields of
// one type are both walked.
func nonNilFields(pass *analysis.Pass, state *passState, st *types.Struct, path []int, chain map[*types.Struct]bool) []nonNilField {
	if chain[st] {
		return nil
	}
	chain[st] = true
	defer delete(chain, st)
	var out []nonNilField
	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		at := append(append([]int{}, path...), i)
		if name, ok := nonNilFieldName(pass, state, field); ok {
			out = append(out, nonNilField{name: name, path: at})
		}
		if inner, ok := field.Type().Underlying().(*types.Struct); ok {
			out = append(out, nonNilFields(pass, state, inner, at, chain)...)
		}
	}
	return out
}

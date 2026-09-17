package analysis

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// argNilNarrowed reports whether the value call passes at argIndex is
// provably non-nil where the call runs, per blockProvesFact.
//
// The subject is the SSA value the call actually passes rather than
// anything resolved from the argument's syntax, so a guard on an alias
// narrows it without the matcher knowing how the alias was spelled.
func argNilNarrowed(ssaFn *ssa.Function, call *ast.CallExpr, argIndex int) bool {
	instr := ssaCallInstruction(ssaFn, call.Lparen)
	if instr == nil {
		return false
	}
	subject, ok := ssaCallArg(instr, call, argIndex)
	if !ok {
		return false
	}
	return blockProvesFact(instr.Block(), valueSubject{value: subject}, factNil, false)
}

// resultNilNarrowed reports whether the value ret supplies at
// resultIndex is provably non-nil where the return runs, under the
// same guard matcher argNilNarrowed consults. Indexing runs against
// the SSA Return's own result list, which is one entry per declared
// result slot, so a `return f()` that spreads a multi-value call
// stays aligned with the signature's slot numbering.
func resultNilNarrowed(ssaFn *ssa.Function, ret *ast.ReturnStmt, resultIndex int) bool {
	ssaRet := findSSAReturn(ssaFn, ret.Return)
	if ssaRet == nil || resultIndex < 0 || resultIndex >= len(ssaRet.Results) {
		return false
	}
	return blockProvesFact(ssaRet.Block(), valueSubject{value: ssaRet.Results[resultIndex]}, factNil, false)
}

// ssaCallInstruction returns the call instruction in ssaFn whose
// source position equals callPos, recursing into function literals so
// a call inside a closure resolves to the closure's own instruction.
// Returns nil when no instruction carries the position — typically a
// call the SSA builder dropped as unreachable.
func ssaCallInstruction(ssaFn *ssa.Function, callPos token.Pos) ssa.CallInstruction {
	if ssaFn == nil || callPos == token.NoPos {
		return nil
	}
	for _, block := range ssaFn.Blocks {
		for _, instr := range block.Instrs {
			call, ok := instr.(ssa.CallInstruction)
			if ok && call.Pos() == callPos {
				return call
			}
		}
	}
	for _, anon := range ssaFn.AnonFuncs {
		if call := ssaCallInstruction(anon, callPos); call != nil {
			return call
		}
	}
	return nil
}

// ssaCallArg maps an argument index on the syntactic call onto the
// matching operand of the SSA call instruction, reporting ok=false
// when no operand corresponds to the index.
//
// The operand list is not the syntactic argument list, and three
// reshapings have to be ruled out before any index arithmetic: a
// variadic callee folds its trailing arguments into one slice operand,
// a method in call mode prepends its receiver, and a multi-value call
// spreads one argument across several operands.
//
// Whether the receiver operand is present is asked of the callee, not
// derived by comparing list lengths — the lengths agree for a variadic
// method, and the offset would then hand back the receiver under an
// argument's index.
func ssaCallArg(instr ssa.CallInstruction, call *ast.CallExpr, argIndex int) (ssa.Value, bool) {
	common := instr.Common()
	if sig := common.Signature(); sig == nil || sig.Variadic() {
		return nil, false
	}
	offset := receiverOperandCount(common)
	args := common.Args
	if len(args) != len(call.Args)+offset {
		return nil, false
	}
	ssaIndex := argIndex + offset
	if ssaIndex < 0 || ssaIndex >= len(args) {
		return nil, false
	}
	return args[ssaIndex], true
}

// receiverOperandCount returns 1 when common's operand list leads
// with a receiver and 0 otherwise. Invoke mode keeps the receiver in
// the call's Value rather than in the operands, and a call through a
// function value or a bound-method closure carries no receiver
// operand either, so only a statically resolved method contributes
// one.
func receiverOperandCount(common *ssa.CallCommon) int {
	if common.IsInvoke() {
		return 0
	}
	callee, ok := common.Value.(*ssa.Function)
	if !ok || callee.Signature == nil || callee.Signature.Recv() == nil {
		return 0
	}
	return 1
}

// fieldArgNarrowed reports whether a field-access argument at a call
// site is provably non-nil where the call runs. The subject is the
// field location — the base value the field is read through, plus the
// field's index — rather than the loaded value, because the guard's
// read of the field and this one are separate loads of the same
// address.
//
// ok=false for every shape the field subject cannot name: a receiver
// that is not a parameter or a method receiver the SSA function
// exposes, a base that is not a pointer to a struct, or a call the SSA
// builder dropped. Those keep the caller's own judgement.
func fieldArgNarrowed(pass *analysis.Pass, ssaFn *ssa.Function, call *ast.CallExpr, arg ast.Expr) bool {
	if ssaFn == nil {
		return false
	}
	sel, ok := ast.Unparen(arg).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	subject, ok := fieldSubjectAt(pass, ssaFn, sel)
	if !ok {
		return false
	}
	block, index, ok := ssaCallPosition(ssaFn, call.Lparen)
	if !ok {
		return false
	}
	subject.useBlock = block
	subject.useIndex = index
	return blockProvesFact(block, subject, factNil, false)
}

// fieldSubjectAt builds the field subject a selector expression names,
// resolving the receiver identifier to the SSA value the enclosing
// function holds for it.
func fieldSubjectAt(pass *analysis.Pass, ssaFn *ssa.Function, sel *ast.SelectorExpr) (fieldSubject, bool) {
	ident, ok := ast.Unparen(sel.X).(*ast.Ident)
	if !ok {
		return fieldSubject{}, false
	}
	base := ssaParameterForIdent(pass, ssaFn, ident)
	if base == nil {
		return fieldSubject{}, false
	}
	field, ok := pass.TypesInfo.ObjectOf(sel.Sel).(*types.Var)
	if !ok {
		return fieldSubject{}, false
	}
	return fieldSubjectForSelector(base, field)
}

// ssaCallPosition locates the call instruction at callPos and returns
// its block together with its index inside that block, so a proof can
// be bounded by the instructions that run before it.
func ssaCallPosition(ssaFn *ssa.Function, callPos token.Pos) (*ssa.BasicBlock, int, bool) {
	if ssaFn == nil || callPos == token.NoPos {
		return nil, 0, false
	}
	for _, block := range ssaFn.Blocks {
		for i, instr := range block.Instrs {
			call, ok := instr.(ssa.CallInstruction)
			if ok && call.Pos() == callPos {
				return block, i, true
			}
		}
	}
	for _, anon := range ssaFn.AnonFuncs {
		if block, i, ok := ssaCallPosition(anon, callPos); ok {
			return block, i, true
		}
	}
	return nil, 0, false
}

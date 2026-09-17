package analysis

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// nilness is the abstract value the SSA forward walk attaches to an
// ssa.Value. The four states form a small lattice with KnownNil and
// NonNil as the two leaves and Unknown as the top; Nillable is a
// middle state used when a declaration admits nil but no proof at
// the use site has narrowed it either way.
type nilness int

const (
	nilnessUnknown  nilness = iota // no proof either way
	nilnessNonNil                  // proved non-nil
	nilnessNillable                // admits nil (declared `?` or computed from declared `?`)
	nilnessKnownNil                // proved nil (literal nil / typed-nil conversion)
)

// validateNilDeclCalleeFlow reports a value riding into a position
// pinned as `!`: at a call the callee's signature decl pins it, at a
// return the enclosing function's own decl does. Only a value the
// resolver classifies as Nillable is reported. A statically nil one
// belongs to the AST-only pass and is skipped here, and NonNil and
// Unknown are silent, so the walk under-reports rather than guesses.
//
// resolveArgNilness holds which expressions the resolver classifies.
// Two limits sit outside it and belong here. A value that arrives only
// as an SSA node — a phi merge, an Alloc, a Make* node — is recognised
// by ssaValueNilness and never reaches it, because its one entry hands
// it a parameter and that branch answers without recursing. And an
// identifier is answered from the definition that introduces it, so a
// later assignment to the same name does not move the answer.
func validateNilDeclCalleeFlow(pass *analysis.Pass, state *passState) {
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok || result == nil {
		return
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Body == nil {
				continue
			}
			ssaFn := findSSAFunction(result.SrcFuncs, fn)
			if ssaFn == nil {
				continue
			}
			walkCallAndReturnNilness(pass, state, fn, ssaFn)
		}
	}
}

// walkCallAndReturnNilness walks fn's body and inspects every call
// site argument against the callee's `vow:nil` signature, then
// every return statement against the enclosing function's own
// signature. Each candidate position is rendered through the
// nilness-resolver below; positions the resolver cannot classify
// (Unknown / NonNil) stay silent.
func walkCallAndReturnNilness(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function) {
	callerSig := callerOwnSignature(pass, state, fn)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			checkCallArgNilFlow(pass, state, fn, ssaFn, node)
		case *ast.ReturnStmt:
			if callerSig != nil {
				checkReturnNilFlow(pass, state, fn, ssaFn, callerSig, node)
			}
		}
		return true
	})
}

// callerOwnSignature returns the vow:nil signature decl attached to
// fn, or nil when fn carries no decl. The return-statement check
// consults this signature to learn which return slots are pinned
// at `!`.
func callerOwnSignature(pass *analysis.Pass, state *passState, fn *ast.FuncDecl) *dsl.NilSignature {
	obj := pass.TypesInfo.Defs[fn.Name]
	if obj == nil {
		return nil
	}
	return state.nilDeclSet(pass)[obj]
}

// checkCallArgNilFlow reports nillable / known-nil arguments that
// ride into non-nil-required positions on the callee's signature.
// Arguments that are direct field-access expressions are handled
// by the AST-only caller-side check and skipped here so the two
// surfaces never double-report. The flow check only fires when the
// resolver returns Nillable; Unknown / NonNil positions stay silent.
func checkCallArgNilFlow(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, call *ast.CallExpr) {
	callee := calleeObject(pass, call)
	if callee == nil {
		return
	}
	sig := state.nilDeclSet(pass)[callee]
	if sig == nil || len(sig.Params) == 0 {
		return
	}
	paramNames := calleeParamNames(pass, callee)
	for i, decl := range sig.Params {
		if decl == nil || decl.Nullness != dsl.NullnessNonNil {
			continue
		}
		if i >= len(call.Args) {
			continue
		}
		arg := call.Args[i]
		if argumentIsStaticallyNil(pass, arg) {
			continue
		}
		if _, alreadyAST := argumentIsDeclaredNillableField(pass, state, arg); alreadyAST {
			continue
		}
		flow := resolveArgNilness(pass, state, fn, ssaFn, arg)
		if flow != nilnessNillable {
			continue
		}
		if argProvenNonNil(pass, state, ssaFn, call, i) {
			continue
		}
		if argNilNarrowed(ssaFn, call, i) {
			continue
		}
		vowReportNilSafetyf(
			pass,
			arg.Pos(),
			"vow[nil-safety]: argument %d (%s) may be nil through flow; %s declared ! at this position",
			i+1,
			paramNameAt(paramNames, i),
			nilDeclMarker,
		)
	}
}

// checkReturnNilFlow reports nillable return expressions that ride
// into a `!` return slot on the enclosing function's signature.
// Literal-nil returns are handled by the AST-only self-validation
// pass and skipped here so the two paths do not double-report.
func checkReturnNilFlow(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, sig *dsl.NilSignature, ret *ast.ReturnStmt) {
	if len(sig.Returns) == 0 || len(ret.Results) == 0 {
		return
	}
	for i, decl := range sig.Returns {
		if decl == nil || decl.Nullness != dsl.NullnessNonNil {
			continue
		}
		if i >= len(ret.Results) {
			continue
		}
		expr := ret.Results[i]
		if argumentIsStaticallyNil(pass, expr) {
			continue
		}
		flow := resolveArgNilness(pass, state, fn, ssaFn, expr)
		if flow != nilnessNillable {
			continue
		}
		if resultProvenNonNil(pass, state, ssaFn, ret, i) {
			continue
		}
		if resultNilNarrowed(ssaFn, ret, i) {
			continue
		}
		vowReportNilSafetyf(
			pass,
			expr.Pos(),
			"vow[nil-safety]: return position %d may be nil through flow; %s declared ! at this return slot",
			i+1,
			nilDeclMarker,
		)
	}
}

// argProvenNonNil reports whether the value the call hands to the
// callee at argIndex is provably non-nil. The declaration resolver
// answers for a local name from the definition that introduces it, so
// a value the function replaced before the call still reads as the
// definition's state; the call's own operand is what the callee
// receives.
func argProvenNonNil(pass *analysis.Pass, state *passState, ssaFn *ssa.Function, call *ast.CallExpr, argIndex int) bool {
	instr := ssaCallInstruction(ssaFn, call.Lparen)
	if instr == nil {
		return false
	}
	subject, ok := ssaCallArg(instr, call, argIndex)
	if !ok {
		return false
	}
	return ssaValueNilness(pass, state, subject, map[ssa.Value]struct{}{}) == nilnessNonNil
}

// resultProvenNonNil is argProvenNonNil for a return slot, reading the
// operand the SSA Return carries at resultIndex.
func resultProvenNonNil(pass *analysis.Pass, state *passState, ssaFn *ssa.Function, ret *ast.ReturnStmt, resultIndex int) bool {
	ssaRet := findSSAReturn(ssaFn, ret.Return)
	if ssaRet == nil || resultIndex < 0 || resultIndex >= len(ssaRet.Results) {
		return false
	}
	return ssaValueNilness(pass, state, ssaRet.Results[resultIndex], map[ssa.Value]struct{}{}) == nilnessNonNil
}

// resolveArgNilness returns the abstract nilness of expr at the call
// or return position, dispatching on the shape of the expression:
//
//   - a parameter reference, through the SSA representation, so the
//     enclosing function's signature decl flows in;
//   - any other local identifier, through the definition that
//     introduces it and through whatever that definition's right-hand
//     side resolves to in turn;
//   - a selector expression, from the field registry;
//   - a dereference of a field read, from the layer that dereference
//     lands on;
//   - an index expression, from the element layer its container
//     field's nest decl states;
//   - a call, from the single return slot its callee declares;
//   - an address-of expression, which is non-nil on its own.
//
// A shape outside the list returns Unknown so the caller stays
// silent.
func resolveArgNilness(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, expr ast.Expr) nilness {
	expr = ast.Unparen(expr)
	switch e := expr.(type) {
	case *ast.Ident:
		if v := ssaParameterForIdent(pass, ssaFn, e); v != nil {
			return ssaValueNilness(pass, state, v, map[ssa.Value]struct{}{})
		}
		return aliasNilnessForLocalIdent(pass, state, fn, ssaFn, e)
	case *ast.SelectorExpr:
		return fieldSelectorNilness(pass, state, fn, e)
	case *ast.StarExpr:
		return derefFieldNilness(pass, state, fn, e)
	case *ast.IndexExpr:
		return indexedElementNilness(pass, state, e)
	case *ast.CallExpr:
		return callResultNilness(pass, state, e)
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return nilnessNonNil
		}
	}
	return nilnessUnknown
}

// indexedElementNilness returns the abstract state of an element
// read through a slice index or a map lookup expression. The X
// side must resolve to a field whose decl carries a nest body; the
// nest body's Value position dictates the element's declared
// nullness. Other shapes (an indexed local, a non-decl-bearing
// field, a top-level slice variable) fall through unreported as
// Unknown so the upper-bound check stays silent on positions the
// nest grammar leaves unconstrained.
func indexedElementNilness(pass *analysis.Pass, state *passState, idx *ast.IndexExpr) nilness {
	sel, ok := ast.Unparen(idx.X).(*ast.SelectorExpr)
	if !ok {
		return nilnessUnknown
	}
	obj := pass.TypesInfo.ObjectOf(sel.Sel)
	if obj == nil {
		return nilnessUnknown
	}
	decl := fieldNilForField(pass, state, obj)
	if decl == nil || decl.Nest == nil {
		return nilnessUnknown
	}
	elem := decl.Nest.Value
	if elem == nil {
		return nilnessUnknown
	}
	switch elem.Nullness {
	case dsl.NullnessNillable:
		return nilnessNillable
	case dsl.NullnessNonNil:
		return nilnessNonNil
	}
	return nilnessUnknown
}

// fieldSelectorNilness returns the abstract state of a direct
// field-access expression, which reads the field's outermost layer.
// The field's own decl answers when its run states a token for that
// layer — the run's first. A decl stating none there, one holding a
// nest body alone, leaves the answer to the nest decl on the
// container: a field decl wins by stating the layer, not by existing.
//
// The fallback goes to genericFieldLayerNilness, which reads the nest
// decl carried at the container's own parameter position and states
// the shapes it declines.
func fieldSelectorNilness(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, sel *ast.SelectorExpr) nilness {
	obj := pass.TypesInfo.ObjectOf(sel.Sel)
	if obj == nil {
		return nilnessUnknown
	}
	if decl := fieldNilForField(pass, state, obj); decl != nil {
		switch runNullnessAt(decl, 0) {
		case dsl.NullnessNillable:
			return nilnessNillable
		case dsl.NullnessNonNil:
			return nilnessNonNil
		}
	}
	return genericFieldLayerNilness(pass, state, fn, sel, 0)
}

// derefFieldNilness answers for a dereference of a field read, and
// only for that: the field read itself carries no star and reaches
// fieldSelectorNilness instead. The stars are counted rather than just
// peeled, because a token run states one layer per dereference counted
// from the field read: `box.V` takes the run's first token and
// `*box.V` the next. Counting the expression is what keeps a token for
// one layer from answering for another — under `?!` the analyzer has
// to keep calling `box.V` nillable whatever the run says about
// `*box.V`.
//
// Which decl answers follows the rule the outermost layer uses: the
// field's own decl takes this layer when its run states a token for
// it, and leaves it to the container's nest decl when it states none.
// An unresolved selector is where the two part — this one still tries
// the container, where fieldSelectorNilness stops.
// Shapes with no field selector under the stars fall through
// unreported as Unknown.
func derefFieldNilness(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, star *ast.StarExpr) nilness {
	layer := 0
	var inner ast.Expr = star
	for {
		next, ok := ast.Unparen(inner).(*ast.StarExpr)
		if !ok {
			break
		}
		layer++
		inner = next.X
	}
	sel, ok := ast.Unparen(inner).(*ast.SelectorExpr)
	if !ok {
		return nilnessUnknown
	}
	if obj := pass.TypesInfo.ObjectOf(sel.Sel); obj != nil {
		if decl := fieldNilForField(pass, state, obj); decl != nil {
			switch runNullnessAt(decl, layer) {
			case dsl.NullnessNillable:
				return nilnessNillable
			case dsl.NullnessNonNil:
				return nilnessNonNil
			}
		}
	}
	return genericFieldLayerNilness(pass, state, fn, sel, layer)
}

// runNullnessAt returns what decl's token run states about the layer
// standing `layer` dereferences inward from the outermost, which is
// layer 0. NullnessNone comes back both for a layer past the run's end
// and for one the run reaches without a token, since a decl may carry
// a nest body and no token of its own; a caller that treats them alike
// is right to, because neither states anything about this layer.
func runNullnessAt(decl *dsl.PositionDecl, layer int) dsl.Nullness {
	if layer == 0 {
		return decl.Nullness
	}
	if layer-1 >= len(decl.InnerLayers) {
		return dsl.NullnessNone
	}
	return decl.InnerLayers[layer-1]
}

// genericFieldLayerNilness returns the nilness one layer of a field
// read carries, where the container is a generic instantiation held by
// one of the enclosing function's parameters. The field's declared
// type must reach a type parameter of the container through any number
// of pointers; the decl at the parameter's own position supplies the
// nest body, and the type parameter's index selects the inner decl
// within it whose run states the layers of what a read yields.
//
// Three conditions take a shape out, each answering Unknown so the
// caller reports nothing: a receiver that is not a plain parameter
// identifier (chained selectors and method receivers are not covered),
// a field whose type reaches no type parameter, and a layer whose own
// type cannot hold nil. fn may be nil.
func genericFieldLayerNilness(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, sel *ast.SelectorExpr, layer int) nilness {
	if fn == nil || sel.Sel == nil {
		return nilnessUnknown
	}
	ident, ok := ast.Unparen(sel.X).(*ast.Ident)
	if !ok {
		return nilnessUnknown
	}
	recvObj := pass.TypesInfo.ObjectOf(ident)
	if recvObj == nil {
		return nilnessUnknown
	}
	paramIdx := paramIndexInDecl(pass, fn, recvObj)
	if paramIdx < 0 {
		return nilnessUnknown
	}
	originType := originFieldType(recvObj.Type(), sel.Sel.Name)
	typeArgIdx := typeParamIndex(recvObj.Type(), peelPointers(originType))
	if typeArgIdx < 0 {
		return nilnessUnknown
	}
	if !typeIsNillable(layerType(pass.TypesInfo.TypeOf(sel), layer)) {
		return nilnessUnknown
	}
	sig := callerOwnSignature(pass, state, fn)
	if sig == nil || paramIdx >= len(sig.Params) {
		return nilnessUnknown
	}
	paramDecl := sig.Params[paramIdx]
	if paramDecl == nil || paramDecl.Nest == nil || typeArgIdx >= len(paramDecl.Nest.InnerDecls) {
		return nilnessUnknown
	}
	inner := paramDecl.Nest.InnerDecls[typeArgIdx]
	if inner == nil {
		return nilnessUnknown
	}
	switch runNullnessAt(inner, layer) {
	case dsl.NullnessNillable:
		return nilnessNillable
	case dsl.NullnessNonNil:
		return nilnessNonNil
	}
	return nilnessUnknown
}

// layerType returns the type standing `layer` dereferences inward from
// t, or nil where t does not admit that many. Only a pointer is
// stepped through: the element of a slice or map is a layer the nest
// body reaches, not one a dereference does. A nil answer is not a
// type, so a caller asking whether it can hold nil gets false and
// falls through unreported.
func layerType(t types.Type, layer int) types.Type {
	for i := 0; i < layer; i++ {
		if t == nil {
			return nil
		}
		ptr, ok := t.Underlying().(*types.Pointer)
		if !ok {
			return nil
		}
		t = ptr.Elem()
	}
	return t
}

// paramIndexInDecl returns the argument-index position of target
// inside fn's parameter list, or -1 when target is not a regular
// parameter. The walk expands grouped declarations the same way
// signatureParamNames does so the index stays argument-aligned.
func paramIndexInDecl(pass *analysis.Pass, fn *ast.FuncDecl, target types.Object) int {
	if fn == nil || fn.Type == nil || fn.Type.Params == nil {
		return -1
	}
	idx := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			idx++
			continue
		}
		for _, name := range field.Names {
			if pass.TypesInfo.ObjectOf(name) == target {
				return idx
			}
			idx++
		}
	}
	return -1
}

// aliasNilnessForLocalIdent answers for a local identifier from the
// `:=` that introduces it, resolving that definition's right-hand side
// through the same resolver the caller came from. A chain of
// definitions therefore resolves to the state at its far end.
//
// The match is on the define alone: a `var` declaration is not one,
// and a define whose left side outnumbers its right — the second name
// of `a, b := f()` — leaves no right-hand side to resolve and answers
// Unknown. A later assignment to the same name does not displace the
// definition here; what the value is where it is used is settled at
// the use site, against the SSA operand.
func aliasNilnessForLocalIdent(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, ident *ast.Ident) nilness {
	obj := pass.TypesInfo.ObjectOf(ident)
	if obj == nil || fn.Body == nil {
		return nilnessUnknown
	}
	var rhs ast.Expr
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if rhs != nil {
			return false
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE {
			return true
		}
		for i, lhs := range assign.Lhs {
			lhsIdent, ok := lhs.(*ast.Ident)
			if !ok {
				continue
			}
			if pass.TypesInfo.ObjectOf(lhsIdent) != obj {
				continue
			}
			if i < len(assign.Rhs) {
				rhs = assign.Rhs[i]
			}
			return false
		}
		return true
	})
	if rhs == nil {
		return nilnessUnknown
	}
	return resolveArgNilness(pass, state, fn, ssaFn, rhs)
}

// ssaParameterForIdent maps an AST identifier that names a
// parameter onto its SSA Parameter. Locals and field accesses
// resolve through other paths; this helper handles only the
// parameter case because the SSA forward walk reads each
// parameter's declared nullness through the enclosing function's
// signature decl.
func ssaParameterForIdent(pass *analysis.Pass, ssaFn *ssa.Function, ident *ast.Ident) ssa.Value {
	obj := pass.TypesInfo.ObjectOf(ident)
	if obj == nil {
		return nil
	}
	for _, param := range ssaFn.Params {
		if param.Object() == obj {
			return param
		}
	}
	return nil
}

// ssaValueNilness recurses through phi joins and trivial deref
// loads, returning the abstract nilness of v. Cycles are broken by
// the visited set; a value re-entered during the same walk returns
// Unknown so a phi self-reference does not collapse to a wrong
// constant. The lattice join is conservative on the upper-bound
// side: KnownNil < Nillable < Unknown, with NonNil being the only
// proof-side leaf.
func ssaValueNilness(pass *analysis.Pass, state *passState, v ssa.Value, visited map[ssa.Value]struct{}) nilness {
	if v == nil {
		return nilnessUnknown
	}
	if _, seen := visited[v]; seen {
		return nilnessUnknown
	}
	visited[v] = struct{}{}
	switch n := v.(type) {
	case *ssa.Const:
		if n.IsNil() {
			return nilnessKnownNil
		}
		return nilnessNonNil
	case *ssa.Parameter:
		return parameterNilness(pass, state, n)
	case *ssa.Phi:
		return joinPhi(pass, state, n, visited)
	case *ssa.UnOp:
		if n.Op == token.MUL {
			return ssaValueNilness(pass, state, n.X, visited)
		}
	case *ssa.Alloc, *ssa.MakeInterface, *ssa.MakeClosure, *ssa.MakeMap, *ssa.MakeSlice, *ssa.MakeChan:
		return nilnessNonNil
	case *ssa.FieldAddr:
		return fieldAddrNilness(pass, state, n)
	}
	return nilnessUnknown
}

// parameterNilness consults the enclosing function's signature
// decl to return the parameter's declared abstract state. Functions
// without a decl, or positions not pinned by the decl, return
// Unknown so the upper path does not over-claim.
func parameterNilness(pass *analysis.Pass, state *passState, param *ssa.Parameter) nilness {
	ssaFn := param.Parent()
	if ssaFn == nil {
		return nilnessUnknown
	}
	fnDecl, ok := ssaFn.Syntax().(*ast.FuncDecl)
	if !ok {
		return nilnessUnknown
	}
	obj := pass.TypesInfo.Defs[fnDecl.Name]
	if obj == nil {
		return nilnessUnknown
	}
	sig := state.nilDeclSet(pass)[obj]
	if sig == nil || len(sig.Params) == 0 {
		return nilnessUnknown
	}
	idx := paramIndex(ssaFn, param)
	if idx < 0 || idx >= len(sig.Params) {
		return nilnessUnknown
	}
	decl := sig.Params[idx]
	if decl == nil {
		return nilnessUnknown
	}
	switch decl.Nullness {
	case dsl.NullnessNonNil:
		return nilnessNonNil
	case dsl.NullnessNillable:
		return nilnessNillable
	}
	return nilnessUnknown
}

// paramIndex returns the index of param inside ssaFn.Params,
// accounting for a leading receiver slot when present. The
// signature decl's Params slice is indexed by the regular parameter
// list only, so a receiver slot is skipped before the comparison.
func paramIndex(ssaFn *ssa.Function, param *ssa.Parameter) int {
	recvCount := 0
	if ssaFn.Signature.Recv() != nil {
		recvCount = 1
	}
	for i, p := range ssaFn.Params {
		if p == param {
			return i - recvCount
		}
	}
	return -1
}

// joinPhi joins the abstract states of every phi edge under the
// conservative upper-bound rule. Any edge that resolves to NonNil
// alongside others does not lift the whole phi to NonNil (because a
// nillable predecessor remains a possibility); a Nillable edge
// dominates the join unless every edge resolves to NonNil.
//
// An edge counts as NonNil when the control-flow edge carrying it
// proves so, even where the value itself does not: `if v == nil { v =
// fallback }` reaches the join with the original value on the edge the
// nil test rejected.
func joinPhi(pass *analysis.Pass, state *passState, phi *ssa.Phi, visited map[ssa.Value]struct{}) nilness {
	block := phi.Block()
	result := nilnessUnknown
	allNonNil := true
	for i, edge := range phi.Edges {
		edgeState := ssaValueNilness(pass, state, edge, visited)
		if edgeState != nilnessNonNil && edgeCarriesNonNil(block, i, edge) {
			edgeState = nilnessNonNil
		}
		if edgeState == nilnessNillable || edgeState == nilnessKnownNil {
			result = nilnessNillable
			allNonNil = false
			continue
		}
		if edgeState != nilnessNonNil {
			allNonNil = false
		}
	}
	if allNonNil && len(phi.Edges) > 0 {
		return nilnessNonNil
	}
	return result
}

// edgeCarriesNonNil reports whether the control-flow edge that feeds a
// phi operand proves the operand non-nil where that edge is taken. The
// operand's index into the phi matches its predecessor's index into
// the join block, which is what lets a per-edge question be asked at
// all.
func edgeCarriesNonNil(block *ssa.BasicBlock, index int, edge ssa.Value) bool {
	if block == nil || index < 0 || index >= len(block.Preds) {
		return false
	}
	return edgeProvesFact(block.Preds[index], block, valueSubject{value: edge}, factNil, false, map[*ssa.BasicBlock]bool{})
}

// fieldAddrNilness returns the abstract state of a *ssa.FieldAddr's
// loaded value by resolving the underlying struct field's
// declaration through the field-nil registry. The address itself
// is non-nil; the load that follows carries the field's declared
// nullness, which the resolver returns when the field's parent
// type's TypesInfo points back to the field's Var.
func fieldAddrNilness(pass *analysis.Pass, state *passState, fa *ssa.FieldAddr) nilness {
	stType, ok := fa.X.Type().Underlying().(*types.Pointer)
	if !ok {
		return nilnessUnknown
	}
	st, ok := stType.Elem().Underlying().(*types.Struct)
	if !ok {
		return nilnessUnknown
	}
	if fa.Field < 0 || fa.Field >= st.NumFields() {
		return nilnessUnknown
	}
	field := st.Field(fa.Field)
	decl := fieldNilForField(pass, state, field)
	if decl == nil {
		return nilnessUnknown
	}
	switch decl.Nullness {
	case dsl.NullnessNillable:
		return nilnessNillable
	case dsl.NullnessNonNil:
		return nilnessNonNil
	}
	return nilnessUnknown
}

// calleeParamNames returns the regular parameter names declared on
// callee's signature. The names mirror what an *ast.FuncDecl
// exposes so the diagnostic carries the same labels the AST path
// would have produced; unnamed positions render as the empty
// string.
func calleeParamNames(pass *analysis.Pass, callee types.Object) []string {
	fn, ok := callee.(*types.Func)
	if !ok {
		return nil
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return nil
	}
	params := sig.Params()
	out := make([]string, 0, params.Len())
	for i := 0; i < params.Len(); i++ {
		out = append(out, params.At(i).Name())
	}
	return out
}

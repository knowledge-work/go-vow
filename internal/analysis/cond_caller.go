package analysis

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateCallerLogicalConds scans every call site in pass.Files
// and reports when the caller's argument or receiver literals
// contradict a logical-arrow vow:cond rule the callee declared on
// its function-doc. The evaluator anchors each Reference operand
// at the slot of the callee's signature it names — the receiver
// for the declared receiver name, regular parameters for
// declared parameter names — and decides the operand's boolean
// outcome from the literal shape of the argument the caller
// supplied at that position. References that bind to a return
// value (`$N` positional, named-return identifiers) or that
// carry a postfix chain report undecided at the call site; the
// callee-side return-statement evaluator covers the former and
// the SSA-driven refinement pass covers the latter.
//
// A diagnostic fires only when both operands of a rule resolve to
// known literal nil or known non-nil values at the same call site
// and the boolean combination contradicts the rule's direction.
// Same-package callees consult the in-memory rule cache; cross-
// package callees consult the exported conditionLogicalFact so
// the per-rule contract travels with the function across package
// boundaries.
func validateCallerLogicalConds(pass *analysis.Pass, state *passState) {
	ssaFns := ssaFunctions(pass)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Body == nil {
					continue
				}
				ssaFn := findSSAFunction(ssaFns, d)
				walkCallSitesForLogicalConds(pass, state, d, ssaFn, d.Body)
			case *ast.GenDecl:
				// Top-level var initialisers may contain call
				// expressions to functions that carry logical-arrow
				// rules. The enclosing SSA function is absent at
				// this scope, so the evaluator runs through its
				// literal-only path; that still catches the same
				// literal-decided contradictions the function-body
				// walk would catch.
				walkCallSitesForLogicalConds(pass, state, nil, nil, d)
			}
		}
	}
}

// walkCallSitesForLogicalConds visits every call expression under
// root and runs the logical-arrow rule check against each. The fn
// and ssaFn arguments name the enclosing function for the SSA
// refinement path; a top-level node has no enclosing function and
// passes nil for both, so the evaluator skips the SSA layers and
// runs through its literal-only path instead.
func walkCallSitesForLogicalConds(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, root ast.Node) {
	ast.Inspect(root, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		checkCallSiteLogicalConds(pass, state, fn, ssaFn, call)
		return true
	})
}

// ssaCallBlock returns the SSA basic block whose instruction list
// contains the call expression at callPos. The Go SSA builder
// records each `*ssa.Call` instruction at its source `Lparen`
// position, so the caller passes `call.Lparen` rather than the
// call expression's leading position. The walk recurses into
// `ssaFn.AnonFuncs` so a call inside a function literal still
// resolves to the host block of its enclosing closure rather than
// degrading silently to a nil block. Returns nil when no
// instruction carries the expected position — typically the
// expression sits in a context the SSA builder dropped, for
// instance an unreachable branch eliminated by dead-code analysis.
func ssaCallBlock(ssaFn *ssa.Function, callPos token.Pos) *ssa.BasicBlock {
	if ssaFn == nil || callPos == token.NoPos {
		return nil
	}
	for _, block := range ssaFn.Blocks {
		for _, instr := range block.Instrs {
			if instr.Pos() == callPos {
				return block
			}
		}
	}
	for _, anon := range ssaFn.AnonFuncs {
		if block := ssaCallBlock(anon, callPos); block != nil {
			return block
		}
	}
	return nil
}

// checkCallSiteLogicalConds resolves the callee for call, looks
// up its logical-arrow rules, and evaluates each rule against the
// call's argument literals. The enclosing function and its SSA
// counterpart let the SSA refinement narrow argument identifiers
// through alias chains and dominating if-guards at the call site.
func checkCallSiteLogicalConds(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, call *ast.CallExpr) {
	callee := calleeObject(pass, call)
	if callee == nil {
		return
	}
	rules := logicalRulesForCallee(pass, state, callee)
	if len(rules) == 0 {
		return
	}
	calleeFn, ok := callee.(*types.Func)
	if !ok {
		return
	}
	sig, ok := calleeFn.Type().(*types.Signature)
	if !ok {
		return
	}
	paramNames := signatureParamNamesFromTypes(sig)
	recvName := ""
	if recv := sig.Recv(); recv != nil {
		recvName = recv.Name()
	}
	block := ssaCallBlock(ssaFn, call.Lparen)
	for _, group := range groupRulesWithContrapositives(rules) {
		if evaluateLogicalRuleAtCall(pass, state, fn, ssaFn, block, call, group.Original, nil, "", sig, paramNames, recvName) {
			continue
		}
		if group.Contrapositive != nil {
			if evaluateLogicalRuleAtCall(pass, state, fn, ssaFn, block, call, group.Contrapositive, group.Original, "", sig, paramNames, recvName) {
				continue
			}
		}
		for _, t := range group.Transitives {
			if evaluateLogicalRuleAtCall(pass, state, fn, ssaFn, block, call, t.Rule, nil, "transitive closure of "+t.ChainDesc, sig, paramNames, recvName) {
				break
			}
		}
	}
}

// evaluateLogicalRuleAtCall drives one rule against the call site
// and returns true when the evaluator committed (holding or
// violating with a reported diagnostic). The caller uses the
// return flag to skip the derived contrapositive and any chain
// closure when the original already decided so the three never
// report the same offending claim twice. Diagnostics anchor at the
// call expression so the author sees the offending callsite rather
// than the callee declaration. The block argument hosts the SSA
// narrowing path; passing nil for it degrades the evaluator to the
// literal-only behaviour. The origin argument names the source
// rule when rule itself is a derived contrapositive; the diagnostic
// renders the source rule with a `contrapositive of` prefix. The
// descOverride argument, when non-empty, replaces the rule
// rendering entirely so chain-closure diagnostics announce the
// full chain rather than the synthetic derived rule's surface form.
func evaluateLogicalRuleAtCall(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, block *ssa.BasicBlock, call *ast.CallExpr, rule *dsl.ArrowRule, origin *dsl.ArrowRule, descOverride string, sig *types.Signature, paramNames []string, recvName string) bool {
	if rule == nil || rule.Left == nil || rule.Right == nil {
		return false
	}
	lhs, lOk := evaluateOperandAtCall(pass, state, fn, ssaFn, block, call, rule.Left, paramNames, recvName)
	if !lOk {
		return false
	}
	rhs, rOk := evaluateOperandAtCall(pass, state, fn, ssaFn, block, call, rule.Right, paramNames, recvName)
	if !rOk {
		return false
	}
	if !logicalRuleHolds(lhs, rhs, rule.Direction) {
		desc := descOverride
		if desc == "" {
			desc = describeLogicalRule(rule, origin)
		}
		reason := logicalViolationReason(lhs, rhs, rule, "call")
		// The lifecycle-pair qualifier rides on top of the
		// per-rule diagnostic only. Transitive-closure synthetic
		// rules carry their chain rendering through descOverride;
		// gating the qualifier on an empty descOverride keeps the
		// chain narrative ("transitive closure of … and …")
		// readable instead of mixing it with the pair narrative.
		if descOverride == "" && isLifecyclePairRule(pass, state, rule, sig, paramNames, recvName) {
			vowReportf(pass, call.Pos(), "%s", describeLifecyclePairCallViolation(desc, reason))
		} else {
			vowReportf(pass, call.Pos(), "vow[sentinel-error]: call violates %s: %s", desc, reason)
		}
	}
	return true
}

// evaluateOperandAtCall decides the boolean outcome of expr at
// call. The triple return is (held bool, decided bool); decided
// is false when neither the literal-only evaluator nor the SSA
// refinement reached a commitment. References to parameters and
// the receiver bind to the matching argument expression at the
// call; references to return slots stay undecided because the
// call has not produced them.
func evaluateOperandAtCall(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, block *ssa.BasicBlock, call *ast.CallExpr, expr *dsl.Expression, paramNames []string, recvName string) (bool, bool) {
	if expr == nil {
		return false, false
	}
	bound, ok := resolveReferenceAtCall(call, expr.Ref, paramNames, recvName)
	if ok {
		switch expr.Kind {
		case dsl.ExprNilCheck:
			if held, decided := evaluateNilCheck(pass, bound, expr.Op); decided {
				return held, true
			}
			return ssaNarrowedNilCheck(pass, state, fn, ssaFn, bound, expr.Op, block)
		case dsl.ExprComparison:
			return evaluateComparisonMembers(pass, bound, expr.Op, expr.Members)
		}
		return false, false
	}
	// Path-bearing references (`recv.Field[idx]`, `arg.Map["key"]`)
	// resolve through the path-narrowing helper. The head binds to a
	// receiver or argument expression at the call; the path walks
	// through the field-nil registry for a declarative commit and
	// through the SSA dominator chain for a guard-narrowed commit.
	head, path, ok := resolvePathHeadAtCall(call, expr.Ref, paramNames, recvName)
	if !ok {
		return false, false
	}
	if expr.Kind != dsl.ExprNilCheck {
		return false, false
	}
	return ssaNarrowedPathNilCheck(pass, state, ssaFn, head, path, expr.Op, block)
}

// resolvePathHeadAtCall returns the call-site head expression and
// the unconsumed path for a postfix-chained reference whose head
// names a parameter or the receiver. The head binds the same way
// resolveReferenceAtCall would on a path-free reference, and the
// path is returned untouched so the narrowing helper consumes it
// through the SSA / field-nil layers.
func resolvePathHeadAtCall(call *ast.CallExpr, ref dsl.Reference, paramNames []string, recvName string) (ast.Expr, []dsl.PathStep, bool) {
	if len(ref.Path) == 0 || ref.Kind != dsl.RefIdent {
		return nil, nil, false
	}
	head, ok := bindHeadIdentAtCall(call, ref.Name, paramNames, recvName)
	if !ok {
		return nil, nil, false
	}
	return head, ref.Path, true
}

// resolveReferenceAtCall binds ref to the argument or receiver
// expression it names at call. The receiver name wins when the
// rule carries the same identifier as the callee's declared
// receiver; regular parameters resolve through the index-aligned
// argument list. Postfix-chained references, `$N` positional
// references, and identifiers that do not match any slot report
// ok=false so the literal evaluator defers.
func resolveReferenceAtCall(call *ast.CallExpr, ref dsl.Reference, paramNames []string, recvName string) (ast.Expr, bool) {
	if len(ref.Path) > 0 {
		return nil, false
	}
	if ref.Kind != dsl.RefIdent {
		return nil, false
	}
	return bindHeadIdentAtCall(call, ref.Name, paramNames, recvName)
}

// bindHeadIdentAtCall returns the call-site expression that binds
// name to the call's receiver or one of its arguments. The
// receiver wins when name matches the callee's declared receiver;
// otherwise the helper scans paramNames for an index-aligned
// match. Names that bind to no signature slot, or argument lists
// shorter than the matched index, report ok=false.
func bindHeadIdentAtCall(call *ast.CallExpr, name string, paramNames []string, recvName string) (ast.Expr, bool) {
	if name == "" {
		return nil, false
	}
	if recvName != "" && name == recvName {
		if recv := callReceiverExpr(call); recv != nil {
			return recv, true
		}
	}
	for i, paramName := range paramNames {
		if paramName == "" || paramName != name {
			continue
		}
		if i >= len(call.Args) {
			return nil, false
		}
		return call.Args[i], true
	}
	return nil, false
}

// callReceiverExpr returns the receiver expression for a method
// call. Free-function calls and other non-selector callees return
// nil because they carry no receiver. Package-qualified calls
// (`pkg.Func()`) are not distinguished here; the higher-level
// dispatcher gates the receiver branch with sig.Recv() so a
// callee with no receiver never reaches this helper through the
// receiver path.
func callReceiverExpr(call *ast.CallExpr) ast.Expr {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	return sel.X
}

// logicalRulesForCallee returns the logical-arrow rules declared
// on callee. Same-package callees consult the cached
// rules-by-object set computed lazily by passState; cross-package
// callees consult the conditionLogicalFact exported by the
// declaring package. Returning nil signals the callee carries no
// rule the literal evaluator can apply.
func logicalRulesForCallee(pass *analysis.Pass, state *passState, callee types.Object) []*dsl.ArrowRule {
	if rules, ok := state.logicalRulesSet(pass)[callee]; ok {
		return rules
	}
	var fact conditionLogicalFact
	if !pass.ImportObjectFact(callee, &fact) {
		return nil
	}
	return materialiseFactRules(fact.Rules)
}

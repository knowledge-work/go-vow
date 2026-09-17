package analysis

import (
	"go/ast"
	"go/token"
	"strconv"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// checkLogicalRuleReturns evaluates the logical-arrow rules
// (`=>`, `<=>`) carried by every function's vow:cond payload
// against the function's return statements. Each Reference operand
// binds to the return-statement slot it names — named returns by
// identifier, `$N` positional references by index — and the
// evaluator decides the operand's boolean outcome from the literal
// shape of the bound expression. References to parameters,
// receivers, and postfix chains stay undecided because the return
// site carries no information about them without flow analysis;
// the caller-side pass and the SSA-driven refinement cover those
// shapes through dedicated pipelines.
//
// A diagnostic fires only when both operands of a rule resolve to
// known literal nil or known non-nil values at the same return
// statement and the boolean combination contradicts the rule's
// direction. The `=>` rule demands lhs implies rhs (a violation
// is lhs holds but rhs does not); the `<=>` rule demands lhs is
// equivalent to rhs (a violation is one side holds and the other
// does not).
func checkLogicalRuleReturns(pass *analysis.Pass, state *passState) {
	ssaFns := ssaFunctions(pass)
	for _, file := range pass.Files {
		scope := state.ruleScope(pass)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			conds, ok := state.annotation(fn, scope)
			if !ok {
				continue
			}
			rules := collectLogicalRules(conds)
			if len(rules) == 0 {
				continue
			}
			ssaFn := findSSAFunction(ssaFns, fn)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				ret, isRet := n.(*ast.ReturnStmt)
				if !isRet {
					return true
				}
				ssaRet := findSSAReturnFor(ssaFn, ret.Pos())
				checkLogicalRulesAtReturn(pass, state, fn, ssaFn, ssaRet, ret, rules)
				return true
			})
		}
	}
}

// collectLogicalRules returns every DirImply / DirEquiv arrow-rule
// across conds in source order. Subject-scoped conditions are
// skipped because their executable semantics belong to the
// higher-order integration scope that wires the contract against
// the named callback rather than the enclosing function.
func collectLogicalRules(conds []*dsl.Condition) []*dsl.ArrowRule {
	var rules []*dsl.ArrowRule
	for _, cond := range conds {
		if cond == nil || len(cond.Subject) > 0 {
			continue
		}
		for _, r := range cond.Rules {
			if r == nil || r.Kind != dsl.RuleArrow || r.Arrow == nil {
				continue
			}
			if r.Arrow.Direction == dsl.DirStructural {
				continue
			}
			rules = append(rules, r.Arrow)
		}
	}
	return rules
}

// checkLogicalRulesAtReturn evaluates each rule against ret and
// emits a diagnostic when both operands decide and the boolean
// combination contradicts the rule's direction. The walk groups
// each author-written rule with its derived contrapositive so a
// site where the original commits never re-reports through the
// contrapositive; the contrapositive only fires when the original
// could not commit. Transitive chain closures take their turn
// last, ordered by their seed origin so a forward chain (seeded
// from an author-written rule) takes precedence over a
// contrapositive-rotated chain when both could commit at this
// site — the forward derivation reads as the natural extension of
// the author's reasoning and produces the more legible
// diagnostic. Rules whose operands stay undecided at this return
// defer silently — operand shapes outside the literal-only and
// SSA-narrowed reach stay unreported here.
func checkLogicalRulesAtReturn(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, ssaRet *ssa.Return, ret *ast.ReturnStmt, rules []*dsl.ArrowRule) {
	block := returnBlock(ssaRet)
	for _, group := range groupRulesWithContrapositives(rules) {
		if evaluateLogicalRuleAtReturn(pass, state, fn, ssaFn, block, ret, group.Original, nil, "") {
			continue
		}
		if group.Contrapositive != nil {
			if evaluateLogicalRuleAtReturn(pass, state, fn, ssaFn, block, ret, group.Contrapositive, group.Original, "") {
				continue
			}
		}
		for _, t := range transitivesByForwardFirst(group.Transitives) {
			if evaluateLogicalRuleAtReturn(pass, state, fn, ssaFn, block, ret, t.Rule, nil, "transitive closure of "+t.ChainDesc) {
				break
			}
		}
	}
}

// transitivesByForwardFirst returns the transitive entries in a
// stable order that places every forward-derivation entry ahead
// of every contrapositive-rotated entry. The natural construction
// order through `groupRulesWithContrapositives` seeds the pool
// author-first, so the helper preserves that order through a
// stable partition rather than re-sorting; the explicit pass
// keeps the forward-first precedence visible at the call site
// rather than implicit in the construction order. The caller
// breaks on the first committing entry, so a forward-derivation
// chain wins over a contrapositive-rotated chain whenever both
// could commit at the same site.
func transitivesByForwardFirst(entries []transitiveEntry) []transitiveEntry {
	if len(entries) == 0 {
		return entries
	}
	var forward, contrapositive []transitiveEntry
	for _, t := range entries {
		if t.Kind == transitiveEntryForward {
			forward = append(forward, t)
			continue
		}
		contrapositive = append(contrapositive, t)
	}
	if len(contrapositive) == 0 {
		return forward
	}
	if len(forward) == 0 {
		return contrapositive
	}
	return append(forward, contrapositive...)
}

// evaluateLogicalRuleAtReturn drives one rule against ret and
// returns true when the evaluator committed at this site (either
// holding or violating with a reported diagnostic). The caller
// uses the return flag to skip the derived contrapositive and any
// chain closure when the original already decided so the
// original-first walk keeps the one-diagnostic-per-offending-claim
// invariant intact. The origin argument names the source rule
// when rule itself is a derived contrapositive so the diagnostic
// renders the source rule with a `contrapositive of` prefix. The
// descOverride argument, when non-empty, replaces the rule
// rendering entirely so chain-closure diagnostics announce the
// full chain through renderChain rather than the synthetic
// derived rule's surface form.
func evaluateLogicalRuleAtReturn(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, block *ssa.BasicBlock, ret *ast.ReturnStmt, rule *dsl.ArrowRule, origin *dsl.ArrowRule, descOverride string) bool {
	if rule == nil || rule.Left == nil || rule.Right == nil {
		return false
	}
	lhs, lOk := evaluateOperandAtReturn(pass, state, fn, ssaFn, block, ret, rule.Left)
	if !lOk {
		return false
	}
	rhs, rOk := evaluateOperandAtReturn(pass, state, fn, ssaFn, block, ret, rule.Right)
	if !rOk {
		return false
	}
	if !logicalRuleHolds(lhs, rhs, rule.Direction) {
		desc := descOverride
		if desc == "" {
			desc = describeLogicalRule(rule, origin)
		}
		vowReportf(
			pass,
			ret.Pos(),
			"vow[sentinel-error]: return violates %s: %s",
			desc,
			logicalViolationReason(lhs, rhs, rule, "return"),
		)
	}
	return true
}

// returnBlock returns the SSA basic block that hosts ssaRet, or nil
// when the lookup failed. The narrowing helpers tolerate a nil
// block by reporting undecided, so the SSA refinement degrades
// gracefully when the buildssa pass left the function untracked.
func returnBlock(ssaRet *ssa.Return) *ssa.BasicBlock {
	if ssaRet == nil {
		return nil
	}
	return ssaRet.Block()
}

// logicalRuleHolds reports whether the boolean combination
// satisfies the arrow's direction. DirImply demands lhs implies
// rhs; DirEquiv demands lhs equals rhs.
func logicalRuleHolds(lhs, rhs bool, dir dsl.Direction) bool {
	switch dir {
	case dsl.DirImply:
		return !lhs || rhs
	case dsl.DirEquiv:
		return lhs == rhs
	}
	return true
}

// logicalViolationReason renders the human-readable reason that
// accompanies a contradiction diagnostic. The reason names which
// side holds and which side fails so the author sees the asymmetry
// without re-deriving it from the rule text. The site argument
// names where the evaluation happened ("return" / "call") so the
// reader recognises whether the contradiction surfaced at a
// callee-side return statement or at a caller-side call site.
func logicalViolationReason(lhs, rhs bool, rule *dsl.ArrowRule, site string) string {
	left := rule.Left.String()
	right := rule.Right.String()
	switch rule.Direction {
	case dsl.DirImply:
		return left + " holds at this " + site + " but " + right + " does not"
	case dsl.DirEquiv:
		if lhs {
			return left + " holds at this " + site + " but " + right + " does not"
		}
		return right + " holds at this " + site + " but " + left + " does not"
	}
	return ""
}

// evaluateOperandAtReturn decides the boolean outcome of expr at
// ret. The triple return is (held bool, decided bool): decided is
// false when neither the literal-only evaluator nor the SSA
// refinement reached a commitment, in which case the caller drops
// the rule for this return. Expression shapes the evaluator
// recognises :
//
//   - ExprNilCheck whose Reference binds to a return slot the
//     literal-only evaluator reads as nil or as a syntactically
//     non-nil shape (composite literal, address-of, function
//     literal). The SSA refinement extends the reach to operands
//     bound through alias chains, `vow:nil` parameter or field
//     decls, and identifiers narrowed by a dominating if-guard.
//   - ExprComparison whose Reference binds to a return slot the
//     literal-only evaluator reads as a basic literal value
//     (integer, string, or bool) compatible with the rule's
//     literal right-hand side. Equality and inequality decide for
//     every supported kind; ordered comparisons only commit on
//     integers.
//
// Operands neither path reaches fall through with decided=false.
func evaluateOperandAtReturn(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, block *ssa.BasicBlock, ret *ast.ReturnStmt, expr *dsl.Expression) (bool, bool) {
	if expr == nil {
		return false, false
	}
	if bound, ok := resolveReferenceAtReturn(fn, ret, expr.Ref); ok {
		switch expr.Kind {
		case dsl.ExprNilCheck:
			if held, decided := evaluateNilCheck(pass, bound, expr.Op); decided {
				return held, true
			}
			return ssaNarrowedNilCheck(pass, state, fn, ssaFn, bound, expr.Op, block)
		case dsl.ExprComparison:
			if held, decided := evaluateComparisonMembers(pass, bound, expr.Op, expr.Members); decided {
				return held, true
			}
			return ssaNarrowedComparisonMembers(pass, state, fn, ssaFn, bound, expr.Op, expr.Members, block)
		}
		return false, false
	}
	// Operands that bind to a parameter or receiver carry no
	// return-statement expression; the SSA refinement narrows them
	// through the enclosing if-guard chain instead. The nil-check
	// shape resolves through the nilness refinement and the
	// comparison shape resolves through the integer range
	// refinement; both rely on the signature scope to supply a
	// parameter SSA value the dominator walk tracks. Path-bearing
	// references (`recv.Field[idx]`) route through the path
	// narrowing helper because the SSA value the dominator walk
	// reads sits one or more Load / FieldAddr / IndexAddr steps
	// past the parameter.
	if expr.Ref.Kind != dsl.RefIdent {
		return false, false
	}
	ident, ok := signatureIdent(fn, expr.Ref.Name)
	if !ok {
		return false, false
	}
	if len(expr.Ref.Path) > 0 {
		if expr.Kind != dsl.ExprNilCheck {
			return false, false
		}
		return ssaNarrowedPathNilCheck(pass, state, ssaFn, ident, expr.Ref.Path, expr.Op, block)
	}
	switch expr.Kind {
	case dsl.ExprNilCheck:
		return ssaNarrowedNilCheck(pass, state, fn, ssaFn, ident, expr.Op, block)
	case dsl.ExprComparison:
		return ssaNarrowedComparisonMembers(pass, state, fn, ssaFn, ident, expr.Op, expr.Members, block)
	}
	return false, false
}

// ssaNarrowedComparisonMembers applies the SSA-narrowing
// evaluator to each member of a comparison sum and combines the
// per-member decisions the same way evaluateComparisonMembers
// does. A nil member routes through ssaNarrowedNilCheck and a
// non-nil member routes through ssaNarrowedComparison. The helper
// stays uncommitted when no member commits, so the caller falls
// through to the next refinement tier.
func ssaNarrowedComparisonMembers(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, ssaFn *ssa.Function, expr ast.Expr, op dsl.CompareOp, members []dsl.ExprMember, block *ssa.BasicBlock) (bool, bool) {
	if len(members) == 0 {
		return false, false
	}
	if op != dsl.OpEq && op != dsl.OpNeq {
		if len(members) != 1 || members[0].Nil {
			return false, false
		}
		return ssaNarrowedComparison(pass, ssaFn, expr, op, members[0].Literal, block)
	}
	matched := false
	allDecided := true
	for _, m := range members {
		var held, decided bool
		if m.Nil {
			held, decided = ssaNarrowedNilCheck(pass, state, fn, ssaFn, expr, dsl.OpEq, block)
		} else {
			held, decided = ssaNarrowedComparison(pass, ssaFn, expr, dsl.OpEq, m.Literal, block)
		}
		if !decided {
			allDecided = false
			continue
		}
		if held {
			matched = true
			break
		}
	}
	if matched {
		return op == dsl.OpEq, true
	}
	if allDecided {
		return op == dsl.OpNeq, true
	}
	return false, false
}

// signatureIdent returns the declaration identifier for the named
// regular parameter or receiver of fn. The walk inspects fn.Recv
// first so a method's receiver wins over a parameter that
// shadowed it would, then fn.Type.Params in source order. The
// returned identifier shares the same types.Object the SSA pass
// resolves the parameter through, so a downstream narrowing path
// reads the SSA parameter without an extra lookup.
func signatureIdent(fn *ast.FuncDecl, name string) (*ast.Ident, bool) {
	if fn == nil || fn.Type == nil || name == "" {
		return nil, false
	}
	if fn.Recv != nil {
		for _, field := range fn.Recv.List {
			for _, ident := range field.Names {
				if ident.Name == name {
					return ident, true
				}
			}
		}
	}
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			for _, ident := range field.Names {
				if ident.Name == name {
					return ident, true
				}
			}
		}
	}
	return nil, false
}

// resolveReferenceAtReturn binds ref to the return-statement
// expression it names. Named returns match by identifier name;
// `$N` references parse N as a 1-indexed position. References to
// parameters or receivers, or chained references (`.field`,
// `[idx]`), return ok=false so the literal evaluator defers to a
// here.
func resolveReferenceAtReturn(fn *ast.FuncDecl, ret *ast.ReturnStmt, ref dsl.Reference) (ast.Expr, bool) {
	if len(ref.Path) > 0 {
		return nil, false
	}
	if fn == nil || fn.Type == nil || fn.Type.Results == nil {
		return nil, false
	}
	index, ok := returnIndexForReference(fn, ref)
	if !ok {
		return nil, false
	}
	if index < 0 || index >= len(ret.Results) {
		return nil, false
	}
	return ret.Results[index], true
}

// returnIndexForReference reports the index in fn's return list
// the reference names. The `$N` form parses N as a 1-indexed
// position; the named form scans the return list for a matching
// identifier, expanding grouped declarations (`(a, b T)`) so the
// reported index is the slot the author would point at.
func returnIndexForReference(fn *ast.FuncDecl, ref dsl.Reference) (int, bool) {
	switch ref.Kind {
	case dsl.RefDollar:
		n, err := strconv.Atoi(ref.Name)
		if err != nil || n < 1 {
			return 0, false
		}
		return n - 1, true
	case dsl.RefIdent:
		index := 0
		for _, field := range fn.Type.Results.List {
			if len(field.Names) == 0 {
				index++
				continue
			}
			for _, name := range field.Names {
				if name.Name == ref.Name {
					return index, true
				}
				index++
			}
		}
		return 0, false
	}
	return 0, false
}

// evaluateNilCheck applies a nil-comparison operator against the
// bound return expression. The decision is literal-only: the bare
// `nil` identifier and the typed-nil conversion `(*T)(nil)` decide
// the comparison as nil-side; composite literals, address-of
// operators on any expression, and function literals decide the
// comparison as non-nil-side. Other shapes (function calls,
// variables, type assertions) leave the evaluator with too little
// information to commit and report decided=false.
func evaluateNilCheck(pass *analysis.Pass, expr ast.Expr, op dsl.CompareOp) (bool, bool) {
	if op != dsl.OpEq && op != dsl.OpNeq {
		return false, false
	}
	if argumentIsStaticallyNil(pass, expr) {
		return op == dsl.OpEq, true
	}
	if isStaticallyNonNil(expr) {
		return op == dsl.OpNeq, true
	}
	return false, false
}

// isStaticallyNonNil reports whether expr proves a non-nil shape
// at the syntactic level. Composite literals (`Foo{}`, `&Foo{}`),
// address-of operators on any expression (`&x`), and function
// literals all yield values whose nil-ness is decided by syntax
// alone. Other shapes fall through unreported so the evaluator
// stays sound.
func isStaticallyNonNil(expr ast.Expr) bool {
	expr = ast.Unparen(expr)
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return true
	case *ast.UnaryExpr:
		return e.Op == token.AND
	case *ast.FuncLit:
		return true
	}
	return false
}

// evaluateComparisonMembers applies the comparison operator
// against a sum of right-hand-side members and combines the per-
// member decisions according to the operator's distribution.
// Equality distributes as OR (any matching member proves the
// rule held); inequality distributes as AND by De Morgan (every
// member must differ for the rule to hold). A nil member routes
// through evaluateNilCheck so a mixed `nil | 0` sum stays a
// single dispatch. Ordered operators reject a multi-member RHS at
// parse time; a single-member ordered comparison routes through
// the literal path unchanged.
func evaluateComparisonMembers(pass *analysis.Pass, expr ast.Expr, op dsl.CompareOp, members []dsl.ExprMember) (bool, bool) {
	if len(members) == 0 {
		return false, false
	}
	if op != dsl.OpEq && op != dsl.OpNeq {
		if len(members) != 1 || members[0].Nil {
			return false, false
		}
		return evaluateComparison(expr, op, members[0].Literal)
	}
	matched := false
	allDecided := true
	for _, m := range members {
		var held, decided bool
		if m.Nil {
			held, decided = evaluateNilCheck(pass, expr, dsl.OpEq)
		} else {
			held, decided = evaluateComparison(expr, dsl.OpEq, m.Literal)
		}
		if !decided {
			allDecided = false
			continue
		}
		if held {
			matched = true
			break
		}
	}
	if matched {
		return op == dsl.OpEq, true
	}
	if allDecided {
		return op == dsl.OpNeq, true
	}
	return false, false
}

// evaluateComparison applies an equality, inequality, or ordered
// comparison against the bound return expression and the rule's
// literal right-hand side. The decision is literal-only: both
// operands must reduce to compatible basic literals (integer,
// string, or bool). Equality and inequality decide for every
// supported kind; ordered comparisons only commit on integers
// because string and bool ordering carries no useful semantic at
// the surface the grammar exposes. Shapes the literal
// extractor cannot reach report decided=false.
func evaluateComparison(expr ast.Expr, op dsl.CompareOp, spelling string) (bool, bool) {
	bound, ok := basicLiteralValue(expr)
	if !ok {
		return false, false
	}
	rule, ok := classifyComparisonLiteral(spelling)
	if !ok {
		return false, false
	}
	return compareLiterals(bound, rule, op)
}

// litKind enumerates the basic-literal shapes the evaluator
// compares at the return site. The kind discriminator selects
// which slot of literalValue carries the parsed payload.
type litKind int

const (
	litUnknown litKind = iota
	litInt
	litString
	litBool
)

// literalValue carries a basic-literal value extracted from a
// return expression or from a vow:cond comparison's right-hand
// side. Exactly one of the typed slots carries the payload; kind
// names which.
type literalValue struct {
	kind    litKind
	intVal  int64
	strVal  string
	boolVal bool
}

// basicLiteralValue extracts the literal value carried by expr,
// or reports ok=false when the expression does not reduce to a
// basic literal. Parenthesised expressions are unwrapped so
// `("live")` reads the same as `"live"`; signed integer literals
// (`-3`, `+0xff`) parse through the unary-operator branch so
// authors who write a negative constant return value see their
// rule evaluated as expected. Other shapes (function calls,
// identifiers, composite literals, type assertions) report
// ok=false because their value is not decidable without flow
// analysis.
func basicLiteralValue(expr ast.Expr) (literalValue, bool) {
	expr = ast.Unparen(expr)
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			n, err := strconv.ParseInt(e.Value, 0, 64)
			if err != nil {
				return literalValue{}, false
			}
			return literalValue{kind: litInt, intVal: n}, true
		case token.STRING:
			s, err := strconv.Unquote(e.Value)
			if err != nil {
				return literalValue{}, false
			}
			return literalValue{kind: litString, strVal: s}, true
		}
	case *ast.Ident:
		switch e.Name {
		case "true":
			return literalValue{kind: litBool, boolVal: true}, true
		case "false":
			return literalValue{kind: litBool, boolVal: false}, true
		}
	case *ast.UnaryExpr:
		if e.Op != token.SUB && e.Op != token.ADD {
			return literalValue{}, false
		}
		lit, ok := e.X.(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			return literalValue{}, false
		}
		n, err := strconv.ParseInt(lit.Value, 0, 64)
		if err != nil {
			return literalValue{}, false
		}
		if e.Op == token.SUB {
			n = -n
		}
		return literalValue{kind: litInt, intVal: n}, true
	}
	return literalValue{}, false
}

// classifyComparisonLiteral parses the rule's literal RHS spelling
// into a typed value. The spelling mirrors the lexer's token
// values: a quoted string round-trips through strconv.Unquote, the
// bool keywords parse as bool, and signed decimal or based
// integer literals parse through strconv.ParseInt. Spellings the
// classifier cannot place (identifiers, malformed numerics) report
// ok=false so the evaluator stays sound.
func classifyComparisonLiteral(spelling string) (literalValue, bool) {
	switch spelling {
	case "true":
		return literalValue{kind: litBool, boolVal: true}, true
	case "false":
		return literalValue{kind: litBool, boolVal: false}, true
	}
	if len(spelling) >= 2 {
		first := spelling[0]
		if first == '"' || first == '`' || first == '\'' {
			s, err := strconv.Unquote(spelling)
			if err != nil {
				return literalValue{}, false
			}
			return literalValue{kind: litString, strVal: s}, true
		}
	}
	n, err := strconv.ParseInt(spelling, 0, 64)
	if err == nil {
		return literalValue{kind: litInt, intVal: n}, true
	}
	return literalValue{}, false
}

// compareLiterals applies op to a pair of literal values whose
// kinds agree. Equality and inequality compare every supported
// kind directly; ordered comparisons only commit on integers
// because the grammar's ordered operators carry no useful semantic
// for strings or bools at the surface. A mismatched-kind
// pair or an ordered comparison against a non-integer pair reports
// decided=false so the evaluator stays sound.
func compareLiterals(left, right literalValue, op dsl.CompareOp) (bool, bool) {
	if left.kind != right.kind || left.kind == litUnknown {
		return false, false
	}
	switch op {
	case dsl.OpEq, dsl.OpNeq:
		eq := false
		switch left.kind {
		case litInt:
			eq = left.intVal == right.intVal
		case litString:
			eq = left.strVal == right.strVal
		case litBool:
			eq = left.boolVal == right.boolVal
		}
		if op == dsl.OpEq {
			return eq, true
		}
		return !eq, true
	case dsl.OpLt, dsl.OpLte, dsl.OpGt, dsl.OpGte:
		if left.kind != litInt {
			return false, false
		}
		switch op {
		case dsl.OpLt:
			return left.intVal < right.intVal, true
		case dsl.OpLte:
			return left.intVal <= right.intVal, true
		case dsl.OpGt:
			return left.intVal > right.intVal, true
		case dsl.OpGte:
			return left.intVal >= right.intVal, true
		}
	}
	return false, false
}

package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// bindingSource distinguishes the fact origin of a caller-side
// dead-guard binding. Both origins feed the same downstream
// narrow-state reasoning, so only the fact-collection phase
// differs; the report and invalidation paths dispatch on this
// field to keep the diagnostic quoting each source's contract in
// its own terms.
type bindingSource int

const (
	// bindingSourceNil marks a binding derived from a `vow:nil`
	// signature-mirror return declaration (`!` on the return
	// slot). Facts of this source narrow unconditionally at the
	// call site — no paired guard is required for the pending →
	// narrowed transition, so they enter the narrowed map
	// directly.
	bindingSourceNil bindingSource = iota

	// bindingSourceCond marks a binding derived from a `vow:cond`
	// logical-arrow rule (`<lhs> == nil <arrow> <ref> != nil` or
	// its biconditional swap). Facts of this source narrow
	// conditionally: the binding enters the pending map at the
	// call site and only becomes narrowed once a paired guard
	// short-circuits the counter-branch (or the target itself is
	// directly guarded).
	bindingSourceCond
)

// narrowState describes the equality relation the target
// satisfies once the binding is narrowed. The operator is one
// of dsl.OpEq (`==`) or dsl.OpNeq (`!=`); the value is either
// nil (when isNil is true) or the source spelling of a
// non-nil literal (`"5"`, `"\"login\""`, `"true"`). Every
// downstream dead-guard judgement reads this pair so an inner
// comparison is dead when the narrow's outcome makes the
// comparison trivially true (redundant) or trivially false
// (impossible), and stays silent when the two together do not
// decide the branch (undecided).
//
// The nil case is a special value of this axis, not a separate
// type: nil-check facts land here with isNil=true and every
// non-nil literal shares the same shape. Stack 3 introduced the
// generalisation; the earlier "narrowToNonNil / narrowToNil"
// enum is now the two nil-shaped subsets (narrowState{Neq, nil}
// and narrowState{Eq, nil} respectively).
type narrowState struct {
	// op is OpEq (`==`) or OpNeq (`!=`) — the equality relation
	// the target satisfies against value.
	op dsl.CompareOp
	// value is the literal the target is compared against. Only
	// equality-shaped narrow states are represented today;
	// ordered narrow (< / <= / > / >=) is a follow-up scope.
	value narrowValue
}

// narrowValue holds the literal a narrowState compares against.
// A nil-check narrow sets isNil to true and leaves literal
// empty; a literal narrow sets isNil to false and carries the
// source spelling in literal so equality checks against another
// literal fold into a single string comparison.
type narrowValue struct {
	isNil   bool
	literal string
}

// nilNarrowValue returns the narrowValue that marks a nil-check
// narrow. Every earlier nil-shaped path routes through this
// constructor so the two axes (nil vs non-nil literal) stay
// distinguishable by the isNil flag.
func nilNarrowValue() narrowValue {
	return narrowValue{isNil: true}
}

// equalNarrowValues reports whether two narrowValues refer to
// the same literal — the nil marker matches nil, and two
// non-nil literals match when their source spellings agree.
// Type discrimination is deferred to the source parser; the
// spellings are what appear in the vow:cond rule and the
// caller's guard, so a lexical match here is a semantic match.
func equalNarrowValues(a, b narrowValue) bool {
	if a.isNil != b.isNil {
		return false
	}
	if a.isNil {
		return true
	}
	return a.literal == b.literal
}

// callerBinding records a local identifier bound to a call
// return slot whose narrow state has been established (nil
// source: unconditionally by the callee's `vow:nil !` return
// declaration; cond source: conditionally by a `vow:cond` rule
// once the paired guard fires). The report path quotes the
// callee and — for cond bindings — the rule spelling so a reader
// who edits the callee's contract can find the guard that
// assumed it.
type callerBinding struct {
	calleeName string
	returnPos  int // 1-based, for diagnostic quoting.
	source     bindingSource
	// narrow is the equality relation the target satisfies once
	// the binding is narrowed. `vow:nil !` returns yield the
	// non-nil nil-shaped state; a `vow:cond` rule yields either
	// a nil-shaped or a literal-shaped narrow depending on the
	// rule's RHS.
	narrow narrowState
	// guardNarrow is the equality relation the paired guard
	// proves on the paired local when the guard fires. For
	// nil-source bindings the guard is unconditional so this
	// field is unused; for cond-source bindings it captures the
	// rule's LHS shape so the promote and body-narrow paths can
	// decide when a caller-side guard implies the antecedent.
	guardNarrow narrowState
	// ruleSrc holds the rule's source-form spelling for a cond
	// binding; empty for nil bindings.
	ruleSrc string
	// guardName holds the paired local whose comparison the
	// rule LHS reads for a cond binding; empty for nil bindings.
	guardName string
}

// validateCallerDeadGuard walks every block in pass.Files and
// reports a nil guard standing on a local whose non-nilness a
// preceding fact — either a `vow:nil !` return declaration on
// the callee or a `vow:cond` rule whose paired guard has
// already short-circuited — already proves. The pass unifies the
// two caller-side dead-guard surfaces that used to live in
// separate files (`vow:nil` and `vow:cond`): the two share the
// same state machine, invalidation rules, and diagnostic
// anchor, so the merged pass carries a single walk with a
// source-dispatched emit.
//
// The pass stays AST-only. Nested if-body blocks inherit the
// outer block's pending / narrowed state through a body-narrow
// carry so a rule can prove the target's nil-state inside the
// guard's body; other nested block kinds (for / switch / range
// bodies, and function literals) still scan independently with
// a fresh state because their control flow does not carry the
// outer guard's narrow. Every leave-alone shape —
// reassignment, address-of, function literals, init-clause
// guards, non-short-circuit guard bodies — matches the
// syntactic-pattern stance the earlier split surfaces
// documented so authors read one set of rules across both
// origins.
func validateCallerDeadGuard(pass *analysis.Pass, state *passState) {
	for _, file := range pass.Files {
		scanned := map[*ast.BlockStmt]struct{}{}
		ast.Inspect(file, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			if _, done := scanned[block]; done {
				return true
			}
			scanned[block] = struct{}{}
			checkBlockCallerDeadGuards(pass, state, block, nil, nil, nil, scanned)
			return true
		})
	}
}

// checkBlockCallerDeadGuards walks block's statement list once,
// carrying two source-neutral maps keyed on the bound
// identifier: pending (a cond-source binding awaits its paired
// guard) and narrowed (either origin's narrow has been proven).
// The inherited maps let a nested if-body scan begin with the
// outer block's narrow context; a top-level call passes nil for
// each. Each statement runs through six ordered steps:
//
//  1. Report dead guards against already-narrowed bindings.
//  2. Drop every binding when a function literal appears — a
//     closure can rewrite captured locals out of syntactic view.
//  3. Invalidate bindings the statement rewrites (assign LHS,
//     inc/dec, address-of), with a guardName cascade so a
//     reassigned local also drops any cond binding that named it
//     as its paired guard.
//  4. Promote pending cond bindings when this statement is a
//     recognised short-circuit guard shape.
//  5. Record new bindings — cond bindings enter pending, nil
//     bindings enter narrowed directly.
//  6. Descend into the statement's if-body (and else-body, if
//     any) with a body-narrow carry — the outer guard proves
//     one of its two branches at entry to that block, and any
//     pending binding whose paired guard matches gets promoted
//     for the inner scan. The scanned set stops the outer walk
//     from re-scanning the same block as an independent scan.
func checkBlockCallerDeadGuards(
	pass *analysis.Pass,
	state *passState,
	block *ast.BlockStmt,
	inheritedPending map[string]callerBinding,
	inheritedNarrowed map[string]callerBinding,
	inheritedNarrowedNames map[string]struct{},
	scanned map[*ast.BlockStmt]struct{},
) {
	if block == nil {
		return
	}
	pending := cloneCallerBindings(inheritedPending)
	narrowed := cloneCallerBindings(inheritedNarrowed)
	narrowedNames := cloneNameSet(inheritedNarrowedNames)
	for _, stmt := range block.List {
		if ifs, ok := stmt.(*ast.IfStmt); ok {
			reportCallerDeadGuard(pass, ifs, narrowed, narrowedNames)
		}
		if containsFuncLit(stmt) {
			pending = map[string]callerBinding{}
			narrowed = map[string]callerBinding{}
			narrowedNames = map[string]struct{}{}
			continue
		}
		invalidateCallerBindings(stmt, pending, narrowed, narrowedNames)
		promoteCallerBindingsOnGuard(stmt, pending, narrowed, narrowedNames)
		recordCallerBindings(pass, state, stmt, pending, narrowed, narrowedNames)
		descendIntoIfBody(pass, state, stmt, pending, narrowed, narrowedNames, scanned)
	}
}

// descendIntoIfBody scans the if-body (and else-body, when
// present) of an if statement with the outer block's narrow
// context extended by the guard's body-narrow. The outer
// guard's shape decides which pending bindings promote into the
// body — an `if x == nil { <body> }` proves `x == nil` inside
// the body, an `if x != nil { <body> }` proves `x != nil`, and
// the rule direction on each pending binding whose guardName
// matches determines whether that narrow lands on the binding's
// target as non-nil or nil. The scanned set is threaded through
// the recursion so the outer file walk does not re-scan an
// inner block as an independent scan.
func descendIntoIfBody(
	pass *analysis.Pass,
	state *passState,
	stmt ast.Stmt,
	pending map[string]callerBinding,
	narrowed map[string]callerBinding,
	narrowedNames map[string]struct{},
	scanned map[*ast.BlockStmt]struct{},
) {
	ifs, ok := stmt.(*ast.IfStmt)
	if !ok || ifs.Init != nil || ifs.Body == nil {
		return
	}
	bodyGuardName, bodyGuardState, guardKnown := recognisedGuardShape(ifs.Cond)
	bodyPending, bodyNarrowed, bodyNarrowedNames := cloneCallerBindings(pending), cloneCallerBindings(narrowed), cloneNameSet(narrowedNames)
	if guardKnown {
		applyBodyNarrow(bodyGuardName, bodyGuardState, bodyPending, bodyNarrowed, bodyNarrowedNames)
	}
	scanned[ifs.Body] = struct{}{}
	checkBlockCallerDeadGuards(pass, state, ifs.Body, bodyPending, bodyNarrowed, bodyNarrowedNames, scanned)
	// The else branch runs under the opposite narrow. Only descend
	// when the else is a bare block; an `else if` chain leaves
	// the else-scan to the outer walk on that IfStmt's own body.
	elseBlock, ok := ifs.Else.(*ast.BlockStmt)
	if !ok || elseBlock == nil {
		return
	}
	elsePending, elseNarrowed, elseNarrowedNames := cloneCallerBindings(pending), cloneCallerBindings(narrowed), cloneNameSet(narrowedNames)
	if guardKnown {
		applyBodyNarrow(bodyGuardName, oppositeGuardState(bodyGuardState), elsePending, elseNarrowed, elseNarrowedNames)
	}
	scanned[elseBlock] = struct{}{}
	checkBlockCallerDeadGuards(pass, state, elseBlock, elsePending, elseNarrowed, elseNarrowedNames, scanned)
}

// recognisedGuardShape reports the guarded identifier and the
// narrow state (operator + literal) the body of an if-stmt
// proves when cond is a bare-identifier equality check
// (`x == V` or `x != V` where V is nil or a basic literal),
// so the descent can extend the inner block's narrow with the
// outer guard's outcome.
func recognisedGuardShape(cond ast.Expr) (string, narrowState, bool) {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return "", narrowState{}, false
	}
	var op dsl.CompareOp
	switch bin.Op.String() {
	case "==":
		op = dsl.OpEq
	case "!=":
		op = dsl.OpNeq
	default:
		return "", narrowState{}, false
	}
	if name, val, ok := identLiteralOperands(bin.X, bin.Y); ok {
		return name, narrowState{op: op, value: val}, true
	}
	if name, val, ok := identLiteralOperands(bin.Y, bin.X); ok {
		return name, narrowState{op: op, value: val}, true
	}
	return "", narrowState{}, false
}

// identLiteralOperands returns the bare-identifier name from
// identSide and the narrow value from litSide when the two
// halves of a binary expression carry those shapes. Any other
// pairing (both idents, both literals, non-recognised literal)
// reports ok=false so callers keep the equality-comparison
// shape narrow.
func identLiteralOperands(identSide, litSide ast.Expr) (string, narrowValue, bool) {
	name, ok := nameIfBareIdent(identSide)
	if !ok {
		return "", narrowValue{}, false
	}
	val, ok := literalNarrowValue(litSide)
	if !ok {
		return "", narrowValue{}, false
	}
	return name, val, true
}

// literalNarrowValue reports the narrow-value form of expr
// when expr is a nil marker or a basic literal recogniser can
// compare against another literal by source spelling. The
// pre-declared identifiers `true` and `false` route through as
// literal-shaped values so an equality guard against a bool
// constant lands on the same narrow shape as an int or string
// literal does.
func literalNarrowValue(expr ast.Expr) (narrowValue, bool) {
	if isBareNilIdent(expr) {
		return nilNarrowValue(), true
	}
	switch v := expr.(type) {
	case *ast.BasicLit:
		return narrowValue{literal: v.Value}, true
	case *ast.Ident:
		if v.Name == "true" || v.Name == "false" {
			return narrowValue{literal: v.Name}, true
		}
	}
	return narrowValue{}, false
}

// oppositeGuardState returns the narrow state the counter-
// branch proves — the same value, but with the equality
// operator flipped.
func oppositeGuardState(state narrowState) narrowState {
	if state.op == dsl.OpEq {
		return narrowState{op: dsl.OpNeq, value: state.value}
	}
	return narrowState{op: dsl.OpEq, value: state.value}
}

// applyBodyNarrow promotes pending cond bindings whose paired
// guardName matches the outer guard's identifier and whose
// rule antecedent matches the guard's proven state on that
// identifier. Match is structural: the guard proves some
// (op, value) pair on guardName, the rule requires some
// (op, value) pair on the same slot, and they promote only
// when the two states agree exactly (`narrowStatesMatch`).
//
// The nil-shaped case is a special value of this axis, so the
// two nil-shaped combinations Stack 2 encoded (positive-form
// rule + `guardName == nil` proof, negative-form rule +
// `guardName != nil` proof) fold naturally into the same match
// check.
func applyBodyNarrow(
	guardName string,
	guardProof narrowState,
	pending map[string]callerBinding,
	narrowed map[string]callerBinding,
	narrowedNames map[string]struct{},
) {
	for bound, binding := range pending {
		if binding.source != bindingSourceCond {
			continue
		}
		if binding.guardName != guardName {
			continue
		}
		if !narrowStatesMatch(binding.guardNarrow, guardProof) {
			continue
		}
		narrowed[bound] = binding
		narrowedNames[bound] = struct{}{}
		delete(pending, bound)
	}
}

// narrowStatesMatch reports whether two narrow states describe
// the same equality relation. Both the operator and the value
// must agree; a mismatch on either axis stops a promotion.
func narrowStatesMatch(a, b narrowState) bool {
	return a.op == b.op && equalNarrowValues(a.value, b.value)
}

// cloneCallerBindings returns a fresh map containing the same
// entries as in. A nil input yields an empty map. Used to give
// nested-block scans their own state without mutating the
// caller's.
func cloneCallerBindings(in map[string]callerBinding) map[string]callerBinding {
	out := make(map[string]callerBinding, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// cloneNameSet returns a fresh set containing the same members
// as in. A nil input yields an empty set.
func cloneNameSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

// reportCallerDeadGuard emits the diagnostic when ifs carries
// an `if x <op> <literal> { return }` shape over a narrowed
// local and the narrow state combined with the guard shape
// makes the branch trivially true (redundant) or trivially
// false (impossible). The judgement is form-independent: any
// equality comparison on a narrowed local is decidable when
// the narrow's value is known (op Eq) or when the guard
// compares against the same value (op Neq); other shapes stay
// undecided and the diagnostic stays silent. The message
// dispatches on the binding's source (nil-source quotes the
// `!` return declaration; cond-source quotes the rule
// spelling and paired guard name).
func reportCallerDeadGuard(
	pass *analysis.Pass,
	ifs *ast.IfStmt,
	narrowed map[string]callerBinding,
	narrowedNames map[string]struct{},
) {
	if len(narrowedNames) == 0 || ifs.Init != nil {
		return
	}
	if !branchShortCircuits(ifs.Body) {
		return
	}
	name, guardState, ok := ifCondIsComparisonShape(ifs.Cond, narrowedNames)
	if !ok {
		return
	}
	binding := narrowed[name]
	outcome := deadGuardOutcome(binding.narrow, guardState)
	if outcome == "" {
		return
	}
	switch binding.source {
	case bindingSourceNil:
		vowReportNilSafetyf(
			pass,
			ifs.Pos(),
			"vow[nil-safety]: guard on %s is dead (%s); %s declared ! at return position %d of %s (the call cannot return nil)",
			name,
			outcome,
			nilDeclMarker,
			binding.returnPos,
			binding.calleeName,
		)
	case bindingSourceCond:
		vowReportNilSafetyf(
			pass,
			ifs.Pos(),
			"vow[nil-safety]: guard on %s is dead (%s); %s rule %q on %s narrows return position %d %s once the guard on %s short-circuits",
			name,
			outcome,
			rulesMarker,
			binding.ruleSrc,
			binding.calleeName,
			binding.returnPos,
			narrowSpelling(binding.narrow),
			binding.guardName,
		)
	}
}

// deadGuardOutcome renders the guard's relationship to the
// narrow state — `redundant` when the narrow already implies
// the guard's condition (branch fires unconditionally), or
// `impossible` when the narrow contradicts the guard's
// condition (branch never fires). An undecidable combination
// returns the empty string so the caller skips the diagnostic.
//
// The truth table:
//
//	narrow \ guard  Eq value-match   Eq value-mismatch   Neq value-match   Neq value-mismatch
//	Eq V1           redundant        impossible          impossible        redundant
//	Neq V1          impossible       undecided           redundant         undecided
//
// The Eq-narrow row is decidable everywhere: the narrow pins
// x to a single literal so any comparison against another
// literal is trivially true or false. The Neq-narrow row is
// decidable only when the guard compares against the same
// literal the narrow excluded: any other guard leaves x's
// value open.
func deadGuardOutcome(narrow, guard narrowState) string {
	match := equalNarrowValues(narrow.value, guard.value)
	if narrow.op == dsl.OpEq {
		if narrow.op == guard.op {
			if match {
				return "redundant"
			}
			return "impossible"
		}
		if match {
			return "impossible"
		}
		return "redundant"
	}
	if !match {
		return ""
	}
	if narrow.op == guard.op {
		return "redundant"
	}
	return "impossible"
}

// narrowSpelling renders the narrow state for the diagnostic —
// the message quotes the target's proven relation so a reader
// edits the callee's contract with the right shape in mind. A
// nil-shaped narrow keeps the terse `to nil` / `to non-nil`
// spelling Stack 2 used; a literal-shaped narrow spells the
// operator and value directly.
func narrowSpelling(state narrowState) string {
	if state.value.isNil {
		if state.op == dsl.OpEq {
			return "to nil"
		}
		return "to non-nil"
	}
	if state.op == dsl.OpEq {
		return "to " + state.value.literal
	}
	return "away from " + state.value.literal
}

// ifCondIsComparisonShape reports the identifier name and the
// equality-comparison narrow state (operator + literal) when
// cond is a bare-identifier equality check against a nil marker
// or a basic literal. Other shapes (negated, typed-nil
// conversion, compound expressions, ordered comparisons) return
// false — the ordered narrow lands in a follow-up scope.
func ifCondIsComparisonShape(cond ast.Expr, candidates map[string]struct{}) (string, narrowState, bool) {
	name, state, ok := recognisedGuardShape(cond)
	if !ok {
		return "", narrowState{}, false
	}
	if _, isCandidate := candidates[name]; !isCandidate {
		return "", narrowState{}, false
	}
	return name, state, true
}

// promoteCallerBindingsOnGuard scans stmt for a short-
// circuiting guard and promotes the affected cond-source
// pending bindings to narrowed based on the fall-through's
// narrow outcome. Nil-source bindings never enter pending, so
// this step never acts on them.
//
// The guard shape is a bare-identifier equality comparison
// (`x == V` or `x != V` where V is nil or a basic literal).
// Short-circuiting the guard's body removes that branch, so
// the fall-through runs under the opposite state; every
// pending binding whose paired guardName matches x and whose
// rule antecedent matches that opposite state promotes. The
// direct-target guard shape (the caller guarded the bound
// local directly rather than the paired local) also lands
// here — a pending binding whose target is x itself and whose
// narrow the fall-through directly proves promotes on the
// same axis.
//
// An if statement with an init clause is skipped so the init
// clause cannot rebind the identifier the condition reads. A
// guard whose body does not short-circuit is also skipped —
// falling through preserves the pending state.
func promoteCallerBindingsOnGuard(
	stmt ast.Stmt,
	pending map[string]callerBinding,
	narrowed map[string]callerBinding,
	narrowedNames map[string]struct{},
) {
	if len(pending) == 0 {
		return
	}
	ifs, ok := stmt.(*ast.IfStmt)
	if !ok || ifs.Init != nil {
		return
	}
	if !branchShortCircuits(ifs.Body) {
		return
	}
	guardName, guardState, ok := recognisedGuardShape(ifs.Cond)
	if !ok {
		return
	}
	fallThroughState := oppositeGuardState(guardState)
	applyBodyNarrow(guardName, fallThroughState, pending, narrowed, narrowedNames)
	// Direct-target guard: the local named by guardName is
	// itself the target of a pending binding, and the fall-
	// through directly proves the binding's target narrow.
	if binding, exists := pending[guardName]; exists && narrowStatesMatch(binding.narrow, fallThroughState) {
		narrowed[guardName] = binding
		narrowedNames[guardName] = struct{}{}
		delete(pending, guardName)
	}
}

// invalidateCallerBindings drops every pending or narrowed
// binding this statement can rewrite. Assignment LHS, inc/dec,
// and address-of walks match the vow:nil surface's leave-alone
// stance, and the cond-source cascade drops any pending or
// narrowed cond binding whose guardName pairs with a rewritten
// local so a stale guard reference cannot survive a
// reassignment.
func invalidateCallerBindings(
	stmt ast.Stmt,
	pending map[string]callerBinding,
	narrowed map[string]callerBinding,
	narrowedNames map[string]struct{},
) {
	if len(pending) == 0 && len(narrowedNames) == 0 {
		return
	}
	drop := func(name string) {
		// The blank identifier is not a real local: an assignment
		// `_ = X` cannot invalidate any binding because `_` never
		// carries an identity a binding could reference. Skipping
		// the drop here also protects a pending cond binding
		// whose guardName is `_` (a caller that wrote
		// `foo, _ := New()`) from being cascade-deleted by an
		// unrelated `_ = other()` statement later in the block.
		if name == "_" {
			return
		}
		delete(pending, name)
		delete(narrowed, name)
		delete(narrowedNames, name)
		// Cond-source cascade: a reassigned local cannot serve as
		// the guard reference for any cond binding that named it.
		// Nil-source bindings have no cross-identifier dependency
		// so the walk skips them.
		for bound, binding := range pending {
			if binding.source == bindingSourceCond && binding.guardName == name {
				delete(pending, bound)
			}
		}
		for bound, binding := range narrowed {
			if binding.source == bindingSourceCond && binding.guardName == name {
				delete(narrowed, bound)
				delete(narrowedNames, bound)
			}
		}
	}
	ast.Inspect(stmt, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range v.Lhs {
				if name, ok := nameIfBareIdent(lhs); ok {
					drop(name)
				}
			}
		case *ast.IncDecStmt:
			if name, ok := nameIfBareIdent(v.X); ok {
				drop(name)
			}
		case *ast.UnaryExpr:
			if v.Op == token.AND {
				if name, ok := nameIfBareIdent(v.X); ok {
					drop(name)
				}
			}
		}
		return true
	})
}

// recordCallerBindings admits new bindings from a single-call
// assignment. Nil-source bindings (callee return slot pinned
// `!`) land directly in narrowed because the narrow is
// unconditional; cond-source bindings enter pending and wait
// for a paired guard to promote them. A single call can emit
// both kinds — a nil-source non-nil return alongside a cond
// rule referencing the same slot — in which case the nil-source
// narrow wins and the cond-source binding is dropped (a slot
// that is already unconditionally non-nil has no conditional
// narrow to add).
func recordCallerBindings(
	pass *analysis.Pass,
	state *passState,
	stmt ast.Stmt,
	pending map[string]callerBinding,
	narrowed map[string]callerBinding,
	narrowedNames map[string]struct{},
) {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assign.Rhs) != 1 {
		return
	}
	call, ok := ast.Unparen(assign.Rhs[0]).(*ast.CallExpr)
	if !ok {
		return
	}
	callee := calleeObject(pass, call)
	if callee == nil {
		return
	}
	recordCallerNilBindings(pass, state, callee, assign, narrowed, narrowedNames)
	recordCallerCondBindings(pass, state, callee, assign, pending, narrowed)
}

// recordCallerNilBindings admits nil-source narrow bindings
// directly into narrowed for every return slot the callee's
// signature-mirror vow:nil declaration pins as `!`. Only
// single-call assignments contribute; a tuple of independent
// expressions does not because the LHS-to-return-slot
// alignment only holds for the single-call shape.
func recordCallerNilBindings(
	pass *analysis.Pass,
	state *passState,
	callee types.Object,
	assign *ast.AssignStmt,
	narrowed map[string]callerBinding,
	narrowedNames map[string]struct{},
) {
	sig := nilDeclForFunc(pass, state, callee)
	if sig == nil {
		return
	}
	for i, lhs := range assign.Lhs {
		if i >= len(sig.Returns) {
			break
		}
		decl := sig.Returns[i]
		if decl == nil || decl.Nullness != dsl.NullnessNonNil {
			continue
		}
		name, isIdent := nameIfBareIdent(lhs)
		if !isIdent || name == "_" {
			continue
		}
		narrowed[name] = callerBinding{
			calleeName: callee.Name(),
			returnPos:  i + 1,
			source:     bindingSourceNil,
			narrow:     narrowState{op: dsl.OpNeq, value: nilNarrowValue()},
		}
		narrowedNames[name] = struct{}{}
	}
}

// recordCallerCondBindings admits cond-source pending bindings
// for every callee logical-arrow rule that fits the caller-
// narrow shape. A cond binding is dropped when the same target
// name already carries a nil-source narrow (the unconditional
// narrow supersedes the conditional one), or when the target /
// guard positions cannot align against the assignment.
func recordCallerCondBindings(
	pass *analysis.Pass,
	state *passState,
	callee types.Object,
	assign *ast.AssignStmt,
	pending map[string]callerBinding,
	narrowed map[string]callerBinding,
) {
	rules := logicalRulesForCallee(pass, state, callee)
	if len(rules) == 0 {
		return
	}
	sigType, hasSig := callee.Type().(*types.Signature)
	if !hasSig || sigType.Results() == nil {
		return
	}
	for _, rule := range rules {
		shape, matched := condCallerNarrowRuleShape(rule)
		if !matched {
			continue
		}
		guardPos, guardOK := returnPositionForReference(sigType, shape.guardRef)
		targetPos, targetOK := returnPositionForReference(sigType, shape.targetRef)
		if !guardOK || !targetOK || guardPos == targetPos {
			continue
		}
		if guardPos < 1 || guardPos > len(assign.Lhs) {
			continue
		}
		if targetPos < 1 || targetPos > len(assign.Lhs) {
			continue
		}
		guardName, guardBare := nameIfBareIdent(assign.Lhs[guardPos-1])
		if !guardBare {
			continue
		}
		targetName, targetBare := nameIfBareIdent(assign.Lhs[targetPos-1])
		if !targetBare || targetName == "_" {
			continue
		}
		if _, alreadyNarrowed := narrowed[targetName]; alreadyNarrowed {
			continue
		}
		if _, alreadyPending := pending[targetName]; alreadyPending {
			continue
		}
		pending[targetName] = callerBinding{
			calleeName:  callee.Name(),
			returnPos:   targetPos,
			source:      bindingSourceCond,
			narrow:      shape.targetNarrow,
			guardNarrow: shape.guardNarrow,
			ruleSrc:     shape.ruleSrc,
			guardName:   guardName,
		}
	}
}

// ifCondIsNonNilCheckOnBareIdent reports the identifier name
// when cond is `x != nil` (or `nil != x`) for some bare
// identifier x. Mirrors ifCondIsNilCheckOnParam on the opposite
// comparison operator without a candidate filter — the caller
// filters against its own pending map.
func ifCondIsNonNilCheckOnBareIdent(cond ast.Expr) (string, bool) {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op.String() != "!=" {
		return "", false
	}
	if name, ok := nameIfBareIdent(bin.X); ok && isBareNilIdent(bin.Y) {
		return name, true
	}
	if name, ok := nameIfBareIdent(bin.Y); ok && isBareNilIdent(bin.X) {
		return name, true
	}
	return "", false
}

// ifCondIsNilCheckOnBareIdent reports the identifier name when
// cond is `x == nil` (or `nil == x`) for some bare identifier
// x. The direct-target-guard sibling of the non-nil helper
// above.
func ifCondIsNilCheckOnBareIdent(cond ast.Expr) (string, bool) {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op.String() != "==" {
		return "", false
	}
	if name, ok := nameIfBareIdent(bin.X); ok && isBareNilIdent(bin.Y) {
		return name, true
	}
	if name, ok := nameIfBareIdent(bin.Y); ok && isBareNilIdent(bin.X) {
		return name, true
	}
	return "", false
}

// condCallerNarrowRuleShape reports whether rule fits the
// caller-narrow shape and returns the guard-side reference, the
// target-side reference, the direction the target is narrowed
// to (non-nil or nil), and the rule's source spelling. Only
// forward implication and biconditional rules with one
// `<ref> == nil` operand and one `<ref> != nil` operand
// qualify; structural rules, path-bearing references, and
// rules whose comparisons are not the bare nil-check shape stay
// out of scope.
//
// callerRuleShape describes a rule that fits the caller-narrow
// surface: two bare-reference equality checks paired by `=>` or
// `<=>`. The guard side carries the rule's antecedent (the
// state a caller-side guard must prove for the implication to
// fire); the target side carries the rule's consequent (the
// state the target binding is narrowed to once the antecedent
// holds).
type callerRuleShape struct {
	guardRef     dsl.Reference
	guardNarrow  narrowState
	targetRef    dsl.Reference
	targetNarrow narrowState
	ruleSrc      string
}

// condCallerNarrowRuleShape reports whether rule fits the
// caller-narrow shape and returns the paired guard / target
// references, the narrow states each side carries, and the
// rule's source spelling. Only forward implication and
// biconditional rules whose two operands are bare-reference
// equality checks against a nil marker or a basic literal
// qualify; structural rules, path-bearing references, ordered
// comparisons, and comparisons whose right-hand side carries a
// sum of literals stay out of scope.
//
// A biconditional rule commutes, so both operand orders yield
// the same narrow relation — the recogniser accepts either
// order and picks the guard / target sides from the operator
// alignment (Eq on the guard side, Neq on the target side for
// the classic nil-check shape; the generalised shape simply
// keeps each side's op and value in the returned narrowState).
// A forward-implication rule does not commute, so its recognised
// orders stay two distinct shapes (positive canonical
// `<ref> == V => <ref> != W` and negative canonical
// `<ref> != V => <ref> == W`).
func condCallerNarrowRuleShape(rule *dsl.ArrowRule) (callerRuleShape, bool) {
	if rule == nil || (rule.Direction != dsl.DirImply && rule.Direction != dsl.DirEquiv) {
		return callerRuleShape{}, false
	}
	if rule.Left == nil || rule.Right == nil {
		return callerRuleShape{}, false
	}
	leftState, leftOK := bareRefEqualityState(rule.Left)
	rightState, rightOK := bareRefEqualityState(rule.Right)
	if !leftOK || !rightOK {
		return callerRuleShape{}, false
	}
	if rule.Direction == dsl.DirEquiv {
		// Biconditional commutes; both operand orders describe the
		// same relation. Pick the Eq side as the guard and the
		// Neq side as the target so the classic nil-check pairing
		// (`A == nil <=> B != nil`) reads guardRef=A, targetRef=B,
		// guardNarrow={Eq, nil}, targetNarrow={Neq, nil}.
		if leftState.op == dsl.OpEq && rightState.op == dsl.OpNeq {
			return callerRuleShape{
				guardRef:     rule.Left.Ref,
				guardNarrow:  leftState,
				targetRef:    rule.Right.Ref,
				targetNarrow: rightState,
				ruleSrc:      rule.String(),
			}, true
		}
		if leftState.op == dsl.OpNeq && rightState.op == dsl.OpEq {
			return callerRuleShape{
				guardRef:     rule.Right.Ref,
				guardNarrow:  rightState,
				targetRef:    rule.Left.Ref,
				targetNarrow: leftState,
				ruleSrc:      rule.String(),
			}, true
		}
		return callerRuleShape{}, false
	}
	// Forward implication: LHS is the antecedent (guard side),
	// RHS is the consequent (target side).
	return callerRuleShape{
		guardRef:     rule.Left.Ref,
		guardNarrow:  leftState,
		targetRef:    rule.Right.Ref,
		targetNarrow: rightState,
		ruleSrc:      rule.String(),
	}, true
}

// bareRefEqualityState reports the narrow state expr encodes
// when expr is a bare-reference equality comparison. Nil-check
// operands land on a nil-shaped narrowState; equal / not-equal
// comparisons against a single literal member land on a
// literal-shaped narrowState. Path-bearing references,
// multi-member value sums, and ordered comparisons return
// ok=false so the recogniser scope stays on the bare-equality
// shape.
func bareRefEqualityState(expr *dsl.Expression) (narrowState, bool) {
	if expr == nil {
		return narrowState{}, false
	}
	if len(expr.Ref.Path) != 0 {
		return narrowState{}, false
	}
	if expr.Op != dsl.OpEq && expr.Op != dsl.OpNeq {
		return narrowState{}, false
	}
	switch expr.Kind {
	case dsl.ExprNilCheck:
		return narrowState{op: expr.Op, value: nilNarrowValue()}, true
	case dsl.ExprComparison:
		if len(expr.Members) != 1 {
			return narrowState{}, false
		}
		m := expr.Members[0]
		if m.Nil {
			return narrowState{op: expr.Op, value: nilNarrowValue()}, true
		}
		if m.Literal == "" {
			return narrowState{}, false
		}
		return narrowState{op: expr.Op, value: narrowValue{literal: m.Literal}}, true
	}
	return narrowState{}, false
}

// containsFuncLit reports whether stmt contains a function
// literal. A closure can capture a local and rewrite it on a
// path the statement-ordered walk cannot follow, so a statement
// carrying one retires every binding in the block rather than
// reasoning about which locals the literal touches.
func containsFuncLit(stmt ast.Stmt) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			found = true
			return false
		}
		return !found
	})
	return found
}

// returnPositionForReference resolves a Reference against sig's
// return list to a 1-based return position. Positional
// references (`$N`) map directly; identifier references map by
// matching the callee's named-return list. Path-bearing
// references and references that name a parameter (not a
// return) are out of scope for the caller-narrow surface.
func returnPositionForReference(sig *types.Signature, ref dsl.Reference) (int, bool) {
	if len(ref.Path) != 0 {
		return 0, false
	}
	results := sig.Results()
	if results == nil {
		return 0, false
	}
	switch ref.Kind {
	case dsl.RefDollar:
		n, err := strconv.Atoi(ref.Name)
		if err != nil || n < 1 || n > results.Len() {
			return 0, false
		}
		return n, true
	case dsl.RefIdent:
		for i := 0; i < results.Len(); i++ {
			if results.At(i).Name() == ref.Name {
				return i + 1, true
			}
		}
	}
	return 0, false
}

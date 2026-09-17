package analysis

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// validateCondCallerSafetyViolation walks every block in
// pass.Files and reports a dereference of a call-result local
// whose paired vow:cond guard has not yet short-circuited its
// counter-branch. The rule leaves the value possibly-nil until
// the paired guard runs, so a deref taken before that point —
// including the "err bound to `_` and dereferenced anyway"
// shape — reads a value the contract has not proven safe.
//
// The pass shares the same state machine as the vow:cond dead-
// guard recogniser (validateCallerDeadGuard). The
// dead-guard surface reports a guard that stands over an
// already-narrowed local; the safety-violation surface reports
// a deref that stands over a still-pending local. Together they
// cover the two ways a caller can misread the rule: keeping a
// defensive check the rule has already answered, and skipping
// the check the rule requires.
//
// The pass is AST-only. Nested if-body blocks inherit the outer
// block's pending / narrowed state through the same body-narrow
// carry the dead-guard pass uses, so a deref that hides behind
// an if-guard the outer scan does track (through the carry)
// stays silent when the rule has proven the target safe in the
// body. Other nested block kinds (for / switch / range bodies,
// function literals) still scan independently with a fresh
// state.
func validateCondCallerSafetyViolation(pass *analysis.Pass, state *passState) {
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
			checkBlockCondCallerSafetyViolation(pass, state, block, nil, nil, nil, scanned)
			return true
		})
	}
}

// checkBlockCondCallerSafetyViolation walks block's statement
// list once, running the pending / narrowed state machine from
// the dead-guard surface and reporting a deref against every
// still-pending binding. The inherited maps let a nested if-body
// scan begin with the outer block's narrow context; a top-level
// call passes nil for each. The step order matches the dead-
// guard pass — guard checks first, then invalidation, then
// promotion, then record, then if-body descent — with the
// safety check inserted right after the guard check so both
// surfaces read the same pending / narrowed snapshot at the
// same statement boundary.
func checkBlockCondCallerSafetyViolation(
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
		reportCondCallerSafetyViolation(pass, stmt, pending)
		if containsFuncLit(stmt) {
			pending = map[string]callerBinding{}
			narrowed = map[string]callerBinding{}
			narrowedNames = map[string]struct{}{}
			continue
		}
		invalidateCallerBindings(stmt, pending, narrowed, narrowedNames)
		promoteCallerBindingsOnGuard(stmt, pending, narrowed, narrowedNames)
		recordCallerBindings(pass, state, stmt, pending, narrowed, narrowedNames)
		descendSafetyIntoIfBody(pass, state, stmt, pending, narrowed, narrowedNames, scanned)
	}
}

// descendSafetyIntoIfBody scans the if-body (and else-body,
// when present) of an if statement with the outer block's
// narrow context extended by the guard's body-narrow, mirroring
// the dead-guard pass's descent. The inner block's safety check
// then reads the extended pending / narrowed maps so a deref
// that the rule proves safe past the outer guard stays silent.
func descendSafetyIntoIfBody(
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
	checkBlockCondCallerSafetyViolation(pass, state, ifs.Body, bodyPending, bodyNarrowed, bodyNarrowedNames, scanned)
	elseBlock, ok := ifs.Else.(*ast.BlockStmt)
	if !ok || elseBlock == nil {
		return
	}
	elsePending, elseNarrowed, elseNarrowedNames := cloneCallerBindings(pending), cloneCallerBindings(narrowed), cloneNameSet(narrowedNames)
	if guardKnown {
		applyBodyNarrow(bodyGuardName, oppositeGuardState(bodyGuardState), elsePending, elseNarrowed, elseNarrowedNames)
	}
	scanned[elseBlock] = struct{}{}
	checkBlockCondCallerSafetyViolation(pass, state, elseBlock, elsePending, elseNarrowed, elseNarrowedNames, scanned)
}

// reportCondCallerSafetyViolation scans stmt for a selector
// expression whose head names a still-pending binding. A
// pending binding is bound to a call whose vow:cond rule pairs
// it with a guard on another local, but no guard has yet
// short-circuited the `<lhs> != nil` counter-branch; the rule
// therefore leaves the target slot possibly-nil, and any
// selector-driven deref (`foo.Field`, `foo.Method()`) reads a
// value the contract has not proven safe.
//
// The walk stops at nested BlockStmt boundaries so a deref
// hidden behind an if-guard the outer scan cannot track is
// not attributed to the outer block's pending map. The blank
// guard identifier ("_") is a legitimate pending — Go rejects
// `if _ != nil` so the paired guard can never run, and the
// caller has explicitly opted out of reading the error — so a
// deref against that binding is exactly the shape the safety
// violation targets.
func reportCondCallerSafetyViolation(
	pass *analysis.Pass,
	stmt ast.Stmt,
	pending map[string]callerBinding,
) {
	if len(pending) == 0 {
		return
	}
	reported := map[string]struct{}{}
	ast.Inspect(stmt, func(n ast.Node) bool {
		if _, isBlock := n.(*ast.BlockStmt); isBlock {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name, ok := nameIfBareIdent(sel.X)
		if !ok {
			return true
		}
		binding, isPending := pending[name]
		if !isPending || binding.source != bindingSourceCond {
			return true
		}
		if _, done := reported[name]; done {
			return true
		}
		reported[name] = struct{}{}
		vowReportNilSafetyf(
			pass,
			sel.Pos(),
			"vow[nil-safety]: %s is dereferenced without a short-circuiting guard on %s; %s rule %q on %s leaves return position %d possibly-nil until that guard runs",
			name,
			binding.guardName,
			rulesMarker,
			binding.ruleSrc,
			binding.calleeName,
			binding.returnPos,
		)
		return true
	})
}

package analysis

import (
	"go/types"
	"slices"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// checkLogicalRuleSetInconsistencies walks every function whose
// `vow:cond` payload carries a logical-arrow rule and reports
// contradictory rule pairs. Two rules are contradictory when their
// left-hand sides match and their right-hand sides are boolean
// negations of one another: under the matching left-hand side the
// two rules demand opposite right-hand side truth, so no return
// statement and no caller can satisfy both at the same time.
//
// The walk inspects the original rule together with the derived
// contrapositive and every transitive chain closure the rule pool
// produces, so a contradiction that surfaces only after the chain
// expansion still reaches the diagnostic. Diagnostics anchor at
// the function declaration so an author sees the offending rule
// set without having to re-derive the chain through the source.
func checkLogicalRuleSetInconsistencies(pass *analysis.Pass, state *passState) {
	for obj, rules := range state.logicalRulesSet(pass) {
		if obj == nil || len(rules) < 2 {
			continue
		}
		reportRuleSetContradictions(pass, obj, rules)
	}
}

// inconsistencyEntryKind discriminates the derivation path an
// inconsistencyEntry took: an author-written original, the
// derived contrapositive of one, or a transitive chain closure.
// The pairwise contradiction reporter reads the kind to drop
// pairs whose contradiction comes solely from two derived
// contrapositives, because such a pair conveys the same logical
// content as the pair of originals it derives from and emits
// would duplicate the offence the originals already cover (or,
// when the originals do not contradict, surface an
// unsatisfiable-LHS by-product rather than a true rule-set
// contradiction).
type inconsistencyEntryKind int

const (
	entryOriginal inconsistencyEntryKind = iota
	entryContrapositive
	entryTransitive
)

// inconsistencyEntry pairs a derived rule with the rendered
// surface form the diagnostic announces. The Desc field already
// carries the `vow:cond`, `contrapositive of vow:cond`, or
// `transitive closure of ...` prefix the upstream renderers
// produce, so the contradiction reporter prints each side through
// the same shape a reader saw at the original-rule diagnostic.
// Kind names the derivation path so the reporter can filter
// pairs whose contradiction reduces to a pure-contrapositive
// derivation.
type inconsistencyEntry struct {
	Rule *dsl.ArrowRule
	Desc string
	Kind inconsistencyEntryKind
}

// reportRuleSetContradictions materialises the rule pool the
// contradiction walk inspects (original + derived contrapositive
// + transitive chain closures) and reports every pair whose
// left-hand sides match and whose right-hand sides negate each
// other. A duplicate-pair guard keys on the rendered description
// pair so a contradiction never emits twice even when the
// derived pool surfaces both orderings of the same pair, and a
// root-axis guard keys on the unordered pair of left-hand-side
// expressions together with the unordered pair of right-hand-
// side expressions so the same hypothesis-and-axis offence does
// not emit twice through complementary derivation paths (one
// direct, the other transitive) that share both the left-hand
// hypothesis and the right-hand axis of contradiction.
func reportRuleSetContradictions(pass *analysis.Pass, obj types.Object, rules []*dsl.ArrowRule) {
	entries := collectInconsistencyEntries(rules)
	if len(entries) < 2 {
		return
	}
	reported := map[string]bool{}
	reportedRoot := map[string]bool{}
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].Kind == entryContrapositive && entries[j].Kind == entryContrapositive {
				continue
			}
			if !rulesContradict(entries[i].Rule, entries[j].Rule) {
				continue
			}
			key := contradictionPairKey(entries[i].Desc, entries[j].Desc)
			if reported[key] {
				continue
			}
			rootKey := contradictionRootKey(entries[i].Rule, entries[j].Rule)
			if reportedRoot[rootKey] {
				continue
			}
			reported[key] = true
			reportedRoot[rootKey] = true
			vowReportf(
				pass,
				obj.Pos(),
				"vow[sentinel-error]: contradictory vow:cond rules on %s: %s and %s share their left-hand side but require opposite right-hand side truth",
				obj.Name(),
				entries[i].Desc,
				entries[j].Desc,
			)
		}
	}
}

// contradictionRootKey renders the unordered pair of left-hand-
// side expressions a contradiction shares together with the
// unordered pair of right-hand-side expressions. Two entries
// that contradict at the same hypothesis (matching left-hand
// sides) and at the same right-hand-side axis (matching right-
// hand-side operand pair) collapse to the same key so the
// caller suppresses the second emission when one author rule
// pairs with a transitive closure that re-derives the same axis
// of offence. Contradictions that share the hypothesis but
// span distinct right-hand-side axes — a multi-axis rule set
// whose author intends each axis to surface independently —
// produce distinct keys and stay visible. The unordered render
// orders the operand strings lexicographically so the key is
// stable across the i, j ordering the pairwise walk visits.
func contradictionRootKey(a, b *dsl.ArrowRule) string {
	leftA, rightA := ruleOperandStrings(a)
	leftB, rightB := ruleOperandStrings(b)
	lhsKey := orderedPairKey(leftA, leftB)
	rhsKey := orderedPairKey(rightA, rightB)
	return lhsKey + " :: " + rhsKey
}

// ruleOperandStrings returns the rendered left and right operand
// strings of a rule, or empty strings when the rule or either
// operand is nil. The empty fallback keeps contradictionRootKey
// from panicking on the degenerate input the rulesContradict
// gate has already filtered out at the call site.
func ruleOperandStrings(r *dsl.ArrowRule) (string, string) {
	if r == nil {
		return "", ""
	}
	left := ""
	if r.Left != nil {
		left = r.Left.String()
	}
	right := ""
	if r.Right != nil {
		right = r.Right.String()
	}
	return left, right
}

// orderedPairKey returns a lexicographically ordered separator-
// joined pair so callers obtain a key that is invariant under
// the operand ordering at the call site.
func orderedPairKey(a, b string) string {
	if a <= b {
		return a + " || " + b
	}
	return b + " || " + a
}

// collectInconsistencyEntries returns the derived rule pool the
// contradiction walk inspects. Each author-written rule
// contributes its original surface; a forward implication also
// contributes its contrapositive and every transitive chain
// closure the grouping pass produces. The rendered description
// mirrors the diagnostic prefix the return-site and call-site
// evaluators print so a reader recognises which derivation a
// contradiction surfaced through.
//
// A seen set keyed by (rule pointer, derivation kind) guards
// against duplicate entries. The current grouping pass produces
// distinct rule pointers per origin, so the guard runs as a
// defensive invariant rather than as an active deduplicator;
// it keeps the pairwise contradiction walk from inspecting the
// same derived rule twice when the grouping pass shares pointers
// across groups.
func collectInconsistencyEntries(rules []*dsl.ArrowRule) []inconsistencyEntry {
	var entries []inconsistencyEntry
	seen := map[inconsistencyEntryKey]bool{}
	push := func(e inconsistencyEntry) {
		key := inconsistencyEntryKey{Rule: e.Rule, Kind: e.Kind}
		if seen[key] {
			return
		}
		seen[key] = true
		entries = append(entries, e)
	}
	for _, group := range groupRulesWithContrapositives(rules) {
		push(inconsistencyEntry{
			Rule: group.Original,
			Desc: describeLogicalRule(group.Original, nil),
			Kind: entryOriginal,
		})
		if group.Contrapositive != nil {
			push(inconsistencyEntry{
				Rule: group.Contrapositive,
				Desc: describeLogicalRule(group.Contrapositive, group.Original),
				Kind: entryContrapositive,
			})
		}
		for _, t := range group.Transitives {
			push(inconsistencyEntry{
				Rule: t.Rule,
				Desc: "transitive closure of " + t.ChainDesc,
				Kind: entryTransitive,
			})
		}
	}
	return entries
}

// inconsistencyEntryKey identifies an entry by the pair of its
// underlying rule pointer and the derivation kind that produced
// it. The collection helpers use the key as a defensive guard
// against changes that share a `*dsl.ArrowRule` across
// groups; the current grouping pass produces distinct pointers
// so the guard runs without engaging on present-day inputs.
type inconsistencyEntryKey struct {
	Rule *dsl.ArrowRule
	Kind inconsistencyEntryKind
}

// rulesContradict reports whether two rules share a left-hand
// side and require opposite right-hand side truth at that LHS.
// The helper negates the second rule's RHS through
// negateExpression and compares it to the first rule's RHS
// through expressionsEqual; both helpers come from the
// contrapositive derivation pass and read structural fields
// rather than the rendered surface form.
//
// The arrow direction does not gate the check because both
// `=>` and `<=>` demand RHS truth align with LHS truth under a
// matching left-hand side: a forward implication requires RHS
// hold whenever LHS holds, and a biconditional strengthens that
// to equivalence. A contradiction at the RHS level surfaces
// under either direction equally, so the pairwise walk reports
// mixed-direction pairs alongside same-direction pairs.
func rulesContradict(a, b *dsl.ArrowRule) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Left == nil || a.Right == nil || b.Left == nil || b.Right == nil {
		return false
	}
	if !expressionsEqual(a.Left, b.Left) {
		return false
	}
	negB, ok := negateExpression(b.Right)
	if !ok {
		return false
	}
	return expressionsEqual(a.Right, negB)
}

// contradictionPairKey returns a stable identifier for an
// unordered pair of rule descriptions so the diagnostic emits at
// most once per offending pair regardless of which derived
// rendering the outer loops visit first.
func contradictionPairKey(a, b string) string {
	if a <= b {
		return a + "\x00" + b
	}
	return b + "\x00" + a
}

// checkLogicalRuleNilDeclConsistency walks every function whose
// `vow:cond` payload carries a logical-arrow rule and whose
// `vow:nil` signature pins one or more positions as non-nil.
// The walk
// evaluates each rule's operands against the signature contract:
// a nil-check operand whose Reference names a position the
// signature declares as `!` resolves to a fixed truth at the
// function boundary, so `p == nil` reads as false and `p != nil`
// reads as true. When both operands of a rule (or any derived
// contrapositive or transitive chain closure) resolve and their
// boolean combination contradicts the rule's direction, the
// walker reports the inconsistency at the function declaration.
//
// Nullable (`?`) and platform positions stay undecided because
// the signature permits either nil shape, and comparison
// operands stay undecided because vow:nil tracks nil presence
// rather than scalar equality. The two-decisive-operand
// requirement keeps the diagnostic free of false positives that
// would arise from one-sided narrowing.
func checkLogicalRuleNilDeclConsistency(pass *analysis.Pass, state *passState) {
	for obj, rules := range state.logicalRulesSet(pass) {
		if obj == nil || len(rules) == 0 {
			continue
		}
		sig := nilDeclForFunc(pass, state, obj)
		if sig == nil {
			continue
		}
		typesSig, ok := signatureFromObj(obj)
		if !ok {
			continue
		}
		ctx := buildSignatureNameContext(typesSig)
		reportNilDeclContradictions(pass, obj, sig, rules, ctx)
	}
}

// signatureNameContext bundles the parameter, receiver, and
// return name slices the operand resolver reads. The bundle
// keeps the contradiction walker's signature short and lets the
// helper construct the context once per function rather than
// re-deriving the names at every operand lookup.
type signatureNameContext struct {
	ParamNames  []string
	RecvName    string
	ResultNames []string
}

// signatureFromObj returns the underlying *types.Signature for
// obj when obj is a function. Non-function objects (constants,
// variables, type names) carry no parameter list, so the
// `vow:cond` cross-check has nothing to bind against and the
// caller skips them.
func signatureFromObj(obj types.Object) (*types.Signature, bool) {
	funcObj, ok := obj.(*types.Func)
	if !ok {
		return nil, false
	}
	sig, ok := funcObj.Type().(*types.Signature)
	if !ok {
		return nil, false
	}
	return sig, true
}

// buildSignatureNameContext returns the declared names of sig's
// regular parameters, its receiver (or the empty string when
// sig declares no receiver), and its results in source order.
// Unnamed positions carry the empty string so the operand
// resolver skips them through the same predicate the parameter
// list uses on a missing identifier.
func buildSignatureNameContext(sig *types.Signature) signatureNameContext {
	recvName := ""
	if recv := sig.Recv(); recv != nil {
		recvName = recv.Name()
	}
	results := sig.Results()
	resultNames := make([]string, results.Len())
	for i := 0; i < results.Len(); i++ {
		resultNames[i] = results.At(i).Name()
	}
	return signatureNameContext{
		ParamNames:  signatureParamNamesFromTypes(sig),
		RecvName:    recvName,
		ResultNames: resultNames,
	}
}

// reportNilDeclContradictions walks the derived rule pool for
// obj and reports every entry whose two operands the vow:nil
// signature decides into a boolean combination that contradicts
// the rule's direction. The walk skips derived contrapositives
// because the contrapositive of a rule is logically equivalent
// to the rule itself under any fixed-truth context: a signature
// that fixes the operand truth makes the contrapositive
// contradict whenever the original contradicts, so emitting
// both would surface the same logical offence twice. Transitive
// chain closures stay in the walk because the chain expansion
// produces new operand pairs the original rule list does not
// carry directly.
//
// A diagnostic anchors at the function declaration because the
// offence belongs to the rule together with the signature; no
// return or call site is involved at this layer.
func reportNilDeclContradictions(pass *analysis.Pass, obj types.Object, sig *dsl.NilSignature, rules []*dsl.ArrowRule, ctx signatureNameContext) {
	reported := map[string]bool{}
	for _, entry := range collectNilDeclCheckEntries(rules) {
		rule := entry.Rule
		if rule == nil || rule.Left == nil || rule.Right == nil {
			continue
		}
		lhs, lOk := evaluateOperandFromNilDecl(sig, rule.Left, ctx)
		if !lOk {
			continue
		}
		rhs, rOk := evaluateOperandFromNilDecl(sig, rule.Right, ctx)
		if !rOk {
			continue
		}
		if logicalRuleHolds(lhs, rhs, rule.Direction) {
			continue
		}
		if reported[entry.Desc] {
			continue
		}
		reported[entry.Desc] = true
		vowReportf(
			pass,
			obj.Pos(),
			"vow[sentinel-error]: vow:cond rule on %s contradicts signature: %s: %s",
			obj.Name(),
			entry.Desc,
			logicalViolationReason(lhs, rhs, rule, "function boundary"),
		)
	}
}

// collectNilDeclCheckEntries returns the rule pool the
// signature cross-check walks: every author-written rule and
// every transitive chain closure the grouping pass produces.
// Derived contrapositives are excluded because they convey the
// same logical content as their source rule under a fixed-
// truth signature context, so including them would duplicate
// every diagnostic the cross-check would otherwise emit.
func collectNilDeclCheckEntries(rules []*dsl.ArrowRule) []inconsistencyEntry {
	var entries []inconsistencyEntry
	seen := map[inconsistencyEntryKey]bool{}
	push := func(e inconsistencyEntry) {
		key := inconsistencyEntryKey{Rule: e.Rule, Kind: e.Kind}
		if seen[key] {
			return
		}
		seen[key] = true
		entries = append(entries, e)
	}
	for _, group := range groupRulesWithContrapositives(rules) {
		push(inconsistencyEntry{
			Rule: group.Original,
			Desc: describeLogicalRule(group.Original, nil),
			Kind: entryOriginal,
		})
		for _, t := range group.Transitives {
			push(inconsistencyEntry{
				Rule: t.Rule,
				Desc: "transitive closure of " + t.ChainDesc,
				Kind: entryTransitive,
			})
		}
	}
	return entries
}

// evaluateOperandFromNilDecl decides the boolean outcome of an
// operand from the function's vow:nil signature alone. Only the
// `!` (non-nil) declaration carries decisive information: a
// position pinned as `!` resolves `== nil` as false and `!= nil`
// as true at the function boundary. Nullable (`?`) and platform
// positions stay undecided because the contract permits either
// nil shape; comparison operands stay undecided because vow:nil
// tracks nil presence rather than scalar value.
//
// Postfix-chained references (`p.field`, `p[idx]`) bypass the
// check because the signature contract attaches to the head
// position rather than to any field or element below it.
// Extending the cross-check to postfix-chained references
// would require the nest decl pass to expose the per-path
// slot the chain reaches.
func evaluateOperandFromNilDecl(sig *dsl.NilSignature, expr *dsl.Expression, ctx signatureNameContext) (bool, bool) {
	if sig == nil || expr == nil || expr.Kind != dsl.ExprNilCheck {
		return false, false
	}
	if len(expr.Ref.Path) > 0 {
		return false, false
	}
	decl := nilDeclForReference(sig, expr.Ref, ctx)
	if decl == nil || decl.Nullness != dsl.NullnessNonNil {
		return false, false
	}
	switch expr.Op {
	case dsl.OpEq:
		return false, true
	case dsl.OpNeq:
		return true, true
	}
	return false, false
}

// nilOnlyInductionThreshold is the minimum number of distinct
// `=>` rules narrowing a signature position to nil before the
// induction hint surfaces. A single narrowing rule is a
// legitimate design choice; the hint only fires when a pattern
// across at least two rules makes the nil-only invariant
// explicit enough to deserve a vow:nil declaration.
const nilOnlyInductionThreshold = 2

// reportNilOnlyInductionHints walks every function whose
// `vow:cond` payload narrows one or more signature positions
// only to nil across two or more author-written forward
// implications. The walk inspects the right-hand side of each
// `=>` rule and counts, per position, how many rules narrow it
// to nil (`p == nil`) and how many narrow it to non-nil
// (`p != nil`). A position whose nil-only count reaches the
// threshold and whose non-nil count stays at zero, and which
// the signature marker does not already pin to any nullness on
// that slot, surfaces a hint inviting the author to declare
// the position as nullable.
//
// The walk reads only author-written rules; derived
// contrapositives and transitive chain closures stay out of
// the count because the hint reflects author intent rather
// than chain-derived narrowing. An implication the author
// wrote on the function is a stronger signal of design intent
// than a derivation a chain expansion produced.
func reportNilOnlyInductionHints(pass *analysis.Pass, state *passState) {
	for obj, rules := range state.logicalRulesSet(pass) {
		if obj == nil || len(rules) < nilOnlyInductionThreshold {
			continue
		}
		typesSig, ok := signatureFromObj(obj)
		if !ok {
			continue
		}
		ctx := buildSignatureNameContext(typesSig)
		sig := nilDeclForFunc(pass, state, obj)
		reportNilOnlyInductionForFunc(pass, obj, sig, rules, ctx)
	}
}

// reportNilOnlyInductionForFunc tallies the nil-side and the
// non-nil-side narrowing counts per signature position across
// every author-written `=>` rule and reports the positions that
// meet the threshold without a non-nil narrowing and without an
// existing vow:nil declaration. The walk keeps a separate
// insertion-ordered key list so the diagnostic order matches
// the order positions first appear in the rule list and stays
// deterministic under successive runs.
func reportNilOnlyInductionForFunc(pass *analysis.Pass, obj types.Object, sig *dsl.NilSignature, rules []*dsl.ArrowRule, ctx signatureNameContext) {
	nilOnlyCount := map[string]int{}
	nonNilCount := map[string]int{}
	refByKey := map[string]dsl.Reference{}
	var keyOrder []string
	for _, rule := range rules {
		if rule == nil || rule.Direction != dsl.DirImply || rule.Right == nil {
			continue
		}
		rhs := rule.Right
		if rhs.Kind != dsl.ExprNilCheck || len(rhs.Ref.Path) > 0 {
			continue
		}
		key := rhs.Ref.String()
		if key == "" {
			continue
		}
		if _, seen := refByKey[key]; !seen {
			refByKey[key] = rhs.Ref
			keyOrder = append(keyOrder, key)
		}
		switch rhs.Op {
		case dsl.OpEq:
			nilOnlyCount[key]++
		case dsl.OpNeq:
			nonNilCount[key]++
		}
	}
	for _, key := range keyOrder {
		if nilOnlyCount[key] < nilOnlyInductionThreshold || nonNilCount[key] != 0 {
			continue
		}
		ref := refByKey[key]
		if !referenceNamesSignaturePosition(ref, ctx) {
			continue
		}
		if sig != nil && nilDeclForReference(sig, ref, ctx) != nil {
			continue
		}
		vowReportf(
			pass,
			obj.Pos(),
			"vow[sentinel-error]: %s of %s is narrowed only to nil by %d vow:cond rules; consider declaring vow:nil for %s",
			ref.String(),
			obj.Name(),
			nilOnlyCount[key],
			ref.String(),
		)
	}
}

// referenceNamesSignaturePosition reports whether ref binds to
// a receiver, regular parameter, or named return slot of the
// enclosing function. The induction hint only surfaces for
// positions the vow:nil marker can constrain, so references
// that name no signature slot (free identifiers, unrelated
// scopes) are dropped here.
func referenceNamesSignaturePosition(ref dsl.Reference, ctx signatureNameContext) bool {
	if len(ref.Path) > 0 {
		return false
	}
	switch ref.Kind {
	case dsl.RefIdent:
		if ctx.RecvName != "" && ref.Name == ctx.RecvName {
			return true
		}
		if slices.ContainsFunc(ctx.ParamNames, func(name string) bool {
			return name != "" && name == ref.Name
		}) {
			return true
		}
		if slices.ContainsFunc(ctx.ResultNames, func(name string) bool {
			return name != "" && name == ref.Name
		}) {
			return true
		}
	case dsl.RefDollar:
		if n, err := strconv.Atoi(ref.Name); err == nil && n >= 1 && n-1 < len(ctx.ResultNames) {
			return true
		}
	}
	return false
}

// nilDeclForReference returns the PositionDecl the vow:nil
// signature attaches to ref's named position, or nil when ref
// does not bind to a slot the signature constrains. A receiver
// name wins over a parameter with the same identifier so a
// method whose receiver and a regular parameter share a name
// (an unusual but legal Go shape) anchors on the receiver
// decl. The `$N` positional reference resolves against the
// return list because the grammar reserves the form for return
// slots; named returns reach the return list through the
// identifier branch.
func nilDeclForReference(sig *dsl.NilSignature, ref dsl.Reference, ctx signatureNameContext) *dsl.PositionDecl {
	switch ref.Kind {
	case dsl.RefIdent:
		if ctx.RecvName != "" && ref.Name == ctx.RecvName {
			return sig.Recv
		}
		for i, name := range ctx.ParamNames {
			if name != "" && name == ref.Name && i < len(sig.Params) {
				return sig.Params[i]
			}
		}
		for i, name := range ctx.ResultNames {
			if name != "" && name == ref.Name && i < len(sig.Returns) {
				return sig.Returns[i]
			}
		}
	case dsl.RefDollar:
		n, err := strconv.Atoi(ref.Name)
		if err != nil || n < 1 || n-1 >= len(sig.Returns) {
			return nil
		}
		return sig.Returns[n-1]
	}
	return nil
}

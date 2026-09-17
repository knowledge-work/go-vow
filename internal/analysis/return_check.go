package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
	"github.com/knowledge-work/go-vow/internal/seq"
)

// rulesMarker is the doc-comment marker that introduces a
// `vow:cond` declarative-spec annotation. The payload after
// the marker carries both a parameter requirement and a return
// requirement, split on `->`, and parses through
// ParseConditionAnnotation directly.
const rulesMarker = "vow:cond"

// checkReturnAnnotations diagnoses return statements that violate the
// direct-sum positions (or top-level tuple-sum) declared on their
// enclosing function.
//
// Three member shapes are handled per sum:
//
//   - Term:    `ErrFoo`, `Result`, `*int` — matched against the name
//     of the returned identifier (or selector tail).
//   - Literal: `nil`, `true`, `1`, `"foo"` — matched against the raw
//     value of a BasicLit / bool / nil ident in the return.
//   - Tuple:   `(a, b)` — only valid in a top-level tuple-sum; matched
//     element-by-element against ReturnStmt.Results.
//
// Single Term positions outside a sum accept any identifier on the
// matching axes the shape comparison covers (syntactic name, fullName
// from types.Info, typeName, and underlying).
func checkReturnAnnotations(pass *analysis.Pass, state *passState) {
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
			for _, cond := range conds {
				if len(cond.Subject) == 0 {
					continue
				}
				_, failedAt, status := resolveSignatureSubjectChain(pass, fn, cond.Subject)
				switch status {
				case chainResolveFailed:
					reportChainSubjectUnresolved(pass, fn.Pos(), rulesMarker, cond.Subject, failedAt)
				case chainStepFailed:
					reportChainStepNotCallback(pass, fn.Pos(), rulesMarker, cond.Subject, failedAt)
				}
			}
			// Annotation-shape diagnostics (arity mismatch and
			// const-with-constraint) describe each case
			// independently. They fire per-condition so a
			// multi-line annotation gets one diagnostic per
			// offending case, rather than the matcher silently
			// running against a different arity than the author
			// wrote. A function whose first case already trips
			// the arity check skips that case but still runs the
			// remaining ones — the case-style semantic admits
			// independent contracts on the same function.
			arityOK := make([]bool, len(conds))
			for i, cond := range conds {
				if len(cond.Subject) > 0 {
					// Subject-scoped conditions describe a higher-
					// order contract a layer wires to
					// executable semantics; the per-return validator
					// below would mis-apply the case against the
					// enclosing function's returns.
					continue
				}
				if cond.Conseq == nil {
					// Higher-order arrow-rules whose conseq is
					// another rule describe function-level
					// semantics rather than a return-value shape.
					// There is nothing for the arity / position
					// checker to apply here, so the case stays
					// disabled in arityOK and the per-return
					// validator skips it.
					continue
				}
				if reportArityMismatch(pass, fn, cond.Conseq) {
					continue
				}
				arityOK[i] = true
				reportConstWithQualifier(pass, cond.Conseq, func() token.Pos { return fn.Pos() })
			}
			ssaFn := findSSAFunction(ssaFns, fn)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				ret, isRet := n.(*ast.ReturnStmt)
				if !isRet {
					return true
				}
				// Case-style semantics: each applicable case
				// (Prereq holds) enforces its return requirement.
				// A return that matches no case's prereq is
				// unchecked — the conservative single-condition
				// narrowing carries over to the multi-line
				// surface unchanged.
				for i, cond := range conds {
					if !arityOK[i] {
						continue
					}
					if cond.Prereq != nil {
						ssaRet := findSSAReturnFor(ssaFn, ret.Pos())
						if !prereqHoldsAtReturn(fn, ssaFn, ssaRet, cond.Prereq) {
							continue
						}
					}
					checkReturnAgainstAnnotation(pass, fn, ret, cond.Conseq, state)
				}
				return true
			})
		}
	}
}

// ssaFunctions returns the SrcFuncs of the buildssa result, or nil
// when the pass did not produce one. Callers that need the SSA
// function for a specific FuncDecl pass the result through
// findSSAFunction.
func ssaFunctions(pass *analysis.Pass) []*ssa.Function {
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok || result == nil {
		return nil
	}
	return result.SrcFuncs
}

// findSSAReturnFor is a nil-tolerant wrapper around findSSAReturn so
// the caller can pass an unresolved (nil) ssa.Function without
// guarding the lookup at every call site. Returns nil when ssaFn
// is nil; otherwise delegates to findSSAReturn.
func findSSAReturnFor(ssaFn *ssa.Function, retPos token.Pos) *ssa.Return {
	if ssaFn == nil {
		return nil
	}
	return findSSAReturn(ssaFn, retPos)
}

// parseConditionsFromDoc extracts every successfully-parsed
// vow:cond annotation as an ordered slice of *dsl.Condition. The
// case-style multi-condition semantics treat each line as an
// independent case: applicable cases (Prereq holds) all enforce
// their return requirements, and the chain-authorisation union
// covers every trivial-Prereq case the doc carries. A function
// with a single annotation line lands here as a 1-element slice
// and behaves identically to the single-line path.
//
// The original (pre-expansion) payload is forwarded to the
// marker dispatcher because the dispatcher distinguishes a
// payload that is a single rule-ref (atom-rule shape) from one
// that only contains rule-refs as embedded template expansions.
// Once the dispatcher decides which shape applies, it either
// resolves the rule reference directly or falls through to the
// textual-expansion path that drives multi-rule and arrow-rule
// payloads.
//
// The optional `[<scope>]` qualifier on the marker is captured at
// the lexer level and stored on Condition.Subject; an empty Subject
// means the marker carried no scope and the condition applies to
// the enclosing function as usual.
func parseConditionsFromDoc(decl *ast.FuncDecl, scope dsl.RuleScope) ([]*dsl.Condition, bool) {
	if decl.Doc == nil {
		return nil, false
	}
	var out []*dsl.Condition
	for _, c := range decl.Doc.List {
		line := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		marker, subject, payload, ok := splitDocMarker(line)
		if !ok {
			continue
		}
		cond, err := parsePayloadByMarker(marker, payload, scope)
		if err != nil {
			continue
		}
		cond.Subject = subject
		out = append(out, cond)
	}
	return out, len(out) > 0
}

// splitDocMarker peels the vow:cond prefix from a doc-comment
// line and returns the marker, its optional scope chain, and its
// payload. A line that does not carry the marker (or carries it
// with an empty payload) reports ok=false so the caller skips it
// without further inspection. An empty subject chain means the
// marker carried no scope and the condition applies to the
// enclosing function as usual.
//
// vow:cond accepts the chained `[X][Y]...` scope qualifier so the
// condition retargets at nested higher-order callbacks.
func splitDocMarker(line string) (marker string, subject dsl.SubjectChain, payload string, ok bool) {
	chain, body, hit := stripMarkerScopeChain(line, rulesMarker)
	if !hit || body == "" {
		return "", nil, "", false
	}
	return rulesMarker, dsl.SubjectChain(chain), body, true
}

// parsePayloadByMarker dispatches a `vow:cond` payload to the
// matching parser. A single-rule-ref payload (`@alias.Rule[args]`)
// resolves through liftAtomRuleViaScope: the preset-YAML rule
// expands its body into a return annotation that the existing
// checker keeps applying through the Conseq field. Every other
// payload (multi-rule `;`-separated, arrow-rule shapes, plain
// values) falls through to scope.Expand + the standard parser
// entry so embedded rule-refs (e.g. `@rule.OkErr` inside a sum)
// continue to splice their body in-line.
//
// vow:cond * -> _, (ErrUnknownMarker | _)?
func parsePayloadByMarker(marker, payload string, scope dsl.RuleScope) (*dsl.Condition, error) {
	if marker != rulesMarker {
		return nil, fmt.Errorf("unknown marker %q: %w", marker, ErrUnknownMarker)
	}
	if ref, ok := dsl.AsSingleRuleRef(payload); ok {
		return liftAtomRuleViaScope(ref, scope)
	}
	expanded, err := scope.Expand(payload)
	if err != nil {
		return nil, err
	}
	return dsl.ParseConditionAnnotation(expanded)
}

// liftAtomRuleViaScope resolves a single-rule-ref atom-rule
// payload through the preset-YAML path: the rule body is expanded
// against the file's RuleScope and parsed back into a
// ReturnAnnotation, which the existing function-level shape
// validation consumes.
//
// vow:cond * -> _, (ErrSyntax | ErrUnknownReference | _)?
func liftAtomRuleViaScope(ref *dsl.RuleRef, scope dsl.RuleScope) (*dsl.Condition, error) {
	def, err := scope.Lookup(ref.Alias, ref.RuleName)
	if err != nil {
		return nil, err
	}
	body, err := def.ExpandBody(ref.TypeArgs)
	if err != nil {
		return nil, err
	}
	anno, err := dsl.ParsePresetBody(body)
	if err != nil {
		return nil, err
	}
	return dsl.WrapAtomRuleAsCondition(ref, anno), nil
}

// reportArityMismatch reports when the vow:cond annotation
// declares a different number of return positions than the function
// signature. Returns true to signal the caller it should skip the
// per-return checks for this function — diagnosing both an arity
// problem and downstream "this position is wrong" noise would only
// confuse the author.
func reportArityMismatch(pass *analysis.Pass, fn *ast.FuncDecl, anno *dsl.ReturnAnnotation) bool {
	sigArity := signatureArity(fn)
	annoArity := annotationArity(anno)
	if sigArity == annoArity {
		return false
	}
	vowReportf(
		pass,
		fn.Pos(),
		"vow[sentinel-error]: vow:cond declares %d return %s but the function signature returns %d %s",
		annoArity, pluralize(annoArity, "position", "positions"),
		sigArity, pluralize(sigArity, "value", "values"),
	)
	return true
}

// pluralize returns singular for n == 1 and plural otherwise. Used to
// keep diagnostic prose grammatical without sprinkling `(s)` markers
// across the codebase.
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func signatureArity(fn *ast.FuncDecl) int {
	if fn.Type == nil || fn.Type.Results == nil {
		return 0
	}
	n := 0
	for _, field := range fn.Type.Results.List {
		if len(field.Names) == 0 {
			n++ // unnamed return
			continue
		}
		n += len(field.Names) // grouped named returns: (a, b T)
	}
	return n
}

func annotationArity(anno *dsl.ReturnAnnotation) int {
	if anno.TupleSum != nil {
		if len(anno.TupleSum.Members) == 0 || anno.TupleSum.Members[0].Tuple == nil {
			return 0
		}
		return len(anno.TupleSum.Members[0].Tuple.Elements)
	}
	return len(anno.Positions)
}

func checkReturnAgainstAnnotation(pass *analysis.Pass, fn *ast.FuncDecl, ret *ast.ReturnStmt, anno *dsl.ReturnAnnotation, state *passState) {
	if anno.TupleSum != nil {
		checkTupleSum(pass, fn, ret, anno.TupleSum, state)
		return
	}
	if len(ret.Results) != len(anno.Positions) {
		return
	}
	for i, expr := range ret.Results {
		pos := anno.Positions[i]
		reportPos := expr.Pos()
		expr = resolveExprAlias(fn, expr)
		if isDischargedASTCall(expr, pass, state) {
			continue
		}
		switch {
		case pos.Single != nil:
			if msg, bad := validateSinglePosition(pass, expr, pos.Single); bad {
				vowReportf(pass, reportPos, "vow[sentinel-error]: return position %d: %s", i, msg)
			}
		case pos.Sum != nil:
			if msg, bad := validateSumPosition(pass, expr, pos.Sum, state); bad {
				vowReportf(pass, reportPos, "vow[sentinel-error]: return position %d: %s", i, msg)
			}
		}
	}
}

// resolveExprAlias returns expr unchanged when it is not a local
// alias ident, or the alias's RHS expression when a single-define
// resolution succeeds. The function chases multi-hop alias chains
// by repeating the resolution as long as the RHS is itself another
// local alias ident, so `x := ErrFoo; y := x; return y` folds to
// `ErrFoo` for `vow:cond` validation. A visited set breaks
// pathological self-referential chains.
func resolveExprAlias(fn *ast.FuncDecl, expr ast.Expr) ast.Expr {
	visited := map[*ast.Ident]struct{}{}
	for {
		ident, ok := expr.(*ast.Ident)
		if !ok {
			return expr
		}
		if _, seen := visited[ident]; seen {
			return expr
		}
		visited[ident] = struct{}{}
		rhs := resolveLocalAlias(fn, ident)
		if rhs == nil {
			return expr
		}
		expr = rhs
	}
}

// validateSinglePosition checks the contract carried by a Single-Term
// position. A non-zero predicate (the `nonzero T` form) reports a
// violation when the return expression resolves to a constant zero
// (0, "", false, 0.0). Other constraints remain unchecked here — the
// complete wildcard `*` admits every value, and bare `T` is the
// lenient default. The optional `?` suffix never reaches a Single
// position (`T?` parses to a 1-member Optional sum).
func validateSinglePosition(pass *analysis.Pass, expr ast.Expr, term *dsl.Term) (string, bool) {
	if term.Wildcard || term.Placeholder {
		return "", false
	}
	// `nonzero T` carries the strict NonZero predicate; the
	// zero-value diagnostic identifies the offending value
	// precisely, so the strict-zero branch runs ahead of the
	// generic nil-rejection.
	if termRequiresNonZero(term) {
		if classifyZero(pass, expr) == zeroYes {
			return "returning the zero value at a position declared " + term.String(), true
		}
		return "", false
	}
	// A bare Term without a Nil predicate rejects a nil return — the
	// strict-by-default posture matches the canonical Single-Term
	// semantics. The optional `?` suffix never reaches a Single
	// position: `T?` parses to a 1-member Optional sum, so its nil
	// admission is handled by validateSumPosition.
	if termAdmitsNil(term) {
		return "", false
	}
	if shape, ok := shapeOf(pass, expr); ok && shape.kind == shapeNil {
		return "returning nil but the position is not optional", true
	}
	return "", false
}

// termAdmitsNil reports whether the term's surface form admits a
// `nil` literal at the position. The only admitting Single-Term
// shape is the `Nil` predicate, which constrains the position to
// the nil value directly. The optional `?` suffix is not a
// Single-Term shape: `T?` parses to a 1-member Optional DirectSum
// whose nil admission is handled by validateSumPosition. Every
// other Single-Term shape is strict-by-default and rejects a nil
// return at runtime.
func termAdmitsNil(t *dsl.Term) bool {
	if t == nil {
		return false
	}
	if _, ok := t.Pred.(dsl.Nil); ok {
		return true
	}
	return false
}

// termRequiresNonZero reports whether the term carries the value-
// level predicate Not{Zero{}} — the canonical representation of
// the `nonzero T` prefix surface.
func termRequiresNonZero(t *dsl.Term) bool {
	if t == nil {
		return false
	}
	return dsl.PredicateEquals(t.Pred, dsl.Not{Inner: dsl.Zero{}})
}

// sumAcceptsAnyShape reports whether sum contains a `*` wildcard
// Term member. Such a member trumps every per-shape match below
// — nil literal, basic literal, ident — so callers short-circuit
// the rest of the matcher when this returns true.
func sumAcceptsAnyShape(sum *dsl.DirectSum) bool {
	return slices.ContainsFunc(sum.Members, func(m dsl.SumMember) bool {
		return m.Term != nil && m.Term.Wildcard
	})
}

// sumHasPlaceholder reports whether sum contains a `_` placeholder
// Term member. The placeholder is the label-aware fall-through —
// it admits nil, literals, and non-subject identifiers but not
// labelled subjects, so callers query this together with
// exprIsLabelledSubject when handling shapeIdent.
func sumHasPlaceholder(sum *dsl.DirectSum) bool {
	return slices.ContainsFunc(sum.Members, func(m dsl.SumMember) bool {
		return m.Term != nil && m.Term.Placeholder
	})
}

// exprIsLabelledSubject reports whether expr resolves to one of
// the pass-wide labelled subjects. Expressions whose object
// cannot be resolved (function calls, composite literals, ...)
// are treated as non-subject so the label-aware fall-through can
// admit them — the analyzer already handles such shapes as
// "unanalysable" elsewhere, and pulling them into the
// labelled-subject branch would just turn unrelated leaks into
// sum-membership false positives.
func exprIsLabelledSubject(pass *analysis.Pass, state *passState, expr ast.Expr) bool {
	obj := resolveExprObject(pass, expr)
	if obj == nil {
		return false
	}
	return state.labelledSubjects(pass).contains(obj)
}

func checkTupleSum(pass *analysis.Pass, fn *ast.FuncDecl, ret *ast.ReturnStmt, sum *dsl.DirectSum, state *passState) {
	if len(sum.Members) == 0 || sum.Members[0].Tuple == nil {
		return
	}
	arity := len(sum.Members[0].Tuple.Elements)
	if len(ret.Results) != arity {
		return // arity mismatch is out of scope for this PR
	}
	results := seq.ChainOf(ret.Results...).Map(func(r ast.Expr) ast.Expr {
		return resolveExprAlias(fn, r)
	}).ToSlice()
	for _, r := range results {
		if isDischargedASTCall(r, pass, state) {
			return
		}
	}
	slots := resultSlotTypes(pass, fn)
	for _, m := range sum.Members {
		if m.Tuple == nil {
			continue
		}
		if tupleMatches(pass, results, slots, m.Tuple.Elements, state) {
			return
		}
	}
	reportAt := ret.Pos()
	if len(ret.Results) > 0 {
		reportAt = ret.Results[0].Pos()
	}
	vowReportf(pass, reportAt, "vow[sentinel-error]: return values do not match any tuple of the declared sum %s", sum.String())
}

func tupleMatches(pass *analysis.Pass, results []ast.Expr, slots []types.Type, elements []dsl.TupleElement, state *passState) bool {
	if len(results) != len(elements) {
		return false
	}
	for i, expr := range results {
		var slot types.Type
		if i < len(slots) {
			slot = slots[i]
		}
		if !elementMatches(pass, expr, slot, elements[i], state) {
			return false
		}
	}
	return true
}

// resultSlotTypes returns the declared type of each return position
// of fn, expanding grouped results (`(a, b *T)`) into one entry per
// position. A position whose type does not resolve yields nil.
func resultSlotTypes(pass *analysis.Pass, fn *ast.FuncDecl) []types.Type {
	if fn.Type == nil || fn.Type.Results == nil {
		return nil
	}
	var out []types.Type
	for _, field := range fn.Type.Results.List {
		t := pass.TypesInfo.TypeOf(field.Type)
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for range n {
			out = append(out, t)
		}
	}
	return out
}

func elementMatches(pass *analysis.Pass, expr ast.Expr, slot types.Type, elem dsl.TupleElement, state *passState) bool {
	shape, ok := shapeOf(pass, expr)
	if !ok {
		return false
	}
	if elem.Term != nil {
		// A `*` wildcard element admits every shape, including nil
		// literals and basic literals that the per-shape branches
		// below would reject. The trailing predicate check is also
		// skipped — the wildcard signals "no constraint here".
		if elem.Term.Wildcard {
			return true
		}
		// A `_` placeholder element admits literals, nil, and
		// non-subject idents (the label-aware fall-through). A
		// labelled subject is excluded so the author must spell
		// it out as a named tuple element to forward it.
		if elem.Term.Placeholder {
			if shape.kind != shapeIdent {
				return true
			}
			return !exprIsLabelledSubject(pass, state, expr)
		}
		// Matching by type alone would admit a nil of that type, so
		// the non-zero predicate is what carries the weight here.
		if shape.kind == shapeTyped {
			if !shape.matchesTypeAxes(elem.Term.Type) && !slotTypeMatches(pass, slot, elem.Term.Type) {
				return false
			}
			if !termRequiresNonZero(elem.Term) {
				return true
			}
			return classifyZero(pass, expr) != zeroYes && !argumentIsStaticallyNil(pass, expr)
		}
		if shape.kind != shapeIdent {
			return false
		}
		if !termMatchesIdentShape(elem.Term, shape) {
			return false
		}
		// A non-zero predicate on an element rejects zero values just
		// like a Single Term position would. The same identifier name
		// can satisfy the shape match but trip the predicate — e.g. a
		// const named `ZeroFlag = 0` paired with `(nonzero ZeroFlag, nil)`
		// should fail the tuple match instead of silently succeeding.
		if termRequiresNonZero(elem.Term) && classifyZero(pass, expr) == zeroYes {
			return false
		}
		return true
	}
	if elem.Literal != nil {
		return literalMatchesShape(elem.Literal, shape)
	}
	return false
}

// termMatchesIdentShape reports whether the term matches a
// shapeIdent return expression via the four-axis name comparison.
// A `*` wildcard term short-circuits the name comparison and
// matches every identifier; the type axis is intentionally
// ignored on the wildcard branch so the analyzer treats a pinned
// `* error` form (which carries a documentation Type) as still
// admitting every value. Type-axis matching for the wildcard is
// a candidate extension once an author surface asks for type-
// restricted completion.
func termMatchesIdentShape(term *dsl.Term, shape exprShape) bool {
	if term.Wildcard {
		return true
	}
	return shape.matchesTermName(term.Type)
}

// validateSumPosition reports whether expr is permitted by the
// declared direct sum. The check is syntactic: it inspects
// identifier-style expressions (bare or selector), the nil literal,
// and basic-literals (int / string / bool). Anything else is ignored
// as unanalyzable.
//
// When an identifier matches a Term member, the member's value-
// level predicate is also enforced — `(nonzero Foo | Bar)` and a
// return of a const-zero Foo trips the zero check. A `*` wildcard
// anywhere in the sum admits every shape (nil, literal, ident)
// before the per-shape branches even fire, so authors can write
// `(ErrFoo | *)` for "list the interesting subjects but accept
// every other value" boundaries. The `_` placeholder is the
// label-aware counterpart: it admits literals, nil, and non-
// subject identifiers, but a labelled subject the package
// declares (via vow:define @Sentinel) is excluded
// so authors who want to forward it must list it explicitly.
//
// The `_` vs `*` asymmetry is a deliberate invariant, not sugar:
// `*` admits the whole open set while `_` carves out the labelled
// subset. The same asymmetry holds on the chain-authorisation
// surface — `*` authorises every subject for propagation while
// `_` authorises none (see propagate.go sumListsSubject) — so the
// two markers carry distinct admission sets AND distinct
// propagation roles, never interchangeable.
func validateSumPosition(pass *analysis.Pass, expr ast.Expr, sum *dsl.DirectSum, state *passState) (string, bool) {
	if sumAcceptsAnyShape(sum) {
		return "", false
	}
	shape, ok := shapeOf(pass, expr)
	if !ok {
		return "", false
	}
	switch shape.kind {
	case shapeNil:
		if sum.Qualifier == dsl.QualifierOptional {
			return "", false
		}
		// `_` admits the nil literal — nil is not a labelled
		// subject so the label-aware fall-through covers it.
		if sumHasPlaceholder(sum) {
			return "", false
		}
		for _, m := range sum.Members {
			if m.Literal != nil && m.Literal.Kind == dsl.LitNil {
				return "", false
			}
		}
		return "returning nil but the position is not optional and the sum does not list nil", true
	case shapeLiteral:
		// `_` admits every basic literal — literals are not
		// labelled subjects so the fall-through covers them.
		if sumHasPlaceholder(sum) {
			return "", false
		}
		for _, m := range sum.Members {
			if m.Literal != nil && literalMatchesShape(m.Literal, shape) {
				return "", false
			}
		}
		return "returning literal " + shape.raw + " but the position only accepts " + describeSumMembers(sum), true
	case shapeIdent:
		// `_` admits an ident-shape return only when it does not
		// resolve to a labelled subject. The labelled subjects
		// still need an explicit listing so the author cannot
		// silently propagate them through a fall-through.
		if sumHasPlaceholder(sum) && !exprIsLabelledSubject(pass, state, expr) {
			return "", false
		}
		for _, m := range sum.Members {
			if m.Term == nil {
				continue
			}
			if !termMatchesIdentShape(m.Term, shape) {
				continue
			}
			if termRequiresNonZero(m.Term) && classifyZero(pass, expr) == zeroYes {
				return "matched " + m.Term.String() + " but the value is the zero of its type", true
			}
			return "", false
		}
		return "returning " + shape.name + " but the position only accepts " + describeSumMembers(sum), true
	}
	return "", false
}

// shapeKind enumerates the recognized shapes of a return expression.
type shapeKind int

const (
	shapeIdent   shapeKind = iota // bare ident or pkg.Sel
	shapeNil                      // nil ident
	shapeLiteral                  // basic literal (int / string / bool)
	// shapeTyped names no identifier an annotation could quote —
	// `&T{...}`, a constructor call — but carries a resolvable type,
	// so only a term naming that type matches it.
	shapeTyped
)

// matchesTermName reports whether the shape's identifier matches a
// Term.Type as authored in an annotation. The check tries four axes:
//
//  1. Syntactic short name (`Foo` or `pkg.Foo`) — what the author
//     typed at the call site.
//  2. types.Info-resolved fully-qualified name (`module/path.Foo`) —
//     unambiguous across packages with the same short name.
//  3. The expression's named type (`TypeOf(expr).String()`) — useful
//     when the annotation names a Go type (e.g. `Result`) and the
//     return value is of that type.
//  4. The same type rendered relative to the package under
//     analysis, which is how an in-package annotation spells it
//     (`*Foo`, not `*pkg.Foo`).
//  5. The underlying type (`TypeOf(expr).Underlying().String()`) —
//     transparently follows type aliases so `type MyError = error`
//     allows `error` annotations to match `MyError` returns.
//
// Existing short-form annotations continue to work via axis 1; the
// types-aware axes only fire when axis 1 / 2 didn't match, so the
// surface stays additive.
func (s exprShape) matchesTermName(want string) bool {
	if s.name == want {
		return true
	}
	if s.fullName != "" && s.fullName == want {
		return true
	}
	if s.typeName != "" && s.typeName == want {
		return true
	}
	if s.typeNameRel != "" && s.typeNameRel == want {
		return true
	}
	if s.typeNameQual != "" && s.typeNameQual == want {
		return true
	}
	if s.underlying != "" && s.underlying == want {
		return true
	}
	return false
}

// slotTypeMatches reports whether want names the declared type of a
// return position. A tuple sum describes the positions rather than
// the expressions filling them, so an element naming `error` covers
// a helper that hands back a concrete error type — the position is
// what the contract talks about.
func slotTypeMatches(pass *analysis.Pass, slot types.Type, want string) bool {
	if slot == nil {
		return false
	}
	if slot.String() == want ||
		types.TypeString(slot, types.RelativeTo(pass.Pkg)) == want ||
		types.TypeString(slot, packageNameQualifier(pass.Pkg)) == want {
		return true
	}
	u := slot.Underlying()
	return u != nil && u.String() == want
}

// matchesTypeAxes is matchesTermName restricted to the type axes,
// so a term naming a value cannot match a shape with no identifier.
func (s exprShape) matchesTypeAxes(want string) bool {
	if s.typeName != "" && s.typeName == want {
		return true
	}
	if s.typeNameRel != "" && s.typeNameRel == want {
		return true
	}
	if s.typeNameQual != "" && s.typeNameQual == want {
		return true
	}
	return s.underlying != "" && s.underlying == want
}

// exprShape carries everything the analyzer needs from a return
// expression to compare against a sum member. The combination of
// fields used depends on kind.
//
// name is the syntactic identifier form (`Foo` or `pkg.Foo`); the
// other identifier-flavored fields broaden the match surface so an
// annotation written in any reasonable form lines up with the
// expression's actual type:
//
//   - fullName  = types.Info-resolved package-qualified form.
//   - typeName  = TypeOf(expr).String() — annotation may name the Go
//     type directly (e.g. `Result`).
//   - underlying = TypeOf(expr).Underlying().String() — transparently
//     follows type aliases (`type MyError = error`).
//   - typeNameRel = the same type rendered relative to the package
//     under analysis, which is how an annotation in that package
//     spells it (`*Foo`, not `*pkg.Foo`).
//   - typeNameQual = the same type with foreign packages rendered by
//     name rather than import path, which is how an annotation
//     spells a type it had to import (`*pkg.Foo`).
type exprShape struct {
	kind         shapeKind
	name         string
	fullName     string
	typeName     string
	typeNameRel  string
	typeNameQual string
	underlying   string
	lit          dsl.LiteralKind // shapeLiteral
	raw          string          // shapeLiteral — textual form for comparison
}

// shapeOf extracts the comparable shape of expr, returning false when
// the expression form is unanalyzable. Parenthesized expressions are
// transparently unwrapped so `return (ErrFoo)` is treated identically
// to `return ErrFoo`. pass is consulted for types.Info-driven
// resolution of selector identifiers; passing a nil pass falls back to
// the purely syntactic shape (used by tests that exercise shapeOf in
// isolation).
func shapeOf(pass *analysis.Pass, expr ast.Expr) (exprShape, bool) {
	if paren, ok := expr.(*ast.ParenExpr); ok {
		return shapeOf(pass, paren.X)
	}
	switch e := expr.(type) {
	case *ast.Ident:
		switch e.Name {
		case "nil":
			return exprShape{kind: shapeNil}, true
		case "true", "false":
			return exprShape{kind: shapeLiteral, lit: dsl.LitBool, raw: e.Name}, true
		}
		shape := exprShape{kind: shapeIdent, name: e.Name}
		if pass != nil {
			if obj := pass.TypesInfo.ObjectOf(e); obj != nil && obj.Pkg() != nil {
				shape.fullName = obj.Pkg().Path() + "." + obj.Name()
			}
			fillTypeAxes(pass, e, &shape)
		}
		return shape, true
	case *ast.SelectorExpr:
		// Qualified identifier `pkg.Foo` — keep both halves so a
		// cross-package match doesn't collide with a same-named local
		// identifier. Annotations that want to talk about a foreign
		// preset's symbol must spell the package prefix.
		if pkgIdent, ok := e.X.(*ast.Ident); ok {
			shape := exprShape{kind: shapeIdent, name: pkgIdent.Name + "." + e.Sel.Name}
			if pass != nil {
				if obj := pass.TypesInfo.ObjectOf(e.Sel); obj != nil && obj.Pkg() != nil {
					shape.fullName = obj.Pkg().Path() + "." + obj.Name()
				}
				fillTypeAxes(pass, e, &shape)
			}
			return shape, true
		}
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			return exprShape{kind: shapeLiteral, lit: dsl.LitInt, raw: e.Value}, true
		case token.STRING:
			return exprShape{kind: shapeLiteral, lit: dsl.LitString, raw: e.Value}, true
		}
	case *ast.UnaryExpr:
		// Permit `-1`, `+0xff`, etc.
		if e.Op != token.SUB && e.Op != token.ADD {
			break
		}
		lit, ok := e.X.(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			break
		}
		raw := lit.Value
		if e.Op == token.SUB {
			raw = "-" + raw
		}
		return exprShape{kind: shapeLiteral, lit: dsl.LitInt, raw: raw}, true
	}
	// The sentinel-sum surface this function grew from only ever saw
	// named vars, so anything else read as unanalysable. A type is
	// all a type-naming term needs, and it is what a constructor's
	// returns actually carry.
	if pass != nil {
		shape := exprShape{kind: shapeTyped}
		fillTypeAxes(pass, expr, &shape)
		if shape.typeName != "" {
			return shape, true
		}
	}
	return exprShape{}, false
}

// fillTypeAxes populates the typeName and underlying axes on shape
// from pass.TypesInfo. typeName is the expression's Go type as it
// would print at a declaration site (e.g. `error`, `pkg.MyError`),
// and underlying is the result of one Underlying() step (e.g. the
// `error` interface beneath a `pkg.MyError` named type). Defined
// types whose underlying is structural — pointers, slices, maps —
// still gain a useful underlying axis ("*pkg.S", "[]byte", ...).
// Builtin and alias-resolved types where typeName == underlying do
// not pay extra: matchesTermName short-circuits on duplicates.
func fillTypeAxes(pass *analysis.Pass, e ast.Expr, shape *exprShape) {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return
	}
	shape.typeName = t.String()
	shape.typeNameRel = types.TypeString(t, types.RelativeTo(pass.Pkg))
	shape.typeNameQual = types.TypeString(t, packageNameQualifier(pass.Pkg))
	if u := t.Underlying(); u != nil {
		shape.underlying = u.String()
	}
}

// packageNameQualifier renders a foreign package by name where
// types.RelativeTo renders its import path.
//
// A file importing under an alias that diverges from the package
// name still misses; the axis follows the convention rather than
// carrying file-level import state into the shape.
func packageNameQualifier(pkg *types.Package) types.Qualifier {
	return func(other *types.Package) string {
		if other == nil || other == pkg {
			return ""
		}
		return other.Name()
	}
}

// literalMatchesShape reports whether the declared literal member
// matches a return-expression shape. The nil literal is special:
// the return expression's shape is shapeNil (a bare nil ident), not
// shapeLiteral, so it has its own branch. Other kinds match against
// shapeLiteral after normalization:
//
//   - Int: strconv.ParseInt so `0xff` and `255` compare equal.
//   - String: strconv.Unquote so escape sequences (`\n`, `\t`, `\"`)
//     and unicode escapes (`\uXXXX`) compare as the same character
//     content. Raw and interpreted string literals that decode to the
//     same value therefore match. Unquote errors fall back to byte
//     equality so author-written DSL strings still work.
//   - Bool: raw text comparison.
func literalMatchesShape(member *dsl.LiteralTerm, shape exprShape) bool {
	if member.Kind == dsl.LitNil {
		return shape.kind == shapeNil
	}
	if shape.kind != shapeLiteral || member.Kind != shape.lit {
		return false
	}
	switch member.Kind {
	case dsl.LitInt:
		a, errA := strconv.ParseInt(member.Raw, 0, 64)
		b, errB := strconv.ParseInt(shape.raw, 0, 64)
		return errA == nil && errB == nil && a == b
	case dsl.LitString:
		a, errA := strconv.Unquote(member.Raw)
		b, errB := strconv.Unquote(shape.raw)
		if errA == nil && errB == nil {
			return a == b
		}
		return member.Raw == shape.raw
	case dsl.LitBool:
		return member.Raw == shape.raw
	}
	return false
}

func describeSumMembers(sum *dsl.DirectSum) string {
	parts := seq.ChainOf(sum.Members...).Map(dsl.SumMember.String).ToSlice()
	return strings.Join(parts, " | ")
}

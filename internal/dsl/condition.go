package dsl

import (
	"fmt"
	"slices"
	"strings"

	"github.com/knowledge-work/go-vow/internal/seq"
)

// Condition is the parsed contract for a function's annotation.
// The grammar generalises the single-rule shape
// (Prereq -> Conseq) into a list of rules separated by `;`:
//
//	condition := rule (';' rule)*
//
// Rules carries the rule-list representation; it always holds at
// least one entry. Prereq and Conseq are back-compat fields kept
// for downstream consumers that do not use the rule-
// list reader — they are populated for single-rule arrow-rule
// conditions whose conseq is a plain value (the only shape the
// single-rule surface expresses). For
// atom-rule conditions whose rule resolves to an EffectExpandPayload
// (i.e. preset YAML rules like `@result.OkErr[T, E]`), the wrapper
// also populates Conseq with the expanded body so the
// `vow:cond @rule` semantics carry over at the analyzer boundary
// without rule-list-specific code paths in the existing checker.
//
// For higher-order arrow-rule conditions (Conseq.Kind == ConseqRule
// / ConseqNested) and multi-rule conditions, the back-compat Conseq
// is left nil; back-compat consumers see "no annotation" and skip
// the function, which is the correct conservative behaviour for
// the downstream analyzer that walks Rules directly.
type Condition struct {
	// Rules is the current representation. Always populated with one or
	// more entries. Multi-rule (`;`-separated) payloads land here
	// as a multi-element slice so the analyzer can walk every rule.
	Rules []*Rule

	// Prereq is the back-compat parameter requirement for a single-
	// rule arrow-rule condition. Nil for atom-rules, higher-order
	// conseq arrow-rules, multi-rule conditions, and `vow:cond`
	// sugar (which represents the trivial wildcard `*` as a nil
	// Prereq — the same encoding the single-rule surface uses).
	Prereq *Prerequisite

	// Conseq is the back-compat return requirement for a single-rule
	// arrow-rule condition whose conseq is a plain value, or the
	// expanded body of an atom-rule whose target rule is an
	// EffectExpandPayload. Nil for arrow-rule conseqs that are
	// higher-order (ConseqRule / ConseqNested) and for multi-rule
	// conditions.
	Conseq *ReturnAnnotation

	// Subject names a chain of signature-scope identifiers the
	// condition applies to when the marker carried a `[X][Y]...`
	// scope qualifier. A single-element chain mirrors the
	// `vow:cond[b]` form that retargets the rule at parameter `b`;
	// a multi-element chain steps through nested higher-order
	// callbacks so `vow:cond[outer][inner]` retargets the rule at
	// the `inner` slot of the callback `outer` exposes. An empty
	// Subject means the marker carried no scope and the condition
	// applies to the enclosing function as usual.
	Subject SubjectChain
}

// SubjectChain carries the `[X][Y]...` scope qualifier as the
// chain of identifiers (or `$N` positional references) the marker
// walks through nested callback signatures. A nil or empty chain
// means the marker carried no scope.
type SubjectChain []string

// Prerequisite is a positional list of constraints over the
// function's arguments. Each position binds to the corresponding
// argument by signature order; argument names are not part of the
// surface form so renames flow through the function signature
// without annotation churn. Tuple-sum form is intentionally not
// supported here — multi-case prerequisites are expressed with
// case-style (one `vow:cond` line per case) rather than with
// `(...) | (...)` across positions.
type Prerequisite struct {
	Positions []PositionSpec
}

// String renders the condition back to its surface form. Multi-rule
// conditions print as `rule1 ; rule2 ; ...`. Single-rule conditions
// delegate to the rule's own String method, except for the trivial-
// wildcard sugar shape (a single arrow-rule with nil Prereq and a
// value conseq): that case renders as just the conseq so a
// `vow:cond` payload round-trips to its original surface form
// — `vow:cond error` prints as `error`, not as `* -> error`.
func (c Condition) String() string {
	if len(c.Rules) == 0 {
		return "<empty condition>"
	}
	if len(c.Rules) == 1 {
		r := c.Rules[0]
		if r != nil && r.Kind == RuleArrow && r.Arrow != nil &&
			r.Arrow.Prereq == nil && r.Arrow.Conseq != nil &&
			r.Arrow.Conseq.Kind == ConseqValue {
			return r.Arrow.Conseq.String()
		}
	}
	parts := seq.ChainOf(c.Rules...).Map((*Rule).String).ToSlice()
	return strings.Join(parts, " ; ")
}

// String renders a Prerequisite back to its surface form. Bare
// position lists join with ", "; each position uses its own
// PositionSpec.String rendering so single Terms and direct sums
// emerge in canonical form.
func (p Prerequisite) String() string {
	parts := seq.ChainOf(p.Positions...).Map(PositionSpec.String).ToSlice()
	return strings.Join(parts, ", ")
}

// renderReturnAnnotation prints a ReturnAnnotation in its
// position-wise or tuple-sum surface form. The helper lives here
// rather than as a method on ReturnAnnotation because both
// Condition and ArrowConseq consume it; promoting it to a method
// would invite callers that should be reading the AST directly.
func renderReturnAnnotation(a *ReturnAnnotation) string {
	if a.TupleSum != nil {
		return a.TupleSum.String()
	}
	parts := seq.ChainOf(a.Positions...).Map(PositionSpec.String).ToSlice()
	return strings.Join(parts, ", ")
}

// ParseConditionAnnotation parses a `vow:cond` payload. The
// current grammar accepts one or more rules separated by `;`:
//
//	vow:cond rule (';' rule)*
//
// Each rule is either an atom-rule (a bare rule reference such as
// `@result.OkErr[T, E]`) or an arrow-rule (`prereq -> conseq`).
// Atom-rules
// declare that the enclosing function's signature matches the
// referenced rule directly; arrow-rules carry the
// prereq/conseq case-style contract.
//
// In an arrow-rule, the prereq half rejects rule references (the
// current grammar disallows @rule in parameter requirement positions —
// only types and predicates are allowed there). The conseq half
// generalises beyond plain values to also accept a single rule-ref
// or a parenthesised condition, both of which describe a higher-
// order return (the function returns another function whose
// signature is the embedded rule / condition).
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func ParseConditionAnnotation(payload string) (*Condition, error) {
	parts, err := splitAtTopLevel(payload, ';')
	if err != nil {
		return nil, fmt.Errorf("vow:cond %q: %w", payload, err)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("vow:cond %q: empty payload: %w", payload, ErrSyntax)
	}
	rules := make([]*Rule, 0, len(parts))
	var prev *Rule
	for i, part := range parts {
		rule, err := parseRuleOrSugar(prev, part, i)
		if err != nil {
			return nil, fmt.Errorf("vow:cond %q: %w", payload, err)
		}
		rules = append(rules, rule)
		prev = rule
	}
	cond := &Condition{Rules: rules}
	populateLegacyFields(cond)
	return cond, nil
}

// parseRuleOrSugar dispatches one `;`-separated payload between the
// regular rule parser and the left-hand-side sugar expansion. A
// payload that starts with `=>` or `<=>` carries the sugar shape —
// the parser reuses the previous rule's left-hand side so authors
// can write `A => B ; => C` instead of repeating `A`. Sugar is only
// meaningful after a logical arrow rule; structural rules and
// atom-rules reject the sugar form so the diagnostic surfaces the
// invariant precisely at parse time.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func parseRuleOrSugar(prev *Rule, payload string, idx int) (*Rule, error) {
	trimmed := strings.TrimSpace(payload)
	if arrow, dir, ok := leadingSugarArrow(trimmed); ok {
		return expandSugarRule(prev, idx, arrow, dir, trimmed[len(arrow):])
	}
	return parseOneRule(payload)
}

// leadingSugarArrow reports whether the trimmed payload starts with
// a sugar arrow (`=>` or `<=>`) and returns the spelling together
// with the matching Direction. The `<=>` check runs first so a
// `<=>`-led payload is not misread as `<=` followed by `>`.
func leadingSugarArrow(trimmed string) (string, Direction, bool) {
	if strings.HasPrefix(trimmed, "<=>") {
		return "<=>", DirEquiv, true
	}
	if strings.HasPrefix(trimmed, "=>") {
		return "=>", DirImply, true
	}
	return "", 0, false
}

// expandSugarRule builds a new rule by carrying the left-hand side
// of the previous rule across the sugar arrow. The previous rule
// must be a logical arrow rule (`=>` or `<=>`); any other shape
// rejects so the surface stays unambiguous. Mixing arrow flavours
// within a sugar chain (`A => B ; <=> C`) is permitted at parse
// time because both arrows leave the left-hand side meaningful;
// readability is a separate concern that the parser does not
// enforce.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func expandSugarRule(prev *Rule, idx int, arrow string, dir Direction, rhsPayload string) (*Rule, error) {
	if prev == nil {
		return nil, fmt.Errorf("rule %d: sugar clause `%s ...` has no preceding rule to inherit a left-hand side from: %w", idx, arrow, ErrInvalidGrammar)
	}
	if prev.Kind != RuleArrow || prev.Arrow == nil {
		return nil, fmt.Errorf("rule %d: sugar clause `%s ...` cannot follow an atom-rule: %w", idx, arrow, ErrInvalidGrammar)
	}
	if prev.Arrow.Direction == DirStructural {
		return nil, fmt.Errorf("rule %d: sugar clause `%s ...` cannot follow a structural `->` rule; the sugar carry only applies to logical arrows: %w", idx, arrow, ErrInvalidGrammar)
	}
	if prev.Arrow.Left == nil {
		return nil, fmt.Errorf("rule %d: preceding logical rule is missing a left-hand side; cannot expand sugar `%s ...`: %w", idx, arrow, ErrSyntax)
	}
	rhs := strings.TrimSpace(rhsPayload)
	if rhs == "" {
		return nil, fmt.Errorf("rule %d: sugar clause `%s` has no right-hand side: %w", idx, arrow, ErrSyntax)
	}
	rhsToks, err := tokenize(rhs, lexCond)
	if err != nil {
		return nil, fmt.Errorf("rule %d sugar clause: %w", idx, err)
	}
	rightExpr, err := parseExpression(rhsToks)
	if err != nil {
		return nil, fmt.Errorf("rule %d sugar right-hand side: %w", idx, err)
	}
	rule := NewLogicalRule(prev.Arrow.Left, dir, rightExpr)
	rule.Arrow.SugarCarriedFromPrevious = true
	return rule, nil
}

// parseOneRule parses a single rule (atom or arrow) from its
// textual payload. The dispatch is driven by two surface markers:
// a payload that is wholly a single rule-ref is an atom-rule;
// otherwise the parser expects a top-level `->` and falls into the
// arrow-rule path.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func parseOneRule(payload string) (*Rule, error) {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return nil, fmt.Errorf("empty rule: %w", ErrSyntax)
	}
	if ref, ok := AsSingleRuleRef(trimmed); ok {
		return NewAtomRule(ref), nil
	}
	return parseArrowRulePayload(trimmed)
}

// parseArrowRulePayload parses a `prereq -> conseq` payload. The
// prereq half is tokenised through the standard pipeline and
// checked for any rule-ref position (rejected). The conseq half
// distinguishes three shapes: a single rule-ref (ConseqRule), a
// parenthesised nested condition (ConseqNested), and the value
// shape (ConseqValue).
//
// The whole payload tokenises in `lexCond` mode so the logical
// arrow path picks up the postfix and comparison tokens it needs.
// For the structural arrow path the conseq half re-tokenises in
// `lexReturn` mode so qualified type identifiers (`pkg.Name`,
// `pkg.Err`) stay atomic — the return-annotation grammar reads
// the qualified head as a single ident, while the cond-side
// lexer would split it into three tokens.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func parseArrowRulePayload(payload string) (*Rule, error) {
	toks, err := tokenize(payload, lexCond)
	if err != nil {
		return nil, err
	}
	arrowIdx, direction, err := findTopLevelDirection(toks)
	if err != nil {
		return nil, err
	}
	leftToks := toks[:arrowIdx]
	rightToks := toks[arrowIdx+1:]
	if direction != DirStructural {
		return parseLogicalRuleHalves(leftToks, rightToks, direction)
	}
	if len(leftToks) == 0 {
		return nil, fmt.Errorf("parameter requirement is empty (write `*` for the trivial parameter requirement): %w", ErrSyntax)
	}
	prereq, err := parsePrerequisiteTokens(leftToks)
	if err != nil {
		return nil, fmt.Errorf("parameter requirement: %w", err)
	}
	if err := rejectRuleRefInPrereq(prereq); err != nil {
		return nil, fmt.Errorf("parameter requirement: %w", err)
	}
	conseqText, err := arrowConseqText(payload, toks, arrowIdx)
	if err != nil {
		return nil, err
	}
	conseqToks, err := tokenize(conseqText, lexReturn)
	if err != nil {
		return nil, fmt.Errorf("return requirement: %w", err)
	}
	conseq, err := parseArrowConseqTokens(conseqToks)
	if err != nil {
		return nil, fmt.Errorf("return requirement: %w", err)
	}
	return NewArrowRule(normalisePrereq(prereq), conseq), nil
}

// arrowConseqText returns the substring of payload that sits to
// the right of the structural arrow at toks[arrowIdx]. The slice
// preserves the conseq half's source text so the re-tokenisation
// step reads the same characters the original parse saw.
//
// vow:cond * -> _, ErrSyntax | nil
func arrowConseqText(payload string, toks []token, arrowIdx int) (string, error) {
	if arrowIdx < 0 || arrowIdx >= len(toks) {
		return "", fmt.Errorf("arrow token index out of range: %w", ErrSyntax)
	}
	arrow := toks[arrowIdx]
	end := arrow.Pos + len(arrow.Val)
	if end > len(payload) {
		return "", fmt.Errorf("arrow token spans past the payload boundary: %w", ErrSyntax)
	}
	return payload[end:], nil
}

// parseLogicalRuleHalves parses the `=>` / `<=>` form. Each half
// must be a single Expression — a nil-check, a comparison, or a
// bare reference. Empty halves are rejected here rather than
// surfaced as cryptic Expression diagnostics so authors see a
// precise pointer to the missing operand.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func parseLogicalRuleHalves(leftToks, rightToks []token, direction Direction) (*Rule, error) {
	if len(stripTrailingEOF(leftToks)) == 0 {
		return nil, fmt.Errorf("left-hand side of `%s` is empty: %w", direction.String(), ErrSyntax)
	}
	if len(stripTrailingEOF(rightToks)) == 0 {
		return nil, fmt.Errorf("right-hand side of `%s` is empty: %w", direction.String(), ErrSyntax)
	}
	left, err := parseExpression(leftToks)
	if err != nil {
		return nil, fmt.Errorf("left-hand side: %w", err)
	}
	right, err := parseExpression(rightToks)
	if err != nil {
		return nil, fmt.Errorf("right-hand side: %w", err)
	}
	return NewLogicalRule(left, direction, right), nil
}

// rejectRuleRefInPrereq walks the parsed prereq and returns an
// error if any position contains a rule-ref. the current grammar forbids `@rule` in
// the parameter-requirement half — rules describe function-level
// shapes that do not fit a single argument position.
//
// vow:cond * -> ErrInvalidGrammar | nil
func rejectRuleRefInPrereq(prereq *Prerequisite) error {
	if prereq == nil {
		return nil
	}
	for i, pos := range prereq.Positions {
		if positionContainsRuleRef(pos) {
			return fmt.Errorf("position %d: @rule references are not allowed in parameter requirement positions: %w", i, ErrInvalidGrammar)
		}
	}
	return nil
}

// positionContainsRuleRef reports whether a parsed PositionSpec
// holds a rule-ref shape. The token-level lexer admits the raw
// `@alias.Name[args]` run as a single tIdent (scanIdent absorbs
// brackets), and parseIdentLed treats unknown idents as bare
// types, so the rule-ref ends up in Term.Type prefixed with `@`.
// This helper inspects the Term shape that drops out of that path
// to detect the rule-ref.
func positionContainsRuleRef(pos PositionSpec) bool {
	if pos.Single != nil {
		return termIsRuleRef(pos.Single)
	}
	if pos.Sum != nil {
		return slices.ContainsFunc(pos.Sum.Members, func(m SumMember) bool {
			return m.Term != nil && termIsRuleRef(m.Term)
		})
	}
	return false
}

func termIsRuleRef(t *Term) bool {
	if t == nil {
		return false
	}
	return strings.HasPrefix(t.Type, "@")
}

// normalisePrereq returns nil when the prereq is the trivial
// wildcard `*` (a single wildcard position with no type pin), so
// the back-compat Prereq=nil encoding is preserved through the
// rule-list parser. Non-trivial prereqs pass through unchanged.
func normalisePrereq(p *Prerequisite) *Prerequisite {
	if p == nil || len(p.Positions) != 1 {
		return p
	}
	single := p.Positions[0].Single
	if single == nil || !single.Wildcard || single.Type != "" || single.Pred != nil {
		return p
	}
	return nil
}

// parseArrowConseqTokens parses the conseq half of an arrow-rule.
// Three shapes are recognised in order:
//
//   - A parenthesised nested condition: `(rule (';' rule)*)`. The
//     enclosing parens are stripped and the inner payload is
//     parsed recursively as a Condition. Detected by a leading
//     tLParen whose matching tRParen ends the conseq AND whose
//     inner span carries a top-level `;`.
//   - A single rule-ref: a lone tIdent token whose value starts
//     with `@` consumes the entire conseq. Higher-order: the
//     function returns another function described by the rule.
//   - Otherwise: the value conseq parsed through the
//     standard precedence pipeline as a ReturnAnnotation.
//
// vow:cond * -> _, (ErrSyntax | _)?
func parseArrowConseqTokens(toks []token) (*ArrowConseq, error) {
	if len(toks) == 0 {
		return nil, fmt.Errorf("empty return requirement: %w", ErrSyntax)
	}
	if inner, ok, err := tryNestedCondition(toks); err != nil {
		return nil, err
	} else if ok {
		nested, err := ParseConditionAnnotation(inner)
		if err != nil {
			return nil, err
		}
		return NewNestedConseq(nested), nil
	}
	if ref, ok, err := singleRuleRefToken(toks); err != nil {
		return nil, err
	} else if ok {
		return NewRuleConseq(ref), nil
	}
	anno, err := parseAnnotationTokens(toks)
	if err != nil {
		return nil, err
	}
	return NewValueConseq(anno), nil
}

// tryNestedCondition reports whether the token stream is a
// parenthesised payload that should be parsed recursively as a
// nested condition. The detection criterion is:
//
//   - The stream starts with `(` and ends with `)` at matched
//     depth (no trailing tokens outside the parens).
//   - The inner span contains at least one top-level `;` (otherwise
//     `(A | B)?` and `(A, B) | (C, D)` paren shapes would be
//     misclassified as nested conditions).
//
// When detected, the inner textual payload (between the outermost
// parens) is returned so the caller can re-parse via
// ParseConditionAnnotation.
//
// vow:cond * -> _, _, ErrSyntax | nil
func tryNestedCondition(toks []token) (string, bool, error) {
	nonEOF := stripTrailingEOF(toks)
	if len(nonEOF) < 2 || nonEOF[0].Kind != tLParen {
		return "", false, nil
	}
	depth := 0
	closeIdx := -1
	for i, t := range nonEOF {
		switch t.Kind {
		case tLParen:
			depth++
		case tRParen:
			depth--
			if depth == 0 {
				closeIdx = i
			}
		}
		if closeIdx >= 0 {
			break
		}
	}
	if closeIdx < 0 {
		return "", false, fmt.Errorf("unbalanced `(` in conseq: %w", ErrSyntax)
	}
	if closeIdx != len(nonEOF)-1 {
		return "", false, nil
	}
	innerToks := nonEOF[1:closeIdx]
	// A nested-condition shape is only inferred when the inner span
	// carries a top-level `;`. An empty inner span has no `;`, so it
	// naturally falls through to the value-conseq parser, which will
	// emit its own "empty conseq" diagnostic — no special-casing
	// needed here.
	if !innerHasTopLevelSemicolon(innerToks) {
		return "", false, nil
	}
	return tokenSpan(innerToks), true, nil
}

// innerHasTopLevelSemicolon reports whether the token slice contains
// a tSemicolon at depth 0 (outside any further parens).
func innerHasTopLevelSemicolon(toks []token) bool {
	depth := 0
	for _, t := range toks {
		switch t.Kind {
		case tLParen:
			depth++
		case tRParen:
			depth--
		case tSemicolon:
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// tokenSpan reconstructs the textual span covering the supplied
// tokens by joining their values with single spaces. The result is
// re-parseable through the standard tokeniser because every value
// already round-trips and whitespace between tokens is
// insignificant.
func tokenSpan(toks []token) string {
	parts := seq.ChainOf(toks...).
		FilterMap(func(t token) (string, bool) {
			return t.Val, t.Kind != tEOF
		}).
		ToSlice()
	return strings.Join(parts, " ")
}

// singleRuleRefToken reports whether the token slice is exactly one
// rule-ref token (a tIdent whose value starts with `@` and includes
// matched `[...]` brackets) and surfaces any parse error from the
// rule-ref tokeniser so a malformed `@bad[...` payload becomes a
// targeted diagnostic instead of silently falling through to the
// value-conseq parser (which would report a confusing
// "unexpected token" error against the same input).
//
// Returns:
//   - (ref, true, nil)  — well-formed single rule-ref.
//   - (nil, false, nil) — the token slice is not a rule-ref shape
//     (e.g. multiple tokens, or a non-`@`-prefixed leading token).
//   - (nil, false, err) — the slice looked like a rule-ref (leading
//     `@`) but the rule-ref tokeniser rejected the body.
//
// vow:cond * -> _, _, (ErrSyntax | _)?
func singleRuleRefToken(toks []token) (*RuleRef, bool, error) {
	nonEOF := stripTrailingEOF(toks)
	if len(nonEOF) != 1 {
		return nil, false, nil
	}
	t := nonEOF[0]
	if t.Kind != tIdent || !strings.HasPrefix(t.Val, "@") {
		return nil, false, nil
	}
	ref, consumed, err := parseRuleRefAt(t.Val)
	if err != nil {
		return nil, false, err
	}
	if consumed != len(t.Val) {
		return nil, false, fmt.Errorf("trailing text after rule-ref %q: %w", t.Val, ErrSyntax)
	}
	return &ref, true, nil
}

// AsSingleRuleRef reports whether payload is exactly one rule-ref
// (`@alias.Name[args]` with no surrounding tokens) and returns the
// parsed reference when so. Used by both the atom-rule
// detection in the dsl parser and the higher-order `vow:cond`
// dispatch in the analyzer; centralising the recognition here
// keeps the two surfaces consistent.
func AsSingleRuleRef(payload string) (*RuleRef, bool) {
	trimmed := strings.TrimSpace(payload)
	if !strings.HasPrefix(trimmed, "@") {
		return nil, false
	}
	ref, consumed, err := parseRuleRefAt(trimmed)
	if err != nil {
		return nil, false
	}
	if consumed != len(trimmed) {
		return nil, false
	}
	return &ref, true
}

// WrapAtomRuleAsCondition lifts a `vow:cond @rule[args]`
// payload into a single-rule atom-rule Condition. The expanded
// rule body (parsed back into a ReturnAnnotation by the caller) is
// stored on the Conseq field so the back-compat analyzer keeps
// checking the function against the expanded sum, preserving the
// behaviour the existing `vow:cond @result.OkErr[...]` tests
// (atom-rule form) depend on. The atom-rule
// itself is recorded on Rules so downstream passes dispatch on
// the rule reference's Effect kind.
func WrapAtomRuleAsCondition(ref *RuleRef, expandedBody *ReturnAnnotation) *Condition {
	if ref == nil {
		return nil
	}
	rule := NewAtomRule(ref)
	return &Condition{Rules: []*Rule{rule}, Conseq: expandedBody}
}

// populateLegacyFields fills in the back-compat Prereq/Conseq
// fields on a Condition whose Rules holds exactly one arrow-rule
// with a value conseq. Multi-rule, atom-rule, and higher-order
// conseq conditions leave the back-compat fields nil so back-compat
// consumers conservatively skip the function (the correct fallback
// while the analyzer uses walking Rules).
func populateLegacyFields(cond *Condition) {
	if cond == nil || len(cond.Rules) != 1 {
		return
	}
	r := cond.Rules[0]
	if r.Kind != RuleArrow || r.Arrow == nil {
		return
	}
	if r.Arrow.Conseq == nil || r.Arrow.Conseq.Kind != ConseqValue {
		return
	}
	cond.Prereq = r.Arrow.Prereq
	cond.Conseq = r.Arrow.Conseq.Value
}

// findTopLevelDirection scans the token stream for the first
// top-level arrow of any flavour (`->`, `=>`, or `<=>`) and reports
// the matching Direction. Exactly one top-level arrow must be
// present; mixing two flavours (e.g. `A -> B => C`) or repeating
// one (e.g. `A => B => C`) surfaces as ErrInvalidGrammar so authors
// see a precise diagnostic instead of a downstream parse failure.
// Arrows inside parens are ignored — nesting cannot split a rule.
//
// vow:cond * -> _, _, ErrInvalidGrammar | nil
func findTopLevelDirection(toks []token) (int, Direction, error) {
	depth := 0
	arrowIdx := -1
	var direction Direction
	for i, t := range toks {
		switch t.Kind {
		case tLParen:
			depth++
		case tRParen:
			depth--
		case tArrow, tImply, tEquiv:
			if depth != 0 {
				continue
			}
			candidate := arrowDirection(t.Kind)
			if arrowIdx >= 0 {
				if direction == candidate {
					return 0, 0, fmt.Errorf("multiple top-level `%s` separators: %w", t.Val, ErrInvalidGrammar)
				}
				return 0, 0, fmt.Errorf("mixed arrow forms (`%s` and `%s`) at the top level; one rule must use a single arrow flavour: %w", direction.String(), t.Val, ErrInvalidGrammar)
			}
			arrowIdx = i
			direction = candidate
		}
	}
	if arrowIdx < 0 {
		return 0, 0, fmt.Errorf("missing arrow separator (`->`, `=>`, or `<=>`) between rule operands: %w", ErrInvalidGrammar)
	}
	return arrowIdx, direction, nil
}

// arrowDirection maps an arrow token kind to its Direction.
func arrowDirection(k tokKind) Direction {
	switch k {
	case tImply:
		return DirImply
	case tEquiv:
		return DirEquiv
	}
	return DirStructural
}

// parseAnnotationTokens runs the precedence-based parser over a
// pre-tokenised slice. The implementation mirrors
// parseReturnAnnotationNew's body but skips the tokenize step so a
// caller that already owns the token stream (e.g. the Condition
// parser splitting on `->`) can re-enter the recursive descent
// without retokenising.
//
// vow:cond * -> _, ErrSyntax | nil
func parseAnnotationTokens(toks []token) (*ReturnAnnotation, error) {
	if len(toks) == 0 || toks[0].Kind == tEOF {
		return nil, fmt.Errorf("empty annotation: %w", ErrSyntax)
	}
	withEOF := ensureEOFTerminator(toks)
	p := parser{toks: withEOF}
	return p.parseAnnotation()
}

// parsePrerequisiteTokens parses the parameter-requirement half.
// The resulting AST is checked for tuple-sum form (which is
// rejected per the surface design) before being unwrapped into a
// Prerequisite. The single-position trivial wildcard `*` is the
// idiomatic way to author "no constraint" in the surface form;
// the parser dispatches the `*` token through the dedicated
// wildcard entry that the complete-wildcard surface defines. A
// bare `_` (label-aware placeholder) round-trips through the
// same parser path when the wildcard surface is not authored, so
// existing annotations continue to parse without churn.
//
// A bare `nil` token in a parameter-requirement position is
// rewritten into the `Nil` predicate. The shared expression
// parser treats `nil` without a trailing type as the nil literal
// (the form `vow:cond * -> ErrFoo | nil` carries nil as an
// authorised return value), but a parameter-requirement position
// constrains a parameter, so the literal interpretation would
// mean "this parameter equals nil" — exactly what the predicate
// already encodes. Promoting the literal here keeps the surface
// uniform between predicates that always parse as predicates
// (`nonnil`, `zero`, `nonzero`) and `nil`, so an author can drop the
// documentation-only parameter name from every shape
// consistently.
//
// vow:cond * -> _, (ErrInvalidGrammar | _)?
func parsePrerequisiteTokens(toks []token) (*Prerequisite, error) {
	anno, err := parseAnnotationTokens(toks)
	if err != nil {
		return nil, err
	}
	if anno.TupleSum != nil {
		return nil, fmt.Errorf("tuple-sum form is not supported in a parameter requirement; use case-style (one vow:cond line per case) instead: %w", ErrInvalidGrammar)
	}
	positions := seq.ChainOf(anno.Positions...).Map(promoteBareNilToPredicate).ToSlice()
	return &Prerequisite{Positions: positions}, nil
}

// promoteBareNilToPredicate rewrites a `nil`-literal singleton
// position into a Nil-predicate position. The literal shape is
// what parseIdentLed produces for a bare `nil` token; the
// predicate shape is what every other parameter-requirement
// position carries. Any other shape passes through untouched, so
// the rewrite touches only the surface ambiguity it was designed
// for.
func promoteBareNilToPredicate(pos PositionSpec) PositionSpec {
	if pos.Single != nil || pos.Sum == nil {
		return pos
	}
	if len(pos.Sum.Members) != 1 {
		return pos
	}
	m := pos.Sum.Members[0]
	if m.Literal == nil || m.Literal.Kind != LitNil {
		return pos
	}
	if pos.Sum.Qualifier != QualifierNone {
		return pos
	}
	return PositionSpec{Single: &Term{Pred: Nil{}}}
}

// ensureEOFTerminator returns toks unchanged when it already ends
// with tEOF; otherwise it appends one. The recursive-descent
// parser relies on tEOF to bound its lookahead, so a sub-slice
// (e.g. the parameter-requirement half of a `->` split) must be
// terminated explicitly before re-entering the parser.
func ensureEOFTerminator(toks []token) []token {
	if len(toks) > 0 && toks[len(toks)-1].Kind == tEOF {
		return toks
	}
	out := make([]token, len(toks)+1)
	copy(out, toks)
	pos := 0
	if len(toks) > 0 {
		pos = toks[len(toks)-1].Pos + len(toks[len(toks)-1].Val)
	}
	out[len(toks)] = token{Kind: tEOF, Pos: pos}
	return out
}

// stripTrailingEOF returns toks without any trailing tEOF entries.
func stripTrailingEOF(toks []token) []token {
	for len(toks) > 0 && toks[len(toks)-1].Kind == tEOF {
		toks = toks[:len(toks)-1]
	}
	return toks
}

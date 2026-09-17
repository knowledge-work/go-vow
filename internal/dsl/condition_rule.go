package dsl

import "strings"

// Rule is one element of a Condition's rule list in the current grammar:
//
//	condition := rule (';' rule)*
//	rule      := atom-rule | arrow-rule
//
// Exactly one of Atom or Arrow is non-nil; Kind discriminates the
// union. An atom-rule (single rule-ref) declares that the enclosing
// function's signature matches the named rule directly; an
// arrow-rule (prereq -> conseq) is the case-style contract that
// existed before the current grammar.
type Rule struct {
	Kind  RuleKind
	Atom  *RuleRef   // RuleAtom: the function is described by this rule reference.
	Arrow *ArrowRule // RuleArrow: prereq/conseq pair.
}

// RuleKind enumerates the two rule shapes.
type RuleKind int

const (
	// RuleAtom marks a rule that is a single rule-ref. The
	// reference targets the enclosing function: "this function's
	// signature is the @rule expansion".
	RuleAtom RuleKind = iota

	// RuleArrow marks a rule that pairs a parameter requirement
	// with a return requirement.
	RuleArrow
)

// String renders the rule back to its surface form. Atom-rules
// print as the bare rule reference; arrow-rules print as
// `prereq -> conseq` (matching the Condition.String shape).
func (r *Rule) String() string {
	if r == nil {
		return "<nil rule>"
	}
	switch r.Kind {
	case RuleAtom:
		if r.Atom == nil {
			return "<atom-rule with no ref>"
		}
		return "@" + r.Atom.qualifiedName() + ruleRefArgsString(r.Atom)
	case RuleArrow:
		if r.Arrow == nil {
			return "<arrow-rule with no body>"
		}
		return r.Arrow.String()
	}
	return "<unknown rule kind>"
}

// ruleRefArgsString renders the bracketed type-args of a RuleRef.
// A zero-param rule (TypeArgs empty) still prints the `[]` so the
// surface form stays uniform with parameterised references — the
// brackets are the marker that distinguishes a rule from a bare
// type identifier.
func ruleRefArgsString(r *RuleRef) string {
	if r == nil {
		return "[]"
	}
	return "[" + strings.Join(r.TypeArgs, ", ") + "]"
}

// ArrowRule carries the two halves of an arrow-rule. The structural
// form (`->`) keeps the Prereq / Conseq pair so consumers
// that drove the previous grammar see no shape change. The logical
// forms (`=>` and `<=>`) populate Left / Right with Expression
// operands so consumers that handle the expanded grammar fan out via
// the Direction discriminator.
//
// Direction picks which slot pair is meaningful:
//
//   - DirStructural: Prereq + Conseq are set; Left / Right are nil.
//   - DirImply, DirEquiv: Left + Right are set; Prereq / Conseq are
//     nil.
type ArrowRule struct {
	Direction Direction

	// Structural form slots (DirStructural).
	Prereq *Prerequisite // nil signals the trivial wildcard `*`.
	Conseq *ArrowConseq

	// Logical form slots (DirImply, DirEquiv).
	Left  *Expression
	Right *Expression

	// SugarCarriedFromPrevious marks a logical rule whose left-hand
	// side was elided in source (`; => rhs`) and reconstructed by
	// the parser from the preceding rule.
	SugarCarriedFromPrevious bool
}

// String renders the arrow-rule. The structural form preserves the
// `prereq -> conseq` rendering — including the trivial wildcard
// prereq printed as `*` so `vow:cond X` payloads round-trip —
// while the logical forms render their operands separated by the
// matching arrow spelling.
func (a *ArrowRule) String() string {
	if a == nil {
		return "<nil arrow-rule>"
	}
	switch a.Direction {
	case DirImply, DirEquiv:
		left := "<nil lhs>"
		if a.Left != nil {
			left = a.Left.String()
		}
		right := "<nil rhs>"
		if a.Right != nil {
			right = a.Right.String()
		}
		return left + " " + a.Direction.String() + " " + right
	}
	prereq := "*"
	if a.Prereq != nil {
		prereq = a.Prereq.String()
	}
	conseq := "<nil conseq>"
	if a.Conseq != nil {
		conseq = a.Conseq.String()
	}
	return prereq + " -> " + conseq
}

// ArrowConseq is the conseq half of an arrow-rule. It generalises
// the `*ReturnAnnotation` shape with two additional shapes used by
// higher-order declarations:
//
//   - ConseqValue:  a plain return-requirement (the value shape).
//   - ConseqRule:   a single rule-ref describing the *returned*
//     function's signature (higher-order).
//   - ConseqNested: a `(condition)` embed for higher-order returns
//     whose contract spans multiple rules.
//
// Exactly one of Value, Rule, or Nested is non-nil; Kind selects.
type ArrowConseq struct {
	Kind   ConseqKind
	Value  *ReturnAnnotation // ConseqValue
	Rule   *RuleRef          // ConseqRule
	Nested *Condition        // ConseqNested
}

// ConseqKind enumerates the conseq shapes.
type ConseqKind int

const (
	// ConseqValue marks a plain return-requirement conseq —
	// types, predicates, sums, literals.
	ConseqValue ConseqKind = iota

	// ConseqRule marks a higher-order conseq: the function returns
	// another function whose signature matches the referenced rule.
	ConseqRule

	// ConseqNested marks a higher-order conseq whose contract is a
	// `(condition)` embed — a multi-rule signature on the returned
	// function.
	ConseqNested
)

// String renders the conseq back to its surface form.
func (c *ArrowConseq) String() string {
	if c == nil {
		return "<nil conseq>"
	}
	switch c.Kind {
	case ConseqValue:
		if c.Value == nil {
			return "<empty value conseq>"
		}
		return renderReturnAnnotation(c.Value)
	case ConseqRule:
		if c.Rule == nil {
			return "<conseq rule-ref with no ref>"
		}
		return "@" + c.Rule.qualifiedName() + ruleRefArgsString(c.Rule)
	case ConseqNested:
		if c.Nested == nil {
			return "<empty nested conseq>"
		}
		return "(" + c.Nested.String() + ")"
	}
	return "<unknown conseq kind>"
}

// NewAtomRule builds an atom-rule from a rule reference.
func NewAtomRule(ref *RuleRef) *Rule {
	return &Rule{Kind: RuleAtom, Atom: ref}
}

// NewArrowRule builds a structural arrow-rule from a prereq and a
// conseq. A nil prereq encodes the trivial wildcard `*` — the same
// shape the `vow:cond` sugar produces. Direction is set to
// DirStructural so consumers that switch on the discriminator pick
// the structural path.
func NewArrowRule(prereq *Prerequisite, conseq *ArrowConseq) *Rule {
	return &Rule{Kind: RuleArrow, Arrow: &ArrowRule{
		Direction: DirStructural,
		Prereq:    prereq,
		Conseq:    conseq,
	}}
}

// NewLogicalRule builds a logical arrow-rule (`=>` or `<=>`). The
// direction must be DirImply or DirEquiv; DirStructural is rejected
// here because it has the dedicated NewArrowRule constructor.
func NewLogicalRule(left *Expression, dir Direction, right *Expression) *Rule {
	return &Rule{Kind: RuleArrow, Arrow: &ArrowRule{
		Direction: dir,
		Left:      left,
		Right:     right,
	}}
}

// NewValueConseq wraps a ReturnAnnotation as a ConseqValue.
func NewValueConseq(anno *ReturnAnnotation) *ArrowConseq {
	return &ArrowConseq{Kind: ConseqValue, Value: anno}
}

// NewRuleConseq wraps a rule reference as a ConseqRule
// (higher-order: the function returns a function described by the
// rule).
func NewRuleConseq(ref *RuleRef) *ArrowConseq {
	return &ArrowConseq{Kind: ConseqRule, Rule: ref}
}

// NewNestedConseq wraps a nested condition as a ConseqNested
// (higher-order: the function returns a function whose contract is
// the embedded multi-rule condition).
func NewNestedConseq(inner *Condition) *ArrowConseq {
	return &ArrowConseq{Kind: ConseqNested, Nested: inner}
}

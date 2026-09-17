package dsl

import (
	"fmt"
	"strings"
)

// Predicate is the value-level constraint side of a Term. It is the
// AST representation of the predicate sub-language a Term can carry
// in addition to (or instead of) a Go type. Concrete shapes are
// Zero, Nil, Not, Literal, and SentinelRef.
//
// The wildcard / fall-through role is carried by the `_` placeholder
// on Term itself (Term.Placeholder = true) rather than by a separate
// predicate value. `_` is label-aware: it matches values that are
// not selected as tracked subjects, so a sum like `ErrFoo | _` lets
// a non-subject error fall through while still leaking unlisted
// subjects.
//
// Predicates compose with types via the surface syntax `<pred>
// <type>` (predicate-first, type-last — the predicate modifies the
// type the same way an adjective modifies a noun). A bare Predicate
// without a type stands in tuple positions where the value is
// constrained but its Go type is inferred from the surrounding
// signature.
type Predicate interface {
	// String renders the predicate in DSL surface form. The output
	// is intended to round-trip through ParseAnnotation so canonical
	// rendering tests can verify the parser preserves intent.
	String() string

	// isPredicate is an unexported marker preventing external
	// implementations; every concrete Predicate type defined in
	// this package satisfies it.
	isPredicate()
}

// Zero is the predicate that matches the type's zero value
// (int(0), "", false, the nil interface, the nil pointer, an
// empty composite literal). It is the canonical form for
// describing "is the default state".
type Zero struct{}

func (Zero) String() string { return "zero" }
func (Zero) isPredicate()   {}

// Nil is the predicate that matches the nil literal. It is
// meaningful only for nillable kinds (pointer, interface, slice,
// map, channel, function, error). The parser rejects `nil` /
// `nonnil` against a value-type Term so an int-typed position cannot
// accidentally claim a nil constraint.
type Nil struct{}

func (Nil) String() string { return "nil" }
func (Nil) isPredicate()   {}

// Not negates an inner predicate. The DSL surface uses `!P` as the
// prefix form; `!!P` (double negation) is also accepted at parse
// time and simplified to P in the surface renderer.
type Not struct {
	Inner Predicate
}

func (n Not) String() string {
	switch n.Inner.(type) {
	case Zero:
		return "nonzero"
	case Nil:
		return "nonnil"
	case nil:
		return "!"
	}
	return "!" + n.Inner.String()
}
func (Not) isPredicate() {}

// Literal is a constant value the position must equal. The Kind
// and Raw fields mirror LiteralTerm so Predicate's literal form
// shares storage with the existing direct-sum literal members; the
// rule engine compares Raw through go/constant for kind-aware
// equality.
type Literal struct {
	Kind LiteralKind
	Raw  string
}

func (l Literal) String() string { return l.Raw }
func (Literal) isPredicate()     {}

// SentinelRef names a tracked subject identifier or its qualified
// form (`pkg.ErrFoo`). It is the predicate shape produced when an
// annotation lists a specific sentinel by name (`ErrNotFound` in
// `vow:cond * -> ErrNotFound | nil`). The Name field carries the
// textual identifier as written at the call site; rule engines
// resolve it against the pass-wide subject registry at match time.
type SentinelRef struct {
	Name string
}

func (s SentinelRef) String() string { return s.Name }
func (SentinelRef) isPredicate()     {}

// NewLiteral constructs a Literal predicate from a textual literal,
// returning an error if the text does not parse as a recognised
// literal kind (nil / bool / int / string).
//
// vow:cond * -> _, ErrUnknownReference | nil
func NewLiteral(raw string) (Literal, error) {
	lit, ok := parseLiteral(raw)
	if !ok {
		return Literal{}, fmt.Errorf("predicate: %q is not a recognised literal (expected nil, true/false, int, or quoted string): %w", raw, ErrUnknownReference)
	}
	return Literal{Kind: lit.Kind, Raw: lit.Raw}, nil
}

// PredicateEquals reports whether two predicates are structurally
// identical. Used by tests and by the rule engine when comparing a
// declared predicate against a derived one (e.g. a predicate built
// from `nonzero T` should compare equal to a directly authored
// `nonzero T`).
func PredicateEquals(a, b Predicate) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch av := a.(type) {
	case Zero:
		_, ok := b.(Zero)
		return ok
	case Nil:
		_, ok := b.(Nil)
		return ok
	case Not:
		bv, ok := b.(Not)
		return ok && PredicateEquals(av.Inner, bv.Inner)
	case Literal:
		bv, ok := b.(Literal)
		return ok && av.Kind == bv.Kind && av.Raw == bv.Raw
	case SentinelRef:
		bv, ok := b.(SentinelRef)
		return ok && av.Name == bv.Name
	}
	return false
}

// SimplifyNot collapses double negation (`!!P` → `P`) and removes
// `Not{Inner: nil}` placeholders. The simplification is shallow:
// the inner predicate is not further normalised. Used by the
// parser after recognising a `!`-prefix to keep the AST canonical.
func SimplifyNot(p Predicate) Predicate {
	if p == nil {
		return nil
	}
	outer, ok := p.(Not)
	if !ok {
		return p
	}
	if outer.Inner == nil {
		return nil
	}
	inner, ok := outer.Inner.(Not)
	if !ok {
		return outer
	}
	if inner.Inner == nil {
		return nil
	}
	return SimplifyNot(inner.Inner)
}

// predicateRender is a small helper used by upstream Term.String to
// emit a predicate followed by an optional space-separated type.
// Centralising the join keeps the spacing rule ("predicate, single
// space, type") consistent across Term and Predicate renderings.
func predicateRender(pred Predicate, typ string) string {
	switch {
	case pred == nil && typ == "":
		return ""
	case pred == nil:
		return typ
	case typ == "":
		return pred.String()
	}
	if strings.HasPrefix(typ, "(") {
		return pred.String() + typ
	}
	return pred.String() + " " + typ
}

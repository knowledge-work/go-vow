package dsl

import (
	"fmt"
	"strings"
)

// Qualifier marks a DirectSum as optional. The `?` suffix is the
// only qualifier; the non-zero invariant is spelt as the
// `nonzero T` prefix predicate instead.
//
// The meanings are:
//
//   - QualifierNone     — the position admits exactly the listed
//     alternatives.
//   - QualifierOptional — `(...)?`, and the bare `T?` shorthand
//     which is the 1-member sum `(T)?`. The position also admits
//     the nil/zero state. Only meaningful for nillable kinds
//     (pointer, interface, slice, map, chan, func, error).
type Qualifier int

const (
	QualifierNone Qualifier = iota
	QualifierOptional
)

// String renders a Qualifier as its DSL suffix.
func (q Qualifier) String() string {
	if q == QualifierOptional {
		return "?"
	}
	return ""
}

// Term is a single element of a DSL signature. It is one of:
//
//   - a named Go type (`*int`, `error`, `Result`),
//   - a positional placeholder (`_`),
//   - a complete wildcard (`*`),
//   - a bare value-level predicate (`nonzero`, `nil`, ...),
//   - a predicate constraining a type (`nonzero Result`, `nil error`).
//
// The value-level constraint is carried by Pred. The canonical
// spelling for a non-zero predicate is the prefix form `nonzero T`;
// the parser rejects the `T!` suffix and points authors at the
// prefix form. The optional `?` suffix is not a Term concern: a
// `T?` position is the 1-member Optional DirectSum `(T)?`, so the
// nil admission lives on DirectSum.Qualifier rather than on the
// Term.
//
// Term values are produced by ParseTerm. Construct them directly only
// when wiring tests — the parser owns the responsibility of rejecting
// ill-formed combinations (e.g. a value type with the `?` suffix).
type Term struct {
	// Type is the textual Go type. Empty when Placeholder or
	// Wildcard is true and the author did not pin the position to a
	// type, or when the term is a bare predicate (`nonzero` with no
	// following type).
	Type string
	// Placeholder is true for the `_` form, used to mark positional
	// holes in signatures (e.g. `errors.Is(_, _)`). The placeholder
	// is label-aware: it matches values that are NOT selected as
	// tracked subjects, leaving labelled subjects to be listed
	// explicitly elsewhere in the sum.
	Placeholder bool
	// Wildcard is true for the `*` form, the complete wildcard that
	// matches every value at a position — including labelled
	// subjects that `_` would fall-through past. `*` is the trivial
	// parameter requirement and the return-requirement form for
	// boundaries that accept "anything goes here".
	Wildcard bool
	// Pred is the value-level predicate carried by the term. Set by
	// the prefix syntax (`nonzero T`, `nil`, `nonnil`). Nil when the
	// term has no predicate constraint.
	Pred Predicate
}

// String renders a Term back to its DSL surface form. The
// predicate-then-type form ("nonzero Result") is the canonical
// spelling for Pred-bearing terms; a bare type, placeholder, or
// wildcard is rendered as just the base. A wildcard or placeholder
// pinned to a type renders as `* error` / `_ error`.
func (t Term) String() string {
	base := t.Type
	switch {
	case t.Placeholder:
		base = "_"
	case t.Wildcard && base != "":
		base = "* " + base
	case t.Wildcard:
		base = "*"
	}
	if t.Pred != nil {
		base = predicateRender(t.Pred, base)
	}
	return base
}

// IsValid reports whether the Term satisfies its construction
// invariant: placeholder and wildcard are mutually exclusive, the
// placeholder form must not carry a Type, and every other form
// must carry at least one of Type or Pred. The wildcard form may
// be pinned to a Type (`* error`) so the analyzer's type axis can
// participate in matching; the placeholder form keeps its
// Type="" invariant because the existing grammar does not author
// `_ T`.
func (t Term) IsValid() bool {
	if t.Placeholder && t.Wildcard {
		return false
	}
	if t.Placeholder {
		return t.Type == ""
	}
	if t.Wildcard {
		return true
	}
	return t.Type != "" || t.Pred != nil
}

// ParseTerm parses a textual term (`*int`, `error?`, `_`,
// `Result`, ...) into a Term. It rejects ill-formed combinations:
// a qualifier with no base, `?` applied to obvious value types
// (int, string, bool, ...), and the `T!` suffix (spell the
// predicate as `nonzero T` through the prefix-predicate surface
// that ParsePresetBody accepts).
//
// The `?` suffix is validated (value types are rejected) but not
// recorded on the Term: a `T?` position is the 1-member Optional
// DirectSum reached through ParsePresetBody, so the optional
// admission lives on DirectSum.Qualifier, not on the Term.
//
// ParseTerm does not warn about "bare" terms — that policy decision is
// owned by ValidateTerm so callers can configure strictness.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar)?
func ParseTerm(s string) (Term, error) {
	raw := s
	s = strings.TrimSpace(s)
	if s == "" {
		return Term{}, fmt.Errorf("empty term: %w", ErrSyntax)
	}

	q, base := splitQualifier(s)
	base = strings.TrimSpace(base)
	if base == "" {
		return Term{}, fmt.Errorf("term %q has a qualifier without a base type: %w", raw, ErrSyntax)
	}

	if strings.HasSuffix(base, "?") {
		return Term{}, fmt.Errorf("term %q: qualifiers cannot stack: %w", raw, ErrInvalidGrammar)
	}

	if strings.HasSuffix(base, "!") {
		if q != QualifierNone {
			return Term{}, fmt.Errorf("term %q: qualifiers cannot stack: %w", raw, ErrInvalidGrammar)
		}
		trimmed := strings.TrimSuffix(base, "!")
		if trimmed == "" {
			return Term{}, fmt.Errorf("term %q has a qualifier without a base type: %w", raw, ErrSyntax)
		}
		if strings.HasSuffix(trimmed, "?") || strings.HasSuffix(trimmed, "!") {
			return Term{}, fmt.Errorf("term %q: qualifiers cannot stack: %w", raw, ErrInvalidGrammar)
		}
		return Term{}, fmt.Errorf("term %q: spell the predicate as `nonzero %s` instead of the `T!` suffix: %w", raw, trimmed, ErrInvalidGrammar)
	}

	if base == "_" {
		return Term{Placeholder: true}, nil
	}

	if q == QualifierOptional && isObviouslyValueType(base) {
		return Term{}, fmt.Errorf("term %q: value type %q cannot use ? (Optional): %w", raw, base, ErrInvalidGrammar)
	}

	return Term{Type: base}, nil
}

// splitQualifier strips the longest valid qualifier suffix from s and
// returns the corresponding Qualifier and the remaining base. The base
// is returned unchanged when no suffix is present.
func splitQualifier(s string) (Qualifier, string) {
	switch {
	case strings.HasSuffix(s, "?"):
		return QualifierOptional, strings.TrimSuffix(s, "?")
	default:
		return QualifierNone, s
	}
}

// isObviouslyValueType reports whether the textual type is one of the
// predeclared Go primitive kinds that can never be nil. It is a
// syntactic, conservative check — named types (`Result`, `MyInt`) are
// not classified as value types here, because the parser cannot
// resolve them without a Go type system. Such names accept any
// qualifier; the rule engine will perform the deeper check.
func isObviouslyValueType(s string) bool {
	switch s {
	case "bool",
		"byte",
		"complex64", "complex128",
		"float32", "float64",
		"int", "int8", "int16", "int32", "int64",
		"rune",
		"string",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	}
	return false
}

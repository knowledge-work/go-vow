package dsl

import (
	"fmt"
	"strconv"
	"strings"
)

// LiteralKind classifies a value-literal member of a direct sum.
type LiteralKind int

const (
	LitNil    LiteralKind = iota // `nil`
	LitBool                      // `true` / `false`
	LitInt                       // `1`, `-2`, `0xff`, ...
	LitString                    // `"foo"`
)

// String renders the LiteralKind name.
func (k LiteralKind) String() string {
	switch k {
	case LitNil:
		return "nil"
	case LitBool:
		return "bool"
	case LitInt:
		return "int"
	case LitString:
		return "string"
	default:
		return fmt.Sprintf("LiteralKind(%d)", int(k))
	}
}

// LiteralTerm is a value-literal member of a direct sum or tuple
// element. The raw textual form is preserved so it round-trips
// through String(); analyzers convert it to a Go constant value when
// matching against a return expression.
type LiteralTerm struct {
	Kind LiteralKind
	Raw  string
}

// String returns the literal's raw textual form.
func (l LiteralTerm) String() string {
	return l.Raw
}

// parseLiteral tries to interpret s as a value literal and returns
// (lit, true) when it succeeds. Recognized forms:
//
//   - `nil`               → LitNil
//   - `true` / `false`    → LitBool
//   - decimal / hex / octal / binary integers (optional sign) → LitInt
//   - double-quoted string literals                             → LitString
//
// String escapes are not interpreted here — the raw text is kept and
// the analyzer relies on `go/constant` when matching against actual
// return expressions.
func parseLiteral(s string) (*LiteralTerm, bool) {
	s = strings.TrimSpace(s)
	switch s {
	case "nil":
		return &LiteralTerm{Kind: LitNil, Raw: "nil"}, true
	case "true", "false":
		return &LiteralTerm{Kind: LitBool, Raw: s}, true
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return &LiteralTerm{Kind: LitString, Raw: s}, true
	}
	if isIntegerLiteral(s) {
		return &LiteralTerm{Kind: LitInt, Raw: s}, true
	}
	return nil, false
}

func isIntegerLiteral(s string) bool {
	if s == "" {
		return false
	}
	rest := s
	if rest[0] == '-' || rest[0] == '+' {
		rest = rest[1:]
	}
	if rest == "" {
		return false
	}
	_, err := strconv.ParseInt(rest, 0, 64)
	return err == nil
}

// ReturnAnnotation is the parsed form of a single `// vow:cond ...`
// line on a function declaration.
//
// There are two surface forms:
//
//   - Position-wise. Each element of Positions describes one return
//     position of the function, left to right:
//
//     vow:cond * -> Result!, (ErrFoo | ErrBar)?
//
//   - Tuple-sum. A single direct sum whose members are tuples; each
//     tuple covers all return positions in lock-step. Used to express
//     exclusive value pairs:
//
//     vow:cond * -> (Result!, nil) | (nil, error!)
//
// Exactly one of Positions or TupleSum is populated.
//
// Tuple-sum members coexist with literal values (`1`, `"x"`, `true`,
// `nil`) and with composition references (`@rule.OkErr`) that pull
// shared signatures out of named rules.
type ReturnAnnotation struct {
	Positions []PositionSpec
	TupleSum  *DirectSum
}

// PositionSpec is the contract at one return position. Exactly one of
// Single or Sum is non-nil; the parser guarantees this invariant.
type PositionSpec struct {
	Single *Term
	Sum    *DirectSum
}

// String renders the position back to its surface form.
func (p PositionSpec) String() string {
	switch {
	case p.Single != nil:
		return p.Single.String()
	case p.Sum != nil:
		return p.Sum.String()
	default:
		return "<invalid position>"
	}
}

// DirectSum is the set of alternatives accepted at one position (or,
// when used as the top-level TupleSum, across all return positions).
// The Qualifier is applied to the sum as a whole — per-member
// qualifiers are rejected by the parser.
type DirectSum struct {
	Members   []SumMember
	Qualifier Qualifier
}

// String renders the sum back to its surface form. A multi-member
// sum renders as `(A | B)` so the grouping survives a round trip
// through the parser. A single-member sum drops the parenthesised
// wrapper and renders the bare member with its qualifier suffix —
// `error` when unqualified, `error?` when Optional, the canonical
// surface for the `T?` shorthand. A parenthesised single member
// would otherwise re-parse as a one-position tuple, and the
// analyzer treats a 1-member DirectSum and a Single PositionSpec
// identically, so dropping the wrapper costs nothing.
func (d DirectSum) String() string {
	parts := make([]string, len(d.Members))
	for i, m := range d.Members {
		parts[i] = m.String()
	}
	if len(parts) == 1 {
		return parts[0] + d.Qualifier.String()
	}
	return "(" + strings.Join(parts, " | ") + ")" + d.Qualifier.String()
}

// SumMember is one alternative of a DirectSum. Exactly one of Term,
// Tuple, or Literal is non-nil.
type SumMember struct {
	Term    *Term
	Tuple   *TupleMember
	Literal *LiteralTerm
}

// String renders the member back to its surface form.
func (m SumMember) String() string {
	switch {
	case m.Term != nil:
		return m.Term.String()
	case m.Tuple != nil:
		return m.Tuple.String()
	case m.Literal != nil:
		return m.Literal.String()
	default:
		return "<invalid member>"
	}
}

// TupleMember is a parenthesized tuple of elements, used as a member
// of a tuple-sum (`(a, b) | (c, d)`). Each element corresponds to
// one return position; all tuples in the same sum must have the same
// arity (validated by the parser).
type TupleMember struct {
	Elements []TupleElement
}

// String renders the tuple back to its surface form.
func (t TupleMember) String() string {
	parts := make([]string, len(t.Elements))
	for i, e := range t.Elements {
		parts[i] = e.String()
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// TupleElement is one component of a TupleMember. Exactly one of
// Term or Literal is non-nil.
type TupleElement struct {
	Term    *Term
	Literal *LiteralTerm
}

// String renders the tuple element.
func (e TupleElement) String() string {
	switch {
	case e.Term != nil:
		return e.Term.String()
	case e.Literal != nil:
		return e.Literal.String()
	default:
		return "<invalid element>"
	}
}

// ParsePresetBody parses an expanded preset-YAML rule body into a
// ReturnAnnotation. The body is the post-expansion text the
// preset's `body` field produces after argument substitution, so
// the grammar matches the position-and-sum surface that drives
// the return-position checker. The implementation delegates to
// the precedence-based parser in parser.go; this thin shim
// preserves the public entry point.
func ParsePresetBody(payload string) (*ReturnAnnotation, error) {
	return parseReturnAnnotationNew(payload)
}

// allTupleMembers reports whether every member of sum is a tuple. A
// mixed sum (some tuples, some terms) is treated as malformed —
// `ErrA | (b, c)` is rejected by the caller via this predicate.
func allTupleMembers(sum *DirectSum) bool {
	if len(sum.Members) == 0 {
		return false
	}
	for _, m := range sum.Members {
		if m.Tuple == nil {
			return false
		}
	}
	return true
}

// requireUniformArity checks that every tuple member of a direct
// sum has the same arity, the structural invariant for promoting
// the sum to a tuple-sum at the analyzer boundary.
//
// vow:cond * -> ErrInvalidGrammar | nil
func requireUniformArity(sum *DirectSum) error {
	if len(sum.Members) == 0 {
		return nil
	}
	want := len(sum.Members[0].Tuple.Elements)
	for i, m := range sum.Members {
		if got := len(m.Tuple.Elements); got != want {
			return fmt.Errorf("tuple-sum members must have the same arity: member 0 has %d, member %d has %d: %w", want, i, got, ErrInvalidGrammar)
		}
	}
	return nil
}

// splitAtTopLevel splits s on every occurrence of sep that sits at
// the outermost lexical level — i.e. outside any `(...)` or
// double-quoted string literal. Returns trimmed tokens.
//
// Empty / whitespace-only input returns zero tokens. An unbalanced
// paren produces a structured error so the caller can attribute
// the mistake to the offending payload.
//
// vow:cond * -> _, ErrSyntax | nil
func splitAtTopLevel(s string, sep byte) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []string
	parens := 0
	inString := false
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inString:
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
		case c == '(':
			parens++
		case c == ')':
			parens--
			if parens < 0 {
				return nil, fmt.Errorf("unbalanced ): %w", ErrSyntax)
			}
		case c == sep && parens == 0:
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	if inString {
		return nil, fmt.Errorf("unterminated string literal: %w", ErrSyntax)
	}
	if parens != 0 {
		return nil, fmt.Errorf("unbalanced (: %w", ErrSyntax)
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out, nil
}

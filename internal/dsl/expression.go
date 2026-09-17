package dsl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/knowledge-work/go-vow/internal/seq"
)

// Reference is an addressable run inside an arrow-rule operand: a
// bare identifier (a parameter name, receiver, or named return), a
// `$N` positional return reference, optionally followed by a chain
// of postfix steps that walk a struct field or a slice/map element.
//
// The shape mirrors a Go expression of the form
// `name.field[0].sub["key"]`: the head identifier sets the scope,
// and each PathStep refines it further. Index literals collapse into
// the StepIndex slot; identifier-keyed indexing keeps the identifier
// in StepIdent so the resolver can look it up against the enclosing
// signature scope.
type Reference struct {
	Kind ReferenceKind
	Name string     // identifier name, or the digit/name following `$`.
	Path []PathStep // postfix chain in source order; empty for a bare head.
}

// ReferenceKind enumerates the head shapes of a Reference.
type ReferenceKind int

const (
	// RefIdent marks a head spelt as a bare identifier — a parameter
	// name, receiver, named return value, or any other identifier
	// that resolves against the enclosing signature scope.
	RefIdent ReferenceKind = iota

	// RefDollar marks a head spelt as `$N` (digits) or `$name`. The
	// digit form is the positional reference to a return value;
	// `$name` is reserved as the explicit-named return form so the
	// AST layer keeps both shapes uniform until the resolver decides
	// how to look them up.
	RefDollar
)

// String renders the reference back to its surface form so AST
// round-trips through a single helper. Postfix steps render their
// own delimiters (`.field`, `[0]`, `["key"]`).
func (r Reference) String() string {
	var b strings.Builder
	switch r.Kind {
	case RefDollar:
		b.WriteByte('$')
		b.WriteString(r.Name)
	default:
		b.WriteString(r.Name)
	}
	for _, step := range r.Path {
		b.WriteString(step.String())
	}
	return b.String()
}

// PathStep is one element of a Reference postfix chain. The
// discriminator decides which slot is meaningful: StepField uses
// Name, StepIndex uses Index, StepIdentKey uses Name as the
// identifier-typed key, StepStringKey uses Name as the literal
// payload (already-quoted, just like tStr).
type PathStep struct {
	Kind  StepKind
	Name  string // field name, identifier key, or quoted string key.
	Index int    // integer index for StepIndex.
}

// StepKind enumerates the postfix shapes a Reference admits.
type StepKind int

const (
	// StepField is the dot-prefixed field selector (`x.field`).
	StepField StepKind = iota

	// StepIndex is the slice element access with an integer literal
	// key (`xs[0]`).
	StepIndex

	// StepIdentKey is the indexed access keyed by an identifier
	// resolvable in the enclosing signature scope (`m[k]`).
	StepIdentKey

	// StepStringKey is the indexed access keyed by a literal string
	// (`m["primary"]`). The stored Name keeps the surrounding quotes
	// so the surface round-trips byte-for-byte through String.
	StepStringKey
)

// String renders the step back to its surface form.
func (s PathStep) String() string {
	switch s.Kind {
	case StepField:
		return "." + s.Name
	case StepIndex:
		return "[" + strconv.Itoa(s.Index) + "]"
	case StepIdentKey, StepStringKey:
		return "[" + s.Name + "]"
	}
	return fmt.Sprintf("<unknown step kind %d>", s.Kind)
}

// Direction picks the arrow shape between two operands. The
// structural form (`->`) keeps the structural semantics where a
// parameter-requirement gates a return-requirement. The logical
// forms (`=>` for implication and `<=>` for biconditional) are the
// expanded surface introduced together with the comparison
// operators and postfix references.
type Direction int

const (
	// DirStructural is the `prereq -> conseq` form.
	DirStructural Direction = iota

	// DirImply is the forward implication `lhs => rhs`.
	DirImply

	// DirEquiv is the biconditional `lhs <=> rhs`.
	DirEquiv
)

// String returns the source spelling of the direction.
func (d Direction) String() string {
	switch d {
	case DirStructural:
		return "->"
	case DirImply:
		return "=>"
	case DirEquiv:
		return "<=>"
	}
	return fmt.Sprintf("<unknown direction %d>", d)
}

// CompareOp selects the binary comparison between a Reference and a
// literal (or, for `==`/`!=`, between a Reference and `nil`). The
// ordered variants only carry meaning for numerically comparable
// operands; the validator owns the type compatibility check.
type CompareOp int

const (
	// OpEq is the equality comparison `==`.
	OpEq CompareOp = iota

	// OpNeq is the inequality comparison `!=`.
	OpNeq

	// OpLt is the strict ordered comparison `<`.
	OpLt

	// OpLte is the ordered comparison `<=`.
	OpLte

	// OpGt is the strict ordered comparison `>`.
	OpGt

	// OpGte is the ordered comparison `>=`.
	OpGte
)

// String returns the source spelling of the comparison operator.
func (o CompareOp) String() string {
	switch o {
	case OpEq:
		return "=="
	case OpNeq:
		return "!="
	case OpLt:
		return "<"
	case OpLte:
		return "<="
	case OpGt:
		return ">"
	case OpGte:
		return ">="
	}
	return fmt.Sprintf("<unknown compare op %d>", o)
}

// Expression is one operand of a logical arrow-rule. The grammar
// admits three shapes: a nil-check (`x != nil`), an equal or
// ordered comparison whose right-hand side carries one or more
// value-level alternatives (`x >= 2`, `tag == "login" | "signup"`,
// `foo == nil | 0`), and a bare reference reserved on the AST for
// future grammar growth (the surface parser currently requires a
// comparison operator after every reference). Exactly one of the
// optional slots is meaningful per Kind so consumers can fan out
// via a single switch.
type Expression struct {
	Kind ExpressionKind

	// Ref is the postfix-expression head shared by every shape.
	// For ExprNilCheck and ExprComparison it is the operand being
	// compared; for ExprBareRef it stands alone.
	Ref Reference

	// Op carries the comparison operator for ExprNilCheck and
	// ExprComparison. ExprNilCheck only admits OpEq or OpNeq.
	Op CompareOp

	// Members carries the right-hand side of an ExprComparison as
	// one or more alternative values. A single-member slice
	// represents the plain `ref op literal` shape; a multi-member
	// slice represents a value-level direct sum (`ref op a | b`).
	// The semantic on a multi-member sum follows the comparison
	// operator: `==` matches when any member equals the operand
	// (OR distribution); `!=` matches when every member differs
	// (AND distribution by De Morgan). Ordered operators (`<`,
	// `<=`, `>`, `>=`) reject a multi-member RHS at parse time.
	// ExprNilCheck carries no members and ExprBareRef leaves the
	// slice empty.
	Members []ExprMember
}

// ExprMember is one alternative of a comparison right-hand-side
// sum. A non-empty Literal carries the member's surface spelling
// for a non-nil value (integer, string, bool, or identifier-shaped
// token); Nil is true when the member is the bare `nil` token,
// and in that case Literal stays empty. Exactly one of the two
// shapes is set per member.
type ExprMember struct {
	Literal string
	Nil     bool
}

// String renders the member back to its surface form.
func (m ExprMember) String() string {
	if m.Nil {
		return "nil"
	}
	return m.Literal
}

// ExpressionKind enumerates the operand shapes the arrow grammar
// supports.
type ExpressionKind int

const (
	// ExprNilCheck is a nil-comparison (`x != nil`, `x == nil`).
	ExprNilCheck ExpressionKind = iota

	// ExprComparison is an ordered or equal comparison against a
	// non-nil literal (`x >= 2`, `tag == "login"`).
	ExprComparison

	// ExprBareRef is a standalone Reference reserved on the AST
	// for future grammar growth. The surface parser does not
	// produce this shape — every operand must currently include
	// a comparison operator — so consumers can rely on the slot
	// staying empty until the layer threads through.
	ExprBareRef
)

// String renders the expression back to its surface form.
func (e *Expression) String() string {
	if e == nil {
		return "<nil expression>"
	}
	switch e.Kind {
	case ExprNilCheck:
		return e.Ref.String() + " " + e.Op.String() + " nil"
	case ExprComparison:
		parts := seq.ChainOf(e.Members...).Map(ExprMember.String).ToSlice()
		return e.Ref.String() + " " + e.Op.String() + " " + strings.Join(parts, " | ")
	case ExprBareRef:
		return e.Ref.String()
	}
	return fmt.Sprintf("<unknown expression kind %d>", e.Kind)
}

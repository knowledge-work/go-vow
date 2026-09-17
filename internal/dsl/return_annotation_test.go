package dsl

import (
	"fmt"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestParsePresetBody_singleTerm(t *testing.T) {
	// `nonzero T` parses to a Term with Pred = Not{Zero{}}; we
	// observe the conversion-to-Pred shape here.
	a, err := ParsePresetBody("nonzero error")
	assert.MustNoError(t, "ParsePresetBody", err)
	assert.MustLen(t, "Positions", a.Positions, 1)
	pos := a.Positions[0]
	assert.MustNotNil(t, "Positions[0].Single", pos.Single)
	assert.Equal(t, "Type", pos.Single.Type, "error")
	samePredicate(t, "Pred", pos.Single.Pred, Not{Inner: Zero{}})
}

func TestParsePresetBody_directSum(t *testing.T) {
	a, err := ParsePresetBody("_, (ErrFoo | ErrBar)?")
	assert.MustNoError(t, "ParsePresetBody", err)
	assert.MustLen(t, "Positions", a.Positions, 2)
	assert.MustNotNil(t, "Positions[0].Single", a.Positions[0].Single)
	assert.Equal(t, "Positions[0].Single.Placeholder", a.Positions[0].Single.Placeholder, true)
	sum := a.Positions[1].Sum
	assert.MustNotNil(t, "Positions[1].Sum", sum)
	assert.Equal(t, "Sum.Qualifier", sum.Qualifier, QualifierOptional)
	assert.MustLen(t, "Sum.Members", sum.Members, 2)
	for i, want := range []string{"ErrFoo", "ErrBar"} {
		at := fmt.Sprintf("Sum.Members[%d]", i)
		assert.MustNotNil(t, at+".Term", sum.Members[i].Term)
		assert.Equal(t, at+".Term.Type", sum.Members[i].Term.Type, want)
	}
}

// TestParsePresetBody_sentinelMatcherRejected pins that the
// parser rejects the `<sentinel>` any-sentinel matcher with a
// structured diagnostic that points authors at the
// concrete-sentinel listing form.
func TestParsePresetBody_sentinelMatcherRejected(t *testing.T) {
	_, err := ParsePresetBody("(<sentinel>)?")
	assert.ErrorContains(t, "ParsePresetBody(`(<sentinel>)?`)", err, "<sentinel>")
	assert.ErrorContains(t, "ParsePresetBody(`(<sentinel>)?`)", err, "unsupported")
}

func TestParsePresetBody_mixedQualifiers(t *testing.T) {
	a, err := ParsePresetBody("nonzero Result, (ErrFoo | ErrBar)?")
	assert.MustNoError(t, "ParsePresetBody", err)
	assert.MustLen(t, "Positions", a.Positions, 2)
	assert.MustNotNil(t, "Positions[0].Single", a.Positions[0].Single)
	samePredicate(t, "Positions[0].Single.Pred", a.Positions[0].Single.Pred, Not{Inner: Zero{}})
	assert.MustNotNil(t, "Positions[1].Sum", a.Positions[1].Sum)
	assert.Equal(t, "Positions[1].Sum.Qualifier", a.Positions[1].Sum.Qualifier, QualifierOptional)
}

func TestParsePresetBody_errors(t *testing.T) {
	type errorCase struct {
		in      string
		wantMsg string
	}

	tabletest.Run(t, map[string]errorCase{
		"empty input":            {"", "empty position list"},
		"blank input":            {"  ", "empty position"},
		"trailing comma":         {"error,", "empty position"},
		"unbalanced paren":       {"(ErrA | ErrB", "unbalanced"},
		"stray close paren":      {"ErrA )", "unexpected token"},
		"empty sum member":       {"(ErrA | )", "empty member"},
		"suffix after sum":       {"(ErrA | ErrB)foo", "unexpected suffix"},
		"stacked sum qualifiers": {"(ErrA | ErrB)!?", "unexpected token"},
	}, func(t *testing.T, c errorCase) {
		_, err := ParsePresetBody(c.in)
		assert.ErrorContains(t, fmt.Sprintf("ParsePresetBody(%q)", c.in), err, c.wantMsg)
	})
}

func TestParsePresetBody_tupleSum(t *testing.T) {
	a, err := ParsePresetBody("(nonzero Result, nil) | (Other, ErrFoo)")
	assert.MustNoError(t, "ParsePresetBody", err)
	assert.MustNotNil(t, "TupleSum", a.TupleSum)
	assert.Len(t, "Positions for the tuple-sum form", a.Positions, 0)
	assert.MustLen(t, "TupleSum.Members", a.TupleSum.Members, 2)
	for i, m := range a.TupleSum.Members {
		at := fmt.Sprintf("TupleSum.Members[%d]", i)
		assert.MustNotNil(t, at+".Tuple", m.Tuple)
		assert.Len(t, at+" arity", m.Tuple.Elements, 2)
	}
	// First member: (nonzero Result, nil) — the `nonzero` prefix
	// gives the Term Pred=Not{Zero{}}; nil is a literal of LitNil
	// kind.
	t0 := a.TupleSum.Members[0].Tuple.Elements[0].Term
	assert.MustNotNil(t, "tuple[0][0].Term", t0)
	assert.Equal(t, "tuple[0][0].Term.Type", t0.Type, "Result")
	samePredicate(t, "tuple[0][0].Term.Pred", t0.Pred, Not{Inner: Zero{}})
	l0 := a.TupleSum.Members[0].Tuple.Elements[1].Literal
	assert.MustNotNil(t, "tuple[0][1].Literal", l0)
	assert.Equal(t, "tuple[0][1].Literal.Kind", l0.Kind, LitNil)
}

func TestParsePresetBody_tupleArityMismatch(t *testing.T) {
	_, err := ParsePresetBody("(a, b) | (c)")
	assert.ErrorContains(t, "ParsePresetBody with mismatched tuple arity", err, "arity")
}

func TestParsePresetBody_literalSum(t *testing.T) {
	type literalSumCase struct {
		in        string
		wantKinds []LiteralKind
	}

	tabletest.Run(t, map[string]literalSumCase{
		"(200 | 404 | 500)": {"(200 | 404 | 500)", []LiteralKind{LitInt, LitInt, LitInt}},
		`("a" | "b")`:       {`("a" | "b")`, []LiteralKind{LitString, LitString}},
		"(true | false)":    {"(true | false)", []LiteralKind{LitBool, LitBool}},
		"(-1 | 0 | 0xff)":   {"(-1 | 0 | 0xff)", []LiteralKind{LitInt, LitInt, LitInt}},
	}, func(t *testing.T, c literalSumCase) {
		a, err := ParsePresetBody(c.in)
		assert.MustNoError(t, fmt.Sprintf("ParsePresetBody(%q)", c.in), err)
		assert.MustLen(t, "Positions", a.Positions, 1)
		assert.MustNotNil(t, "Positions[0].Sum", a.Positions[0].Sum)
		members := a.Positions[0].Sum.Members
		assert.MustLen(t, "Sum.Members", members, len(c.wantKinds))
		for i, m := range members {
			at := fmt.Sprintf("Members[%d]", i)
			assert.MustNotNil(t, at+".Literal", m.Literal)
			assert.Equal(t, at+".Literal.Kind", m.Literal.Kind, c.wantKinds[i])
		}
	})
}

func TestParsePresetBody_mixedConstAndNil(t *testing.T) {
	a, err := ParsePresetBody("(ErrFoo | nil)")
	assert.MustNoError(t, "ParsePresetBody", err)
	members := a.Positions[0].Sum.Members
	assert.MustLen(t, "Sum.Members", members, 2)
	assert.MustNotNil(t, "Members[0].Term", members[0].Term)
	assert.Equal(t, "Members[0].Term.Type", members[0].Term.Type, "ErrFoo")
	assert.MustNotNil(t, "Members[1].Literal", members[1].Literal)
	assert.Equal(t, "Members[1].Literal.Kind", members[1].Literal.Kind, LitNil)
}

// TestPositionSpecStringConversion pins the surface-form mapping
// between input and canonical rendering. Bare types, predicate
// prefixes (`nonzero Result`), and placeholders round-trip; `T?`
// round-trips as `T?` because it parses to a 1-member Optional
// DirectSum whose renderer drops the parenthesised wrapper and
// re-emits the bare `?` suffix. A multi-member sum keeps its
// `(A | B)?` grouping so the round trip is unambiguous.
func TestPositionSpecStringConversion(t *testing.T) {
	type renderCase struct {
		in   string
		want string
	}

	tabletest.Run(t, map[string]renderCase{
		"error?":         {"error?", "error?"},
		"nonzero Result": {"nonzero Result", "nonzero Result"},
		"_":              {"_", "_"},
		"(ErrA | ErrB)":  {"(ErrA | ErrB)", "(ErrA | ErrB)"},
		"(ErrA | ErrB)?": {"(ErrA | ErrB)?", "(ErrA | ErrB)?"},
	}, func(t *testing.T, c renderCase) {
		a, err := ParsePresetBody(c.in)
		assert.MustNoError(t, fmt.Sprintf("ParsePresetBody(%q)", c.in), err)
		assert.Equal(t, "Position.String()", a.Positions[0].String(), c.want)
	})
}

// TestOptionalTermCollapsesToSum pins the AST shape of a bare `T?`
// position: it is a 1-member Optional DirectSum, not a Single Term
// carrying an Optional flag. The collapse unifies the optional
// admission on DirectSum.Qualifier so the analyzer reaches a single
// nil-handling path through validateSumPosition. The cases cover
// three representative nillable kinds — interface (`error`), pointer
// (`*int`), and slice (`[]int`) — to confirm the collapse is
// kind-agnostic across the kinds the surface parser folds as a
// single bare-type token.
func TestOptionalTermCollapsesToSum(t *testing.T) {
	type collapseCase struct {
		in       string
		wantType string
	}

	tabletest.Run(t, map[string]collapseCase{
		"error?": {"error?", "error"},
		"*int?":  {"*int?", "*int"},
		"[]int?": {"[]int?", "[]int"},
	}, func(t *testing.T, c collapseCase) {
		a, err := ParsePresetBody(c.in)
		assert.MustNoError(t, fmt.Sprintf("ParsePresetBody(%q)", c.in), err)
		pos := a.Positions[0]
		assert.MustNil(t, "Positions[0].Single; the optional term collapses to a sum", pos.Single)
		assert.MustNotNil(t, "Positions[0].Sum", pos.Sum)
		assert.Equal(t, "Sum.Qualifier", pos.Sum.Qualifier, QualifierOptional)
		assert.MustLen(t, "Sum.Members", pos.Sum.Members, 1)
		assert.MustNotNil(t, "Sum.Members[0].Term", pos.Sum.Members[0].Term)
		assert.Equal(t, "Sum.Members[0].Term.Type", pos.Sum.Members[0].Term.Type, c.wantType)
	})
}

// TestOptionalValueTypeRejected pins that the `?` suffix on an
// obvious value type is rejected during the collapse, mirroring the
// prefix-predicate check: a value type can never be nil. The
// rejection list covers every predeclared primitive isObviouslyValueType
// recognises so the contract stays uniform across signed and unsigned
// integers, the named-alias forms (`byte`, `rune`), the floating and
// complex families, and the textual / boolean primitives.
func TestOptionalValueTypeRejected(t *testing.T) {
	tabletest.Run(t, map[string]string{
		"bool?": "bool?", "byte?": "byte?",
		"complex64?": "complex64?", "complex128?": "complex128?",
		"float32?": "float32?", "float64?": "float64?",
		"int?": "int?", "int8?": "int8?", "int16?": "int16?",
		"int32?": "int32?", "int64?": "int64?",
		"rune?": "rune?", "string?": "string?",
		"uint?": "uint?", "uint8?": "uint8?", "uint16?": "uint16?",
		"uint32?": "uint32?", "uint64?": "uint64?", "uintptr?": "uintptr?",
	}, func(t *testing.T, in string) {
		_, err := ParsePresetBody(in)
		assert.Error(t, fmt.Sprintf("ParsePresetBody(%q); a value type can never be nil", in), err)
	})
}

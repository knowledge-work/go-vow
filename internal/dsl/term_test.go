package dsl

import (
	"fmt"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestParseTerm pins the bare-type and `T?` shapes ParseTerm
// accepts. ParseTerm does not handle predicate prefixes (`nonzero
// T`, `nonnil T`); those reach the AST through ParsePresetBody.
// `T?` is accepted (with its value-type invariants enforced) but
// its constraint drops on conversion: the analyzer does not
// enforce it, and the AST has no Pred shape to carry it. The
// sum-level Optional form lives on DirectSum, parsed by
// ParsePresetBody, not here. Composite shapes — nested pointers,
// slices of pointers, slices of slices, pointer-to-map, and
// map-with-slice-values — survive scanIdent as a single tIdent so
// the `?` suffix rides on the composed type and the parser accepts
// it under the not-value-type fallback.
func TestParseTerm(t *testing.T) {
	// The case name is the surface input, so it stays a field too:
	// the run function parses it.
	type parseCase struct {
		in   string
		want Term
	}

	tabletest.Run(t, map[string]parseCase{
		"*int":              {"*int", Term{Type: "*int"}},
		"*int?":             {"*int?", Term{Type: "*int"}},
		"error":             {"error", Term{Type: "error"}},
		"error?":            {"error?", Term{Type: "error"}},
		"Result":            {"Result", Term{Type: "Result"}},
		"Result?":           {"Result?", Term{Type: "Result"}},
		"_":                 {"_", Term{Placeholder: true}},
		"_?":                {"_?", Term{Placeholder: true}},
		"[]byte?":           {"[]byte?", Term{Type: "[]byte"}},
		"map[string]int?":   {"map[string]int?", Term{Type: "map[string]int"}},
		"*[]byte":           {"*[]byte", Term{Type: "*[]byte"}},
		"*[]byte?":          {"*[]byte?", Term{Type: "*[]byte"}},
		"[]*int":            {"[]*int", Term{Type: "[]*int"}},
		"[]*int?":           {"[]*int?", Term{Type: "[]*int"}},
		"[][]byte":          {"[][]byte", Term{Type: "[][]byte"}},
		"[][]byte?":         {"[][]byte?", Term{Type: "[][]byte"}},
		"*map[string]int":   {"*map[string]int", Term{Type: "*map[string]int"}},
		"*map[string]int?":  {"*map[string]int?", Term{Type: "*map[string]int"}},
		"map[string][]int":  {"map[string][]int", Term{Type: "map[string][]int"}},
		"map[string][]int?": {"map[string][]int?", Term{Type: "map[string][]int"}},
	}, func(t *testing.T, c parseCase) {
		got, err := ParseTerm(c.in)
		assert.MustNoError(t, fmt.Sprintf("ParseTerm(%q)", c.in), err)
		assert.Equal(t, "Type", got.Type, c.want.Type)
		assert.Equal(t, "Placeholder", got.Placeholder, c.want.Placeholder)
		assert.Equal(t, fmt.Sprintf("PredicateEquals(%v, %v)", got.Pred, c.want.Pred),
			PredicateEquals(got.Pred, c.want.Pred), true)
	})
}

func TestParseTermErrors(t *testing.T) {
	type errorCase struct {
		in      string
		wantMsg string
	}

	tabletest.Run(t, map[string]errorCase{
		"empty":      {"", "empty"},
		"blank":      {"   ", "empty"},
		"lone ?":     {"?", "qualifier without a base type"},
		"lone !":     {"!", "qualifier without a base type"},
		"int?":       {"int?", `value type "int" cannot use ?`},
		"string?":    {"string?", `value type "string" cannot use ?`},
		"bool?":      {"bool?", `value type "bool" cannot use ?`},
		"int!":       {"int!", "spell the predicate as `nonzero"},
		"Result!":    {"Result!", "spell the predicate as `nonzero"},
		"*int!":      {"*int!", "spell the predicate as `nonzero"},
		"_!":         {"_!", "spell the predicate as `nonzero"},
		"stacked ??": {"T??", "qualifiers cannot stack"},
		"stacked !?": {"T!?", "qualifiers cannot stack"},
		"stacked ?!": {"T?!", "qualifiers cannot stack"},
	}, func(t *testing.T, c errorCase) {
		_, err := ParseTerm(c.in)
		assert.ErrorContains(t, fmt.Sprintf("ParseTerm(%q)", c.in), err, c.wantMsg)
	})
}

func TestTermIsValid(t *testing.T) {
	type validCase struct {
		term Term
		want bool
	}

	tabletest.Run(t, map[string]validCase{
		"placeholder only":           {Term{Placeholder: true}, true},
		"type only":                  {Term{Type: "int"}, true},
		"type with predicate":        {Term{Type: "int", Pred: Not{Inner: Zero{}}}, true},
		"bare predicate":             {Term{Pred: Nil{}}, true},
		"placeholder with predicate": {Term{Placeholder: true, Pred: Not{Inner: Zero{}}}, true},
		"placeholder and type":       {Term{Placeholder: true, Type: "x"}, false},
		"neither populated":          {Term{}, false},
	}, func(t *testing.T, c validCase) {
		assert.Equal(t, fmt.Sprintf("%+v.IsValid()", c.term), c.term.IsValid(), c.want)
	})
}

func TestParseTermAlwaysReturnsValid(t *testing.T) {
	tabletest.Run(t, map[string]string{
		"*int":    "*int",
		"error?":  "error?",
		"Result":  "Result",
		"_":       "_",
		"[]byte?": "[]byte?",
	}, func(t *testing.T, in string) {
		term, err := ParseTerm(in)
		assert.MustNoError(t, fmt.Sprintf("ParseTerm(%q)", in), err)
		assert.Equal(t, fmt.Sprintf("ParseTerm(%q) = %+v; IsValid()", in, term), term.IsValid(), true)
	})
}

// TestTermStringConversion pins the two-way mapping between
// surface input and canonical rendering. Bare types and
// placeholders round-trip; `T?` drops its value-level content
// (no Pred equivalent) and renders as the bare base — see
// ParseTerm's docstring for the rationale.
func TestTermStringConversion(t *testing.T) {
	type renderCase struct {
		in   string
		want string
	}

	tabletest.Run(t, map[string]renderCase{
		"*int":            {"*int", "*int"},
		"error":           {"error", "error"},
		"Result":          {"Result", "Result"},
		"_":               {"_", "_"},
		"[]byte?":         {"[]byte?", "[]byte"},
		"map[string]int?": {"map[string]int?", "map[string]int"},
		"Result?":         {"Result?", "Result"},
	}, func(t *testing.T, c renderCase) {
		term, err := ParseTerm(c.in)
		assert.MustNoError(t, fmt.Sprintf("ParseTerm(%q)", c.in), err)
		assert.Equal(t, fmt.Sprintf("ParseTerm(%q).String()", c.in), term.String(), c.want)
	})
}

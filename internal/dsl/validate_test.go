package dsl

import (
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestValidateTermBareTypeWarning(t *testing.T) {
	term, err := ParseTerm("Result")
	assert.MustNoError(t, "ParseTerm", err)
	diags := ValidateTerm(term, ValidateOptions{})
	assert.MustLen(t, "ValidateTerm", diags, 1)
	assert.Equal(t, "Severity", diags[0].Severity, TermSeverityWarning)
	assert.Contains(t, "Message", diags[0].Message, `bare type "Result"`)
}

func TestValidateTermStrictPromotesToError(t *testing.T) {
	term, err := ParseTerm("Result")
	assert.MustNoError(t, "ParseTerm", err)
	diags := ValidateTerm(term, ValidateOptions{StrictAnnotation: true})
	assert.MustLen(t, "ValidateTerm", diags, 1)
	assert.Equal(t, "Severity", diags[0].Severity, TermSeverityError)
}

// TestValidateTermClean covers term shapes that carry an explicit
// value-level predicate (so the "prefer ? or !" advisory stays
// silent) or that are placeholders (which intentionally stand for
// "any value at this position"). Through ParseTerm a `T?` parses
// to a bare-type AST (the `?` suffix is dropped, leaving no
// value-level predicate), so the bare-type advisory fires for it
// and the table covers only the predicate-carrying shapes.
func TestValidateTermClean(t *testing.T) {
	predNotZero := Not{Inner: Zero{}}

	tabletest.Run(t, map[string]Term{
		"pointer with a predicate":     {Type: "*int", Pred: predNotZero},
		"error with a predicate":       {Type: "error", Pred: predNotZero},
		"int with a predicate":         {Type: "int", Pred: predNotZero},
		"string with a predicate":      {Type: "string", Pred: predNotZero},
		"named type with a predicate":  {Type: "Result", Pred: predNotZero},
		"bare placeholder":             {Placeholder: true},
		"placeholder with a predicate": {Placeholder: true, Pred: predNotZero},
	}, func(t *testing.T, term Term) {
		assert.Len(t, "ValidateTerm", ValidateTerm(term, ValidateOptions{}), 0)
		assert.Len(t, "ValidateTerm strict", ValidateTerm(term, ValidateOptions{StrictAnnotation: true}), 0)
	})
}

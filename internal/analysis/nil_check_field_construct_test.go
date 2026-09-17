package analysis

import (
	"fmt"
	"go/ast"
	"go/parser"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
)

// TestCompositeLitValues_dropsKeys pins the key dropping that lets a keyed
// and a positional literal expose the same shape to the recursive walk.
// Nothing else in the suite discriminates the result, so without this the
// dropping can be deleted while every test stays green.
func TestCompositeLitValues_dropsKeys(t *testing.T) {
	expr, err := parser.ParseExpr("T{A: x, y, B: z}")
	assert.MustNoError(t, "ParseExpr", err)
	lit, isLit := expr.(*ast.CompositeLit)
	assert.MustEqual(t, "ParseExpr returns a composite literal", isLit, true)

	values := compositeLitValues(lit)
	assert.MustLen(t, "compositeLitValues", values, 3)
	for i, want := range []string{"x", "y", "z"} {
		ident, isIdent := values[i].(*ast.Ident)
		assert.MustEqual(t, fmt.Sprintf("values[%d] is an identifier", i), isIdent, true)
		assert.Equal(t, fmt.Sprintf("values[%d]", i), ident.Name, want)
	}
}

package analysis

import (
	"go/constant"
	"math"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestIsConstantZero exercises the kind-by-kind zero comparison so
// that future refactors of classifyZero do not silently regress.
// Float NaN is intentionally non-zero (documented in the function);
// Int values outside int64 are also treated as non-zero (the exact
// flag returned by constant.Int64Val drops to false).
func TestIsConstantZero(t *testing.T) {
	type zeroCase struct {
		v    constant.Value
		want bool
	}

	tabletest.Run(t, map[string]zeroCase{
		"int zero":          {constant.MakeInt64(0), true},
		"int positive":      {constant.MakeInt64(1), false},
		"int negative":      {constant.MakeInt64(-1), false},
		"int max int64":     {constant.MakeInt64(math.MaxInt64), false},
		"int min int64":     {constant.MakeInt64(math.MinInt64), false},
		"uint zero":         {constant.MakeUint64(0), true},
		"uint max":          {constant.MakeUint64(math.MaxUint64), false},
		"string empty":      {constant.MakeString(""), true},
		"string non-empty":  {constant.MakeString("x"), false},
		"string whitespace": {constant.MakeString(" "), false},
		"bool false":        {constant.MakeBool(false), true},
		"bool true":         {constant.MakeBool(true), false},
		"float zero":        {constant.MakeFloat64(0), true},
		"float positive":    {constant.MakeFloat64(1.5), false},
		"float negative":    {constant.MakeFloat64(-1e-300), false},
		"float NaN":         {constant.MakeFloat64(math.NaN()), false},
		"float infinity":    {constant.MakeFloat64(math.Inf(1)), false},
	}, func(t *testing.T, c zeroCase) {
		assert.Equal(t, "isConstantZero("+c.v.String()+")", isConstantZero(c.v), c.want)
	})
}

// TestIsConstantZero_unsupportedKind confirms the unknown-kind
// branch does not panic and answers "no". Kinds outside Int / String
// / Bool / Float are intentionally treated as non-zero so callers
// fall through to other evidence (e.g. the AST shape branch in
// classifyZero) without raising a false positive.
func TestIsConstantZero_unsupportedKind(t *testing.T) {
	assert.Equal(t, "isConstantZero(Unknown)", isConstantZero(constant.MakeUnknown()), false)
}

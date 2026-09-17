package dsl

import (
	"fmt"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestParseLogicalRule_smoke exercises the new logical-arrow path
// end-to-end through ParseConditionAnnotation. The cases cover the
// three operand shapes the grammar admits (nil-check, comparison,
// postfix chain), each combined with one of the new arrow flavours,
// so a regression in the parser would surface against the surface
// API authors use.
func TestParseLogicalRule_smoke(t *testing.T) {
	type logicalCase struct {
		in     string
		want   string
		dir    Direction
		lhKind ExpressionKind
		rhKind ExpressionKind
	}

	tabletest.Run(t, map[string]logicalCase{
		"nil-check imply nil-check": {
			in:     "x != nil => y != nil",
			want:   "x != nil => y != nil",
			dir:    DirImply,
			lhKind: ExprNilCheck,
			rhKind: ExprNilCheck,
		},
		"field nil-check biconditional": {
			in:     `e.Tag == "login" <=> e.Login != nil`,
			want:   `e.Tag == "login" <=> e.Login != nil`,
			dir:    DirEquiv,
			lhKind: ExprComparison,
			rhKind: ExprNilCheck,
		},
		"ordered comparison imply nil-check": {
			in:     "c.Version >= 2 => c.FeatureX != nil",
			want:   "c.Version >= 2 => c.FeatureX != nil",
			dir:    DirImply,
			lhKind: ExprComparison,
			rhKind: ExprNilCheck,
		},
		"slice access nil-check": {
			in:     "cfg.Servers[0] != nil => cfg.Endpoints[0] != nil",
			want:   "cfg.Servers[0] != nil => cfg.Endpoints[0] != nil",
			dir:    DirImply,
			lhKind: ExprNilCheck,
			rhKind: ExprNilCheck,
		},
		"map access nil-check biconditional": {
			in:     `m["primary"] != nil <=> m["secondary"] == nil`,
			want:   `m["primary"] != nil <=> m["secondary"] == nil`,
			dir:    DirEquiv,
			lhKind: ExprNilCheck,
			rhKind: ExprNilCheck,
		},
		"positional return imply nil-check": {
			in:     "$1 != nil => $2 == nil",
			want:   "$1 != nil => $2 == nil",
			dir:    DirImply,
			lhKind: ExprNilCheck,
			rhKind: ExprNilCheck,
		},
		"equality sum on lhs": {
			in:     "op == 0 | 1 => out != nil",
			want:   "op == 0 | 1 => out != nil",
			dir:    DirImply,
			lhKind: ExprComparison,
			rhKind: ExprNilCheck,
		},
		"inequality sum on rhs": {
			in:     "op == 0 => $1 != 1 | 2",
			want:   "op == 0 => $1 != 1 | 2",
			dir:    DirImply,
			lhKind: ExprComparison,
			rhKind: ExprComparison,
		},
		"sum with nil member": {
			in:     "op == 0 => out == nil | 0",
			want:   "op == 0 => out == nil | 0",
			dir:    DirImply,
			lhKind: ExprComparison,
			rhKind: ExprComparison,
		},
	}, func(t *testing.T, c logicalCase) {
		cond, err := ParseConditionAnnotation(c.in)
		assert.MustNoError(t, fmt.Sprintf("ParseConditionAnnotation(%q)", c.in), err)
		assert.MustLen(t, "rules", cond.Rules, 1)
		rule := cond.Rules[0]
		assert.MustEqual(t, "rule kind", rule.Kind, RuleArrow)
		assert.MustNotNil(t, "rule arrow", rule.Arrow)
		assert.Equal(t, "direction", rule.Arrow.Direction, c.dir)
		assert.Equal(t, "left kind", kindOrNil(rule.Arrow.Left), c.lhKind)
		assert.Equal(t, "right kind", kindOrNil(rule.Arrow.Right), c.rhKind)
		assert.Equal(t, "String()", rule.Arrow.String(), c.want)
	})
}

// TestParseLogicalRule_errors locks the structured diagnostics the
// logical-arrow path owns: mixed arrow forms, empty operands, and
// invalid index keys. Each case names the invariant the error
// signals so a future relaxation cannot pass silently.
func TestParseLogicalRule_errors(t *testing.T) {
	type errorCase struct {
		in      string
		wantMsg string
	}

	tabletest.Run(t, map[string]errorCase{
		"mixed arrow forms":                                 {"x != nil => y -> z", "mixed arrow forms"},
		"empty rhs":                                         {"x != nil =>", "right-hand side of `=>` is empty"},
		"ordered comparison against nil":                    {"x > nil => y != nil", "ordered comparison"},
		"missing field after dot":                           {"x. => y != nil", "must be followed by a field name"},
		"unclosed bracket":                                  {"xs[0 => y != nil", "expected `]`"},
		"missing operator after reference":                  {"x => y", "bare reference"},
		"non-identifier field name":                         {"x.123 != nil => y != nil", "is not a Go identifier"},
		"hex index literal":                                 {"xs[0xff] != nil => y != nil", "neither a decimal integer literal nor a Go identifier"},
		"leading-zero index":                                {"xs[01] != nil => y != nil", "neither a decimal integer literal nor a Go identifier"},
		"ordered comparison with sum rhs":                   {"x >= 0 | 1 => y != nil", "ordered comparison `>=` does not accept a value-level sum"},
		"ordered comparison with sum on right-hand operand": {"x == 0 => y < 1 | 2", "ordered comparison `<` does not accept a value-level sum"},
	}, func(t *testing.T, c errorCase) {
		_, err := ParseConditionAnnotation(c.in)
		assert.ErrorContains(t, fmt.Sprintf("ParseConditionAnnotation(%q)", c.in), err, c.wantMsg)
	})
}

// kindOrNil returns a stable ExpressionKind for a possibly-nil
// Expression so test diagnostics print a recognisable value
// instead of panicking on a nil deref.
func kindOrNil(e *Expression) ExpressionKind {
	if e == nil {
		return -1
	}
	return e.Kind
}

// TestParseSugarRule_smoke confirms that a sugar clause expansion
// produces one rule per clause, that the expanded rules share the
// same left-hand side, and that the SugarCarriedFromPrevious flag
// is set on every expanded rule. The cases cover both arrow
// flavours and the mixed-flavour shape that parser-level accepts.
func TestParseSugarRule_smoke(t *testing.T) {
	type sugarCase struct {
		in       string
		wantLeft string
		wantDirs []Direction
		wantRhs  []string
	}

	tabletest.Run(t, map[string]sugarCase{
		"imply sugar two clauses": {
			in:       "c.Version >= 2 => c.FeatureX != nil ; => c.FeatureY != nil",
			wantLeft: "c.Version >= 2",
			wantDirs: []Direction{DirImply, DirImply},
			wantRhs:  []string{"c.FeatureX != nil", "c.FeatureY != nil"},
		},
		"biconditional sugar two clauses": {
			in:       `e.Tag == "login" <=> e.Login != nil ; <=> e.Logout == nil`,
			wantLeft: `e.Tag == "login"`,
			wantDirs: []Direction{DirEquiv, DirEquiv},
			wantRhs:  []string{"e.Login != nil", "e.Logout == nil"},
		},
		"mixed sugar flavours accepted": {
			in:       "x != nil => y != nil ; <=> z == nil",
			wantLeft: "x != nil",
			wantDirs: []Direction{DirImply, DirEquiv},
			wantRhs:  []string{"y != nil", "z == nil"},
		},
	}, func(t *testing.T, c sugarCase) {
		cond, err := ParseConditionAnnotation(c.in)
		assert.MustNoError(t, fmt.Sprintf("ParseConditionAnnotation(%q)", c.in), err)
		assert.MustLen(t, "rules", cond.Rules, len(c.wantRhs))
		for i, rule := range cond.Rules {
			at := fmt.Sprintf("rule %d", i)
			assert.MustEqual(t, at+" kind", rule.Kind, RuleArrow)
			assert.MustNotNil(t, at+" arrow", rule.Arrow)
			assert.Equal(t, at+" direction", rule.Arrow.Direction, c.wantDirs[i])
			assert.Equal(t, at+" left", rule.Arrow.Left.String(), c.wantLeft)
			assert.Equal(t, at+" right", rule.Arrow.Right.String(), c.wantRhs[i])
			// Rule 0 is the anchor the later clauses carry from, so
			// only it may have the flag clear.
			assert.Equal(t, at+" SugarCarriedFromPrevious", rule.Arrow.SugarCarriedFromPrevious, i != 0)
		}
	})
}

// TestParseSugarRule_errors locks the diagnostics the sugar
// expansion owns: leading sugar without an anchor rule, sugar
// after a structural arrow rule, sugar after an atom-rule, and an
// empty sugar right-hand side. Each case names the invariant the
// error signals.
func TestParseSugarRule_errors(t *testing.T) {
	type sugarErrorCase struct {
		in      string
		wantMsg string
	}

	tabletest.Run(t, map[string]sugarErrorCase{
		"leading sugar without anchor": {"=> x != nil", "has no preceding rule"},
		"sugar after structural arrow": {"nonnil x -> result ; => y != nil", "cannot follow a structural"},
		"sugar after atom-rule":        {"@MyRule[ErrFoo] ; => y != nil", "cannot follow an atom-rule"},
		"sugar with empty rhs":         {"x != nil => y != nil ; =>", "has no right-hand side"},
	}, func(t *testing.T, c sugarErrorCase) {
		_, err := ParseConditionAnnotation(c.in)
		assert.ErrorContains(t, fmt.Sprintf("ParseConditionAnnotation(%q)", c.in), err, c.wantMsg)
	})
}

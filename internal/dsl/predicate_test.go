package dsl

import (
	"fmt"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestPredicateString(t *testing.T) {
	type stringCase struct {
		pred Predicate
		want string
	}

	tabletest.Run(t, map[string]stringCase{
		"zero":               {Zero{}, "zero"},
		"nil":                {Nil{}, "nil"},
		"not zero":           {Not{Inner: Zero{}}, "nonzero"},
		"not nil":            {Not{Inner: Nil{}}, "nonnil"},
		"sentinel ref":       {SentinelRef{Name: "ErrFoo"}, "ErrFoo"},
		"qualified sentinel": {SentinelRef{Name: "pkg.ErrFoo"}, "pkg.ErrFoo"},
		"int literal":        {Literal{Kind: LitInt, Raw: "0"}, "0"},
		"string literal":     {Literal{Kind: LitString, Raw: `"foo"`}, `"foo"`},
		"bool literal":       {Literal{Kind: LitBool, Raw: "true"}, "true"},
		"nil literal":        {Literal{Kind: LitNil, Raw: "nil"}, "nil"},
	}, func(t *testing.T, c stringCase) {
		assert.Equal(t, "Predicate.String()", c.pred.String(), c.want)
	})
}

func TestPredicateEquals(t *testing.T) {
	type equalsCase struct {
		a, b Predicate
		want bool
	}

	tabletest.Run(t, map[string]equalsCase{
		"both nil":             {nil, nil, true},
		"a nil only":           {nil, Zero{}, false},
		"zero same":            {Zero{}, Zero{}, true},
		"zero vs nil":          {Zero{}, Nil{}, false},
		"not zero same":        {Not{Inner: Zero{}}, Not{Inner: Zero{}}, true},
		"not zero vs not nil":  {Not{Inner: Zero{}}, Not{Inner: Nil{}}, false},
		"literal same":         {Literal{Kind: LitInt, Raw: "0"}, Literal{Kind: LitInt, Raw: "0"}, true},
		"literal raw differs":  {Literal{Kind: LitInt, Raw: "0"}, Literal{Kind: LitInt, Raw: "1"}, false},
		"literal kind differs": {Literal{Kind: LitInt, Raw: "0"}, Literal{Kind: LitString, Raw: "0"}, false},
		"sentinel same":        {SentinelRef{Name: "ErrFoo"}, SentinelRef{Name: "ErrFoo"}, true},
		"sentinel differs":     {SentinelRef{Name: "ErrFoo"}, SentinelRef{Name: "ErrBar"}, false},
	}, func(t *testing.T, c equalsCase) {
		assert.Equal(t, "PredicateEquals", PredicateEquals(c.a, c.b), c.want)
	})
}

func TestSimplifyNot(t *testing.T) {
	type simplifyCase struct {
		in   Predicate
		want Predicate
	}

	tabletest.Run(t, map[string]simplifyCase{
		"nil pred":                       {nil, nil},
		"single zero":                    {Zero{}, Zero{}},
		"nonzero stays":                  {Not{Inner: Zero{}}, Not{Inner: Zero{}}},
		"!nonzero collapses":             {Not{Inner: Not{Inner: Zero{}}}, Zero{}},
		"!!nonzero collapses to nonzero": {Not{Inner: Not{Inner: Not{Inner: Zero{}}}}, Not{Inner: Zero{}}},
		"empty Not collapses":            {Not{}, nil},
	}, func(t *testing.T, c simplifyCase) {
		// PredicateEquals rather than DeepEqual: the contract is
		// predicate identity, under which nil and a zero-value Not
		// are the same answer. The label carries both values, which
		// a bare boolean assertion would otherwise drop.
		got := SimplifyNot(c.in)
		assert.Equal(t, fmt.Sprintf("PredicateEquals(SimplifyNot(%v)=%v, %v)", c.in, got, c.want),
			PredicateEquals(got, c.want), true)
	})
}

func TestNewLiteral(t *testing.T) {
	type literalCase struct {
		in       string
		wantOK   bool
		wantKind LiteralKind
		wantRaw  string
	}

	tabletest.Run(t, map[string]literalCase{
		"nil keyword":     {"nil", true, LitNil, "nil"},
		"true":            {"true", true, LitBool, "true"},
		"false":           {"false", true, LitBool, "false"},
		"zero":            {"0", true, LitInt, "0"},
		"negative int":    {"-1", true, LitInt, "-1"},
		"hex int":         {"0xff", true, LitInt, "0xff"},
		"string":          {`"foo"`, true, LitString, `"foo"`},
		"bare identifier": {"notALiteral", false, 0, ""},
		"empty input":     {"", false, 0, ""},
	}, func(t *testing.T, c literalCase) {
		got, err := NewLiteral(c.in)
		if !c.wantOK {
			assert.Error(t, "NewLiteral("+c.in+")", err)
			return
		}
		assert.MustNoError(t, "NewLiteral("+c.in+")", err)
		assert.Equal(t, "Kind", got.Kind, c.wantKind)
		assert.Equal(t, "Raw", got.Raw, c.wantRaw)
	})
}

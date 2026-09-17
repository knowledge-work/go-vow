package dsl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/seq"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// tokenKinds runs the lexer and projects the token stream onto its
// kinds, which is the axis the tokenizer tables pin.
func tokenKinds(t *testing.T, in string, mode lexerMode) []tokKind {
	t.Helper()
	toks, err := tokenize(in, mode)
	assert.MustNoError(t, fmt.Sprintf("tokenize(%q, %v)", in, mode), err)
	return seq.ChainOf(toks...).Map(func(tok token) tokKind { return tok.Kind }).ToSlice()
}

// TestTokenize_kinds covers the lexer contract: every punctuation
// kind must surface as its own token, identifier-shaped runs glue
// across dots / stars / balanced brackets, the double-bang qualifier
// stays a single token, and string literals retain their quotes.
// The string-roundtrip column lets the table double as a stable
// surface-form reference — if a future change rewrites scanIdent the
// expected values document the intent.
func TestTokenize_kinds(t *testing.T) {
	type kindCase struct {
		in   string
		want []tokKind
	}

	tabletest.Run(t, map[string]kindCase{
		"empty":             {"", []tokKind{tEOF}},
		"whitespace only":   {"  \t", []tokKind{tEOF}},
		",":                 {",", []tokKind{tComma, tEOF}},
		"|":                 {"|", []tokKind{tPipe, tEOF}},
		"()":                {"()", []tokKind{tLParen, tRParen, tEOF}},
		"!?":                {"!?", []tokKind{tBang, tQuestion, tEOF}},
		"!!":                {"!!", []tokKind{tBang, tBang, tEOF}},
		"Result":            {"Result", []tokKind{tIdent, tEOF}},
		"*int":              {"*int", []tokKind{tIdent, tEOF}},
		"[]byte":            {"[]byte", []tokKind{tIdent, tEOF}},
		"map[string]int":    {"map[string]int", []tokKind{tIdent, tEOF}},
		"*[]byte":           {"*[]byte", []tokKind{tIdent, tEOF}},
		"[]*int":            {"[]*int", []tokKind{tIdent, tEOF}},
		"[][]byte":          {"[][]byte", []tokKind{tIdent, tEOF}},
		"*map[string]int":   {"*map[string]int", []tokKind{tIdent, tEOF}},
		"map[string][]int":  {"map[string][]int", []tokKind{tIdent, tEOF}},
		"pkg.ErrFoo":        {"pkg.ErrFoo", []tokKind{tIdent, tEOF}},
		"<sentinel>":        {"<sentinel>", []tokKind{tIdent, tEOF}},
		`"foo"`:             {`"foo"`, []tokKind{tStr, tEOF}},
		`"a\"b"`:            {`"a\"b"`, []tokKind{tStr, tEOF}},
		"A, B":              {"A, B", []tokKind{tIdent, tComma, tIdent, tEOF}},
		"A | B":             {"A | B", []tokKind{tIdent, tPipe, tIdent, tEOF}},
		"(a, b)":            {"(a, b)", []tokKind{tLParen, tIdent, tComma, tIdent, tRParen, tEOF}},
		"nonzero Result":    {"nonzero Result", []tokKind{tIdent, tIdent, tEOF}},
		"bare star":         {"*", []tokKind{tStar, tEOF}},
		"star then space":   {"* ", []tokKind{tStar, tEOF}},
		"star then ident":   {"* x", []tokKind{tStar, tIdent, tEOF}},
		"*pkg.T":            {"*pkg.T", []tokKind{tIdent, tEOF}},
		"**T":               {"**T", []tokKind{tIdent, tEOF}},
		"star then comma":   {"*, B", []tokKind{tStar, tComma, tIdent, tEOF}},
		"star after a pipe": {"A | *", []tokKind{tIdent, tPipe, tStar, tEOF}},
	}, func(t *testing.T, c kindCase) {
		assert.DeepEqual(t, fmt.Sprintf("tokenize(%q) kinds", c.in), tokenKinds(t, c.in, lexReturn), c.want)
	})
}

// TestTokenize_lexCondBrackets pins the lexer-mode contract on
// bracket-bearing tokens. In `lexReturn` mode scanIdent absorbs the
// balanced bracket run so a composed type stays atomic; in `lexCond`
// mode the bracket emits as its own tLBracket / tRBracket token so
// the logical-rule grammar can walk a postfix step (`xs[0]`,
// `m["key"]`) without the type lexer eating the punctuation. The
// `@`-prefixed rule reference is the lone carve-out — scanIdent
// keeps absorbing brackets inside it so `@Use[X]` survives as a
// single tIdent the rule-ref parser can consume.
func TestTokenize_lexCondBrackets(t *testing.T) {
	type modeCase struct {
		in   string
		mode lexerMode
		want []tokKind
	}

	tabletest.Run(t, map[string]modeCase{
		"map type in lexReturn stays atomic":     {"map[string]int", lexReturn, []tokKind{tIdent, tEOF}},
		"map type in lexCond splits":             {"map[string]int", lexCond, []tokKind{tIdent, tLBracket, tIdent, tRBracket, tIdent, tEOF}},
		"slice type in lexReturn stays atomic":   {"[]byte", lexReturn, []tokKind{tIdent, tEOF}},
		"slice type in lexCond splits":           {"[]byte", lexCond, []tokKind{tLBracket, tRBracket, tIdent, tEOF}},
		"rule reference keeps its brackets":      {"@Use[X]", lexCond, []tokKind{tIdent, tEOF}},
		"rule reference keeps both bracket runs": {"@Use[X][Y]", lexCond, []tokKind{tIdent, tEOF}},
	}, func(t *testing.T, c modeCase) {
		assert.DeepEqual(t, fmt.Sprintf("tokenize(%q, %v) kinds", c.in, c.mode),
			tokenKinds(t, c.in, c.mode), c.want)
	})
}

// TestTokenize_errors pins the structured diagnostics the lexer
// owns: unterminated strings and unbalanced opening brackets. These
// must bubble out of tokenize() rather than being silently absorbed
// into a single tIdent run, because the parser cannot recover from
// them and downstream error messages would otherwise lose the offset
// of the actual mistake.
func TestTokenize_errors(t *testing.T) {
	type errorCase struct {
		in      string
		wantMsg string
	}

	tabletest.Run(t, map[string]errorCase{
		"unterminated string": {`"unterminated`, "unterminated string literal"},
		"unbalanced bracket":  {"map[string", "unbalanced ["},
		"open brace":          {"{", "unexpected character"},
		"close brace":         {"}", "unexpected character"},
		"brace-wrapped sum":   {"{ A | B }?", "unexpected character"},
	}, func(t *testing.T, c errorCase) {
		_, err := tokenize(c.in, lexReturn)
		assert.ErrorContains(t, fmt.Sprintf("tokenize(%q)", c.in), err, c.wantMsg)
	})
}

// TestParse_precedence proves the comma < pipe < paren grammar
// holds. A payload like `A, B | C, D` must split into three
// positions where the second one is a `B|C` sum (pipe binds
// tighter than comma); a parenthesised group resets the precedence
// so the inner positions are scoped inside the tuple.
func TestParse_precedence(t *testing.T) {
	t.Run("pipe binds tighter than comma", func(t *testing.T) {
		anno, err := ParsePresetBody("A, B | C, D")
		assert.MustNoError(t, "ParsePresetBody", err)
		assert.MustLen(t, "positions", anno.Positions, 3)
		assert.MustNotNil(t, "the middle position must be a sum", anno.Positions[1].Sum)
		members := anno.Positions[1].Sum.Members
		assert.MustLen(t, "middle sum members", members, 2)
		assert.Equal(t, "members[0].Term.Type", members[0].Term.Type, "B")
		assert.Equal(t, "members[1].Term.Type", members[1].Term.Type, "C")
	})
	t.Run("paren scopes inner positions", func(t *testing.T) {
		anno, err := ParsePresetBody("(A, B), C")
		assert.MustNoError(t, "ParsePresetBody", err)
		assert.MustLen(t, "positions", anno.Positions, 2)
		sum := anno.Positions[0].Sum
		assert.MustNotNil(t, "position 0 must wrap one tuple", sum)
		assert.MustLen(t, "position 0 members", sum.Members, 1)
		assert.MustNotNil(t, "position 0 member tuple", sum.Members[0].Tuple)
		assert.Len(t, "tuple arity", sum.Members[0].Tuple.Elements, 2)
	})
}

// TestParse_distributiveLift covers the rewrite from a single
// pipe-of-tuples position to the legacy TupleSum AST. The downstream
// analyzer keys off TupleSum to drive lock-step matching, so the
// lift is structurally observable rather than just a surface
// detail.
func TestParse_distributiveLift(t *testing.T) {
	t.Run("brace-less tuple sum lifts to TupleSum", func(t *testing.T) {
		anno, err := ParsePresetBody("(nonzero Result, nil) | (nil, nonzero error)")
		assert.MustNoError(t, "ParsePresetBody", err)
		assert.MustNotNil(t, "TupleSum", anno.TupleSum)
		assert.Len(t, "Positions when lifted", anno.Positions, 0)
		assert.Len(t, "TupleSum.Members", anno.TupleSum.Members, 2)
	})
	t.Run("mismatched arity rejected", func(t *testing.T) {
		_, err := ParsePresetBody("(a, b) | (c)")
		assert.ErrorContains(t, "ParsePresetBody with mismatched arity", err, "arity")
	})
}

// TestParse_predicateConversion pins the parsing of `nonzero T`
// into the canonical `Pred = Not{Zero{}}` shape; the per-member
// predicate binding inside a pipe-sum is exercised via the case
// below.
func TestParse_predicateConversion(t *testing.T) {
	t.Run("nonzero T parses to Pred Not{Zero{}}", func(t *testing.T) {
		anno, err := ParsePresetBody("nonzero Result")
		assert.MustNoError(t, "ParsePresetBody", err)
		term := anno.Positions[0].Single
		assert.MustNotNil(t, "Single", term)
		assert.Equal(t, "Type", term.Type, "Result")
		samePredicate(t, "Pred", term.Pred, Not{Inner: Zero{}})
	})
	t.Run("pipe with bang-suffix on last member", func(t *testing.T) {
		// The grammar gives per-member predicate prefixes their own
		// binding — a sum-level qualifier requires the grouping
		// form `(A | B)?`. We test the `nonzero` prefix on a single
		// member so we can observe the term-level binding via the
		// Pred field.
		anno, err := ParsePresetBody("A | nonzero B")
		assert.MustNoError(t, "ParsePresetBody", err)
		sum := anno.Positions[0].Sum
		assert.MustNotNil(t, "Sum", sum)
		assert.Equal(t, "Sum.Qualifier; a predicate prefix binds to a member, not the sum",
			sum.Qualifier, QualifierNone)
		assert.MustLen(t, "Sum.Members", sum.Members, 2)
		assert.MustNotNil(t, "members[1].Term", sum.Members[1].Term)
		samePredicate(t, "members[1].Pred", sum.Members[1].Term.Pred, Not{Inner: Zero{}})
	})
}

// TestParse_starWildcard covers the new `*` token. Bare `*` is the
// complete wildcard (Term{Wildcard:true}); `* error` pins the
// wildcard to a Go type for documentation, with the matcher still
// admitting every value. Trailing predicates / suffix qualifiers
// are rejected because a wildcard has no per-value constraint to
// modify.
func TestParse_starWildcard(t *testing.T) {
	t.Run("bare star at single position", func(t *testing.T) {
		anno, err := ParsePresetBody("*")
		assert.MustNoError(t, "ParsePresetBody", err)
		term := anno.Positions[0].Single
		assert.MustNotNil(t, "Single", term)
		assert.Equal(t, "Wildcard", term.Wildcard, true)
		assert.Equal(t, "Type", term.Type, "")
	})
	t.Run("star with type pin", func(t *testing.T) {
		anno, err := ParsePresetBody("* error")
		assert.MustNoError(t, "ParsePresetBody", err)
		term := anno.Positions[0].Single
		assert.MustNotNil(t, "Single", term)
		assert.Equal(t, "Wildcard", term.Wildcard, true)
		assert.Equal(t, "Type", term.Type, "error")
	})
	t.Run("star in sum", func(t *testing.T) {
		anno, err := ParsePresetBody("ErrFoo | *")
		assert.MustNoError(t, "ParsePresetBody", err)
		sum := anno.Positions[0].Sum
		assert.MustNotNil(t, "Sum", sum)
		assert.MustLen(t, "Sum.Members", sum.Members, 2)
		assert.Equal(t, "members[1].Wildcard", sum.Members[1].Term.Wildcard, true)
	})
	t.Run("star round trip", func(t *testing.T) {
		for _, c := range []struct{ in, want string }{
			{"*", "*"},
			{"* error", "* error"},
			{"A, *", "A, *"},
		} {
			anno, err := ParsePresetBody(c.in)
			assert.MustNoError(t, fmt.Sprintf("ParsePresetBody(%q)", c.in), err)
			parts := seq.ChainOf(anno.Positions...).Map(PositionSpec.String).ToSlice()
			assert.Equal(t, fmt.Sprintf("round-trip(%q)", c.in), strings.Join(parts, ", "), c.want)
		}
	})
	t.Run("star rejects suffix qualifier", func(t *testing.T) {
		_, err := ParsePresetBody("*?")
		assert.ErrorContains(t, "ParsePresetBody(`*?`)", err, "suffix qualifier")
	})
}

// TestParse_predicatePrefix covers the prefix-predicate surface.
// `nonzero T` is the canonical non-zero spelling; `nonnil error` is
// the nil-only variant; bare `zero` / `nonzero` produce a Pred-only
// Term whose Type the analyzer infers from the signature.
func TestParse_predicatePrefix(t *testing.T) {
	type prefixCase struct {
		in       string
		wantPred Predicate
		wantType string
	}

	tabletest.Run(t, map[string]prefixCase{
		"nonzero Result": {"nonzero Result", Not{Inner: Zero{}}, "Result"},
		"nonnil error":   {"nonnil error", Not{Inner: Nil{}}, "error"},
		"zero Result":    {"zero Result", Zero{}, "Result"},
		"bare nonzero":   {"nonzero", Not{Inner: Zero{}}, ""},
		"bare zero":      {"zero", Zero{}, ""},
	}, func(t *testing.T, c prefixCase) {
		anno, err := ParsePresetBody(c.in)
		assert.MustNoError(t, fmt.Sprintf("ParsePresetBody(%q)", c.in), err)
		term := anno.Positions[0].Single
		assert.MustNotNil(t, "Single", term)
		samePredicate(t, "Pred", term.Pred, c.wantPred)
		assert.Equal(t, "Type", term.Type, c.wantType)
	})
}

// TestParse_predicateRejects pins the negative space of the
// dispatch: every input that opens with `!` reaches the same
// diagnostic naming the keyword form, and the keyword form
// itself rejects a trailing suffix qualifier.
func TestParse_predicateRejects(t *testing.T) {
	type rejectCase struct {
		in      string
		wantMsg string
	}

	tabletest.Run(t, map[string]rejectCase{
		"!nil":     {"!nil", "use the `nonzero` or `nonnil` keyword"},
		"!zero":    {"!zero", "use the `nonzero` or `nonnil` keyword"},
		"!ErrFoo":  {"!ErrFoo", "use the `nonzero` or `nonnil` keyword"},
		"!true":    {"!true", "use the `nonzero` or `nonnil` keyword"},
		"nonzero!": {"nonzero!", "suffix qualifier"},
	}, func(t *testing.T, c rejectCase) {
		_, err := ParsePresetBody(c.in)
		assert.ErrorContains(t, fmt.Sprintf("ParsePresetBody(%q)", c.in), err, c.wantMsg)
	})
}

// TestParse_predicateAfterPipe covers a code path the table tests
// above do not exercise directly: a keyword-form predicate
// appearing as the right-hand side of a pipe alternation. The
// parser must dispatch to parseIdentLed from inside the pipe loop
// body of parseAlternation, not just from the first call.
func TestParse_predicateAfterPipe(t *testing.T) {
	anno, err := ParsePresetBody("A | nonnil B")
	assert.MustNoError(t, "ParsePresetBody", err)
	sum := anno.Positions[0].Sum
	assert.MustNotNil(t, "Sum", sum)
	assert.MustLen(t, "Sum.Members", sum.Members, 2)
	right := sum.Members[1].Term
	assert.MustNotNil(t, "the right member must be a Term", right)
	samePredicate(t, "right.Pred", right.Pred, Not{Inner: Nil{}})
	assert.Equal(t, "right.Type", right.Type, "B")
}

// TestParse_predicatesBothSides exercises a pipe whose every
// alternative is a prefix-predicate term. Both branches must
// produce their own Pred without one branch's parse state leaking
// into the other.
func TestParse_predicatesBothSides(t *testing.T) {
	anno, err := ParsePresetBody("nonzero T | nonnil error")
	assert.MustNoError(t, "ParsePresetBody", err)
	sum := anno.Positions[0].Sum
	assert.MustNotNil(t, "Sum", sum)
	assert.MustLen(t, "Sum.Members", sum.Members, 2)
	left, right := sum.Members[0].Term, sum.Members[1].Term
	assert.MustNotNil(t, "members[0].Term", left)
	assert.MustNotNil(t, "members[1].Term", right)
	samePredicate(t, "left.Pred", left.Pred, Not{Inner: Zero{}})
	assert.Equal(t, "left.Type", left.Type, "T")
	samePredicate(t, "right.Pred", right.Pred, Not{Inner: Nil{}})
	assert.Equal(t, "right.Type", right.Type, "error")
}

// TestParse_nilLiteralVsPredicate pins the context-sensitive
// `nil` disambiguation rule: bare `nil` in a sum or as a single
// position is the LitNil literal; `nil` followed by a Go-type
// token is the Nil predicate that constrains a position to the
// nil value of that type. The lookahead happens in parseIdentLed
// via peekN(1).
func TestParse_nilLiteralVsPredicate(t *testing.T) {
	t.Run("bare nil is a literal", func(t *testing.T) {
		anno, err := ParsePresetBody("nil")
		assert.MustNoError(t, "ParsePresetBody", err)
		sum := anno.Positions[0].Sum
		assert.MustNotNil(t, "Sum", sum)
		assert.MustLen(t, "Sum.Members", sum.Members, 1)
		assert.MustNotNil(t, "members[0].Literal", sum.Members[0].Literal)
		assert.Equal(t, "members[0].Literal.Kind", sum.Members[0].Literal.Kind, LitNil)
	})
	t.Run("nil in a sum is a literal even with a following type alternative", func(t *testing.T) {
		anno, err := ParsePresetBody("nil | T")
		assert.MustNoError(t, "ParsePresetBody", err)
		sum := anno.Positions[0].Sum
		assert.MustNotNil(t, "Sum", sum)
		assert.MustLen(t, "Sum.Members", sum.Members, 2)
		assert.MustNotNil(t, "members[0].Literal", sum.Members[0].Literal)
		assert.Equal(t, "members[0].Literal.Kind", sum.Members[0].Literal.Kind, LitNil)
		assert.MustNotNil(t, "members[1].Term", sum.Members[1].Term)
		assert.Equal(t, "members[1].Term.Type", sum.Members[1].Term.Type, "T")
	})
}

// TestParse_tupleTailedSumQualifier pins the term-vs-tuple
// qualifier asymmetry documented on parseAlternation. When the
// last alternative is a tuple (or literal), the suffix qualifier
// cannot attach to it syntactically and folds onto the surrounding
// DirectSum. The complement — qualifier on a term-tailed pipe —
// is already pinned by `pipe with bang-suffix on last member` in
// TestParse_predicateConversion.
func TestParse_tupleTailedSumQualifier(t *testing.T) {
	anno, err := ParsePresetBody("A | (B, C)?")
	assert.MustNoError(t, "ParsePresetBody", err)
	sum := anno.Positions[0].Sum
	assert.MustNotNil(t, "Sum", sum)
	assert.Equal(t, "Sum.Qualifier; a tuple-tailed sum binds ? to the sum",
		sum.Qualifier, QualifierOptional)
	assert.MustLen(t, "Sum.Members", sum.Members, 2)
	assert.NotNil(t, "members[1].Tuple", sum.Members[1].Tuple)
}

// TestParse_parenGrouping covers the grouping surface
// `(A | B)` and its qualifier-bearing cousin `(A | B)?`. The
// lookahead distinguishes a grouping (`|` at depth 1 before
// any `,`) from a tuple (`,` at depth 1 first), so both
// surfaces co-exist on the same `(` opener.
func TestParse_parenGrouping(t *testing.T) {
	t.Run("plain grouping carries a DirectSum shape", func(t *testing.T) {
		anno, err := ParsePresetBody("(ErrFoo | ErrBar)")
		assert.MustNoError(t, "ParsePresetBody", err)
		assert.MustLen(t, "positions", anno.Positions, 1)
		sum := anno.Positions[0].Sum
		assert.MustNotNil(t, "Sum", sum)
		assert.MustLen(t, "Sum.Members", sum.Members, 2)
		assert.Equal(t, "Qualifier on the plain grouping", sum.Qualifier, QualifierNone)
		for i, want := range []string{"ErrFoo", "ErrBar"} {
			at := fmt.Sprintf("members[%d]", i)
			assert.MustNotNil(t, at+".Term", sum.Members[i].Term)
			assert.Equal(t, at+".Term.Type", sum.Members[i].Term.Type, want)
		}
	})
	t.Run("optional qualifier on the grouping binds to the sum", func(t *testing.T) {
		anno, err := ParsePresetBody("(ErrFoo | ErrBar)?")
		assert.MustNoError(t, "ParsePresetBody", err)
		assert.MustNotNil(t, "Sum", anno.Positions[0].Sum)
		assert.Equal(t, "Qualifier", anno.Positions[0].Sum.Qualifier, QualifierOptional)
	})
	t.Run("tuple surface still parses to a Tuple member", func(t *testing.T) {
		anno, err := ParsePresetBody("(Result, nil)")
		assert.MustNoError(t, "ParsePresetBody", err)
		assert.MustNotNil(t, "TupleSum", anno.TupleSum)
		assert.MustLen(t, "TupleSum.Members", anno.TupleSum.Members, 1)
		assert.NotNil(t, "TupleSum.Members[0].Tuple", anno.TupleSum.Members[0].Tuple)
	})
	t.Run("degenerate parens span falls through to the legacy tuple path", func(t *testing.T) {
		anno, err := ParsePresetBody("(ErrFoo)")
		assert.MustNoError(t, "ParsePresetBody", err)
		// A parenthesised span with no separator at the outer depth
		// carries no alternation signal, so the lookahead leaves it
		// on the tuple branch. The downstream liftTupleSum then
		// turns the single-position result into a TupleSum (the
		// canonical legacy shape for a `(...)` payload).
		assert.NotNil(t, "TupleSum; the single-position parens must lift to a tuple form", anno.TupleSum)
	})
	t.Run("single member grouping with qualifier binds the qualifier to the sum", func(t *testing.T) {
		// `(ErrFoo)?` carries no `|` or `,` at depth 1, but the
		// trailing qualifier marks the span as a single-member
		// grouping. The result is a 1-member DirectSum carrying
		// the qualifier — so existing `vow:cond _, (ErrX)?`
		// style annotations keep their semantics.
		anno, err := ParsePresetBody("(ErrFoo)?")
		assert.MustNoError(t, "ParsePresetBody(`(ErrFoo)?`)", err)
		sum := anno.Positions[0].Sum
		assert.MustNotNil(t, "Sum", sum)
		assert.Equal(t, "Qualifier", sum.Qualifier, QualifierOptional)
		assert.MustLen(t, "Sum.Members", sum.Members, 1)
		assert.MustNotNil(t, "members[0].Term", sum.Members[0].Term)
		assert.Equal(t, "members[0].Term.Type", sum.Members[0].Term.Type, "ErrFoo")
	})
	t.Run("empty grouping is rejected with a structured error", func(t *testing.T) {
		_, err := ParsePresetBody("()")
		assert.Error(t, "ParsePresetBody(`()`)", err)
	})
}

// TestTermString_pred verifies the Term.String() branch that fires
// for pure-Pred terms: a non-zero predicate renders in the
// canonical `nonzero` prefix form.
func TestTermString_pred(t *testing.T) {
	type renderCase struct {
		term Term
		want string
	}

	tabletest.Run(t, map[string]renderCase{
		"bare not-zero":      {Term{Pred: Not{Inner: Zero{}}}, "nonzero"},
		"not-zero with type": {Term{Type: "Result", Pred: Not{Inner: Zero{}}}, "nonzero Result"},
		"zero with type":     {Term{Type: "Result", Pred: Zero{}}, "zero Result"},
		"nil predicate":      {Term{Type: "error", Pred: Nil{}}, "nil error"},
	}, func(t *testing.T, c renderCase) {
		assert.Equal(t, "Term.String()", c.term.String(), c.want)
	})
}

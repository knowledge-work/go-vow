package dsl

import (
	"fmt"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// assertNullness names a decl slot and pins its nullness, stopping
// the test when the slot is absent so the read below it is safe.
func assertNullness(t *testing.T, label string, decl *PositionDecl, want Nullness) {
	t.Helper()
	assert.MustNotNil(t, label, decl)
	assert.Equal(t, label+".Nullness", decl.Nullness, want)
}

// TestParseNilSignature_symbolForm pins the existing single-byte
// surface so the keyword alias does not regress the canonical
// shape. Both qualifier positions and the recv slot are covered.
func TestParseNilSignature_symbolForm(t *testing.T) {
	sig, err := ParseNilSignature("!.(!, ?) !")
	assert.MustNoError(t, "ParseNilSignature", err)
	assertNullness(t, "recv", sig.Recv, NullnessNonNil)
	assert.MustLen(t, "params", sig.Params, 2)
	assertNullness(t, "params[0]", sig.Params[0], NullnessNonNil)
	assertNullness(t, "params[1]", sig.Params[1], NullnessNillable)
	assert.MustLen(t, "returns", sig.Returns, 1)
	assertNullness(t, "returns[0]", sig.Returns[0], NullnessNonNil)
}

// TestParseNilSignature_keywordForm exercises the keyword alias on
// every qualifier slot the signature grammar reaches. The parsed
// AST must be byte-identical to the symbol form so downstream
// consumers stay oblivious to which surface the author chose.
func TestParseNilSignature_keywordForm(t *testing.T) {
	sig, err := ParseNilSignature("nonnil.(nonnil, nil) nonnil")
	assert.MustNoError(t, "ParseNilSignature", err)
	assertNullness(t, "recv", sig.Recv, NullnessNonNil)
	assert.MustLen(t, "params", sig.Params, 2)
	assertNullness(t, "params[0]", sig.Params[0], NullnessNonNil)
	assertNullness(t, "params[1]", sig.Params[1], NullnessNillable)
	assert.MustLen(t, "returns", sig.Returns, 1)
	assertNullness(t, "returns[0]", sig.Returns[0], NullnessNonNil)
}

// TestParseNilSignature_mixedSurface covers a payload that mixes
// the symbol and keyword forms within a single signature. The
// grammar accepts the surfaces independently per slot, so a
// per-call mixture must parse without ceremony.
func TestParseNilSignature_mixedSurface(t *testing.T) {
	sig, err := ParseNilSignature("(nonnil, ?) nil")
	assert.MustNoError(t, "ParseNilSignature", err)
	assert.MustLen(t, "params", sig.Params, 2)
	assertNullness(t, "params[0]", sig.Params[0], NullnessNonNil)
	assertNullness(t, "params[1]", sig.Params[1], NullnessNillable)
	assert.MustLen(t, "returns", sig.Returns, 1)
	assertNullness(t, "returns[0]", sig.Returns[0], NullnessNillable)
}

// TestParseFieldNilDecl_keywordForm confirms the field surface
// accepts the keyword alias. The field grammar shares the same
// decl-parser path as the signature surface, so a single positive
// case suffices to pin the integration.
func TestParseFieldNilDecl_keywordForm(t *testing.T) {
	type fieldCase struct {
		payload string
		want    Nullness
	}

	tabletest.Run(t, map[string]fieldCase{
		"nonnil": {"nonnil", NullnessNonNil},
		"nil":    {"nil", NullnessNillable},
		"!":      {"!", NullnessNonNil},
		"?":      {"?", NullnessNillable},
	}, func(t *testing.T, c fieldCase) {
		decl, err := ParseFieldNilDecl(c.payload)
		assert.MustNoError(t, fmt.Sprintf("ParseFieldNilDecl(%q)", c.payload), err)
		assert.Equal(t, "nullness", decl.Nullness, c.want)
	})
}

// TestParseNilSignature_keywordWordBoundary rejects a payload
// where an identifier-like token only happens to start with a
// keyword. Without the terminator guard `nillable` would parse as
// `nil` followed by trailing junk; the guard turns the same
// payload into a structured syntax error so the parser does not
// silently accept the half-recognised shape.
func TestParseNilSignature_keywordWordBoundary(t *testing.T) {
	_, err := ParseNilSignature("(nillable)")
	assert.ErrorIs(t, `ParseNilSignature("(nillable)")`, err, ErrSyntax)
}

// TestParseNilSignature_keywordInsideMapNest pins the keyword
// alias on the map-nest key slot. The key layer sits between
// `[` and `]`, so the keyword's terminator guard must accept
// `]` for the surface to stay symmetric with the symbol form.
// Without the `]` terminator the key slot would silently fall
// back to platform and `[nonnil]nonnil` would diverge from
// `[!]!`.
func TestParseNilSignature_keywordInsideMapNest(t *testing.T) {
	sig, err := ParseNilSignature("([nonnil]nonnil)")
	assert.MustNoError(t, "ParseNilSignature", err)
	assert.MustLen(t, "params", sig.Params, 1)
	nest := sig.Params[0].Nest
	assert.MustNotNil(t, "nest", nest)
	assert.MustLen(t, "nest.InnerDecls", nest.InnerDecls, 1)
	assertNullness(t, "nest.InnerDecls[0]", nest.InnerDecls[0], NullnessNonNil)
	assertNullness(t, "nest.Value", nest.Value, NullnessNonNil)
}

// TestParseNilSignature_nestEmpty pins the empty inner-decl shape:
// `([])` yields a NestDecl with no inner decls and no outer decl —
// the bare-slice surface on the unified payload.
func TestParseNilSignature_nestEmpty(t *testing.T) {
	sig, err := ParseNilSignature("([])")
	assert.MustNoError(t, "ParseNilSignature", err)
	assert.MustLen(t, "params", sig.Params, 1)
	nest := sig.Params[0].Nest
	assert.MustNotNil(t, "nest", nest)
	assert.Len(t, "nest.InnerDecls", nest.InnerDecls, 0)
	assert.Nil(t, "nest.Value", nest.Value)
}

// TestParseNilSignature_nestSliceWithValue pins the empty inner-decl
// shape with a trailing outer decl: `([]!)` carries no inner decls
// and the outer slot binds the non-nil token.
func TestParseNilSignature_nestSliceWithValue(t *testing.T) {
	sig, err := ParseNilSignature("([]!)")
	assert.MustNoError(t, "ParseNilSignature", err)
	nest := sig.Params[0].Nest
	assert.MustNotNil(t, "nest", nest)
	assert.MustLen(t, "nest.InnerDecls", nest.InnerDecls, 0)
	assertNullness(t, "nest.Value", nest.Value, NullnessNonNil)
}

// TestParseNilSignature_nestSingleInner pins the single inner-decl
// shape: `([!])` carries one inner decl with a non-nil token and no
// outer decl. The map-key surface reaches the same parsed shape on
// the unified payload.
func TestParseNilSignature_nestSingleInner(t *testing.T) {
	sig, err := ParseNilSignature("([!])")
	assert.MustNoError(t, "ParseNilSignature", err)
	nest := sig.Params[0].Nest
	assert.MustNotNil(t, "nest", nest)
	assert.MustLen(t, "nest.InnerDecls", nest.InnerDecls, 1)
	assertNullness(t, "nest.InnerDecls[0]", nest.InnerDecls[0], NullnessNonNil)
	assert.Nil(t, "nest.Value", nest.Value)
}

// TestParseNilSignature_nestTwoElement pins the multi inner-decl
// shape: `([!,?])` carries two inner decls with distinct nullness
// tokens. The surface targets multi-type-parameter generic
// declarations the analyzer reads at instantiation time.
func TestParseNilSignature_nestTwoElement(t *testing.T) {
	sig, err := ParseNilSignature("([!,?])")
	assert.MustNoError(t, "ParseNilSignature", err)
	nest := sig.Params[0].Nest
	assert.MustNotNil(t, "nest", nest)
	assert.MustLen(t, "nest.InnerDecls", nest.InnerDecls, 2)
	assertNullness(t, "nest.InnerDecls[0]", nest.InnerDecls[0], NullnessNonNil)
	assertNullness(t, "nest.InnerDecls[1]", nest.InnerDecls[1], NullnessNillable)
}

// TestParseNilSignature_nestPlatformSlotInside pins the platform-slot
// inside the inner list: `([,!])` carries a nil decl at index 0 and
// a non-nil decl at index 1. The platform slot uses the same nil-
// pointer-as-platform convention the outer position decls follow.
func TestParseNilSignature_nestPlatformSlotInside(t *testing.T) {
	sig, err := ParseNilSignature("([,!])")
	assert.MustNoError(t, "ParseNilSignature", err)
	nest := sig.Params[0].Nest
	assert.MustNotNil(t, "nest", nest)
	assert.MustLen(t, "nest.InnerDecls", nest.InnerDecls, 2)
	assert.Nil(t, "nest.InnerDecls[0]; the platform slot", nest.InnerDecls[0])
	assertNullness(t, "nest.InnerDecls[1]", nest.InnerDecls[1], NullnessNonNil)
}

// TestParseNilSignature_nestTrailingCommaReject pins the parser's
// rejection of a trailing comma before `]`. The grammar accepts
// zero-or-more comma-separated inner decls but does not admit a
// dangling comma.
func TestParseNilSignature_nestTrailingCommaReject(t *testing.T) {
	_, err := ParseNilSignature("([!,])")
	assert.ErrorIs(t, `ParseNilSignature("([!,])")`, err, ErrSyntax)
}

// TestParseNilSignature_nestRecursive pins recursive nest
// composition: an inner decl that itself carries a nest body.
// `([[!]])` carries one inner decl whose own Nest field holds one
// further inner decl with a non-nil token.
func TestParseNilSignature_nestRecursive(t *testing.T) {
	sig, err := ParseNilSignature("([[!]])")
	assert.MustNoError(t, "ParseNilSignature", err)
	nest := sig.Params[0].Nest
	assert.MustNotNil(t, "nest", nest)
	assert.MustLen(t, "nest.InnerDecls", nest.InnerDecls, 1)
	inner := nest.InnerDecls[0]
	assert.MustNotNil(t, "inner", inner)
	assert.MustNotNil(t, "inner.Nest", inner.Nest)
	assert.MustLen(t, "inner.Nest.InnerDecls", inner.Nest.InnerDecls, 1)
	assertNullness(t, "inner.Nest.InnerDecls[0]", inner.Nest.InnerDecls[0], NullnessNonNil)
}

// TestNullness_StringCanonicalIsSymbol locks the canonical print
// form to the symbol surface across both qualifier states. The
// keyword alias is accept-only by design — the printer keeps the
// high-frequency symbol so position-decl mirrors stay terse.
func TestNullness_StringCanonicalIsSymbol(t *testing.T) {
	assert.Equal(t, "NullnessNonNil.String()", NullnessNonNil.String(), "!")
	assert.Equal(t, "NullnessNillable.String()", NullnessNillable.String(), "?")
}

// TestParseNilSignature_keywordErrorMessage confirms the error
// surface advertises both the symbol and keyword forms so an
// author confronted with a malformed payload sees the keyword
// alias as a legitimate option.
func TestParseNilSignature_keywordErrorMessage(t *testing.T) {
	_, err := ParseNilSignature("(@)")
	assert.MustError(t, `ParseNilSignature("(@)")`, err)
	assert.ErrorContains(t, "the error advertises the nonnil keyword", err, "nonnil")
	assert.ErrorContains(t, "the error advertises the nil keyword", err, "nil")
}

// TestParseNilSignature_layerRun pins the layered surface: the
// symbols after the first one land in InnerLayers in the order they
// are written, so `!?` reads as a non-nil outer layer over a nillable
// inner one.
func TestParseNilSignature_layerRun(t *testing.T) {
	sig, err := ParseNilSignature("(!?, ???)")
	assert.MustNoError(t, "ParseNilSignature", err)
	outer := sig.Params[0]
	assert.MustEqual(t, "param 0 nullness", outer.Nullness, NullnessNonNil)
	assert.DeepEqual(t, "param 0 InnerLayers", outer.InnerLayers, []Nullness{NullnessNillable})
	assert.Len(t, "param 1 InnerLayers", sig.Params[1].InnerLayers, 2)
}

// TestParseNilSignature_layerRunInsideNest pins the run inside a nest
// body, where the inner decl reaches the same parser.
func TestParseNilSignature_layerRunInsideNest(t *testing.T) {
	sig, err := ParseNilSignature("([!?])")
	assert.MustNoError(t, "ParseNilSignature", err)
	nest := sig.Params[0].Nest
	assert.MustNotNil(t, "nest", nest)
	assert.MustLen(t, "nest.InnerDecls", nest.InnerDecls, 1)
	inner := nest.InnerDecls[0]
	assertNullness(t, "inner", inner, NullnessNonNil)
	assert.DeepEqual(t, "inner.InnerLayers", inner.InnerLayers, []Nullness{NullnessNillable})
}

// TestParseNilSignature_layerRunStopsAtNest pins the boundary between
// the run and a nest body that follows it: `!?[]?` declares two layers
// on the position and a further decl on the element.
func TestParseNilSignature_layerRunStopsAtNest(t *testing.T) {
	sig, err := ParseNilSignature("(!?[]?)")
	assert.MustNoError(t, "ParseNilSignature", err)
	decl := sig.Params[0]
	assert.MustDeepEqual(t, "InnerLayers", decl.InnerLayers, []Nullness{NullnessNillable})
	assert.MustNotNil(t, "nest", decl.Nest)
	assertNullness(t, "nest.Value", decl.Nest.Value, NullnessNillable)
}

// TestParseNilSignature_keywordDoesNotStack pins the keyword alias as
// a one-layer spelling: `nonnil!` has no readable boundary between the
// word and the symbol, so it is rejected rather than read as two
// layers.
func TestParseNilSignature_keywordDoesNotStack(t *testing.T) {
	_, err := ParseNilSignature("(nonnil!)")
	assert.ErrorIs(t, `ParseNilSignature("(nonnil!)")`, err, ErrSyntax)
}

// TestParseNilSignature_layerRunRejectsGap pins the adjacency rule: a
// space ends the run, so the second symbol is trailing text rather
// than a second layer.
func TestParseNilSignature_layerRunRejectsGap(t *testing.T) {
	_, err := ParseNilSignature("(! ?)")
	assert.ErrorIs(t, `ParseNilSignature("(! ?)")`, err, ErrSyntax)
}

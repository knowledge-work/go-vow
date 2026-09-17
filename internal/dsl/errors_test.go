package dsl

import (
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
)

// These tests pin the sentinel-chain contract: every error returned
// from a public DSL entry point must wrap one of the package-level
// sentinels so callers can discriminate failure modes with errors.Is
// instead of substring matching on the rendered message.

func TestErrSyntax_wrappedByParseTerm(t *testing.T) {
	_, err := ParseTerm("")
	assert.ErrorIs(t, `ParseTerm("")`, err, ErrSyntax)
}

func TestErrInvalidGrammar_wrappedByParseTerm(t *testing.T) {
	_, err := ParseTerm("Foo??")
	assert.ErrorIs(t, `ParseTerm("Foo??")`, err, ErrInvalidGrammar)
}

func TestErrUnknownReference_wrappedByNewLiteral(t *testing.T) {
	_, err := NewLiteral("not_a_literal")
	assert.ErrorIs(t, `NewLiteral("not_a_literal")`, err, ErrUnknownReference)
}

func TestErrRetiredSurface_wrappedByParsePresetBody(t *testing.T) {
	_, err := ParsePresetBody("<sentinel>")
	assert.ErrorIs(t, `ParsePresetBody("<sentinel>")`, err, ErrRetiredSurface)
}

func TestErrInvalidPreset_wrappedByParse(t *testing.T) {
	// Empty `name:` triggers the "preset name is empty" arm of
	// validate(); Parse forwards it through "validate: %w" so this
	// also exercises the wrap chain a real caller would observe.
	_, err := Parse([]byte("name: \"\"\n"))
	assert.ErrorIs(t, "Parse(empty name)", err, ErrInvalidPreset)
}

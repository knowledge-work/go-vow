package analysis

import (
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/config"
)

// TestPackageBuiltinClosableOverridePrecedence pins the
// driver-supplied override winning over both resolver discovery
// and the embedded default. With no resolver or pass to consult,
// the function still returns the override's list because the
// override check runs first.
func TestPackageBuiltinClosableOverridePrecedence(t *testing.T) {
	overrideList := []string{"io.Closer"}
	override := &config.Config{Closable: config.Closable{BuiltinClosables: &overrideList}}
	state := newPassState(nil, config.NewResolver(), override)
	got := state.packageBuiltinClosableOverride(nil)
	assert.MustNotNil(t, "packageBuiltinClosableOverride; the override must take precedence", got)
	assert.DeepEqual(t, "the override's list", *got, overrideList)
}

// TestPackageBuiltinClosableOverrideAbsent pins the default
// fallthrough: no override, no resolver, and no pass yields nil
// — which the caller reads as "keep the embedded built-in list".
func TestPackageBuiltinClosableOverrideAbsent(t *testing.T) {
	state := newPassState(nil, nil, nil)
	assert.Nil(t, "packageBuiltinClosableOverride with no resolver and no override",
		state.packageBuiltinClosableOverride(nil))
}

// TestPackageBuiltinClosableOverrideEmptyList pins the empty
// override case — adopters who explicitly disable the built-in
// list via an empty slice must reach the analyzer as the empty
// slice rather than as nil.
func TestPackageBuiltinClosableOverrideEmptyList(t *testing.T) {
	empty := []string{}
	override := &config.Config{Closable: config.Closable{BuiltinClosables: &empty}}
	state := newPassState(nil, nil, override)
	got := state.packageBuiltinClosableOverride(nil)
	assert.MustNotNil(t, "an empty override must propagate as a non-nil pointer", got)
	assert.Len(t, "the empty override's contents", *got, 0)
}

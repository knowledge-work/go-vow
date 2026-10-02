// Package nilDeclCompleteness is the analysistest fixture for the
// declaration-completeness rule. The package ships its own vow.yaml
// carrying `nil_decl.require_declarations: true`, so the discovery
// walk turns the rule on for this scope only.
//
// The fixture pins what the rule demands and, just as importantly,
// what it leaves alone: types whose nilness a Go-wide convention
// already settles, type parameters whose constraint decides
// representability, non-nillable kinds, generated sources, and the
// two marker shapes that do not describe the enclosing signature.
package nilDeclCompleteness

import (
	"context"
	"testing"

	faketesting "nilDeclCompleteness/testing"
)

type Record struct{ Name string }

// T shares its name with testing.T, so testingHandleLookalikes can pin
// that the name alone does not make a handle.
type T struct{}

// undeclaredBoth carries no marker at all, so both the pointer
// parameter and the pointer return reach the rule uncovered.
func undeclaredBoth(id *string) *Record { // want `vow\[nil-decl\]: vow:nil leaves parameter id, return 1 without a nullness decl`
	return &Record{Name: *id}
}

// emptyParensOnOneParam pins the position the arity gate cannot hold.
// `()` is the canonical marker for a 0-or-1-param signature, so the
// arity rule accepts it here; the completeness rule still reports the
// parameter, because an accepted marker is not a written contract.
//
// vow:nil ()
func emptyParensOnOneParam(r *Record) { // want emptyParensOnOneParam:"nilDecl\\(\\)" `vow\[nil-decl\]: vow:nil leaves parameter r without a nullness decl`
	_ = r
}

// partiallyDeclared pins per-position granularity: the first slot
// carries `!`, the second is at platform, and only the second is
// named.
//
// vow:nil (!,) !
func partiallyDeclared(first *Record, second *Record) *Record { // want partiallyDeclared:"nilDecl\\(!,\\) !" `vow\[nil-decl\]: vow:nil leaves parameter second without a nullness decl`
	_ = second
	return first
}

// fullyDeclared covers every nillable position and stays silent.
//
// vow:nil (!,?) !
func fullyDeclared(r *Record, opts *Record) *Record { // want fullyDeclared:"nilDecl\\(!,\\?\\) !"
	if opts != nil {
		r.Name = opts.Name
	}
	return r
}

// conventionTypes stays silent without any marker: `context.Context`,
// `error`, and the empty interface each carry a Go-wide nilness
// convention, so a per-position decl would restate it.
func conventionTypes(ctx context.Context, payload any) (any, error) {
	_ = ctx
	return payload, nil
}

// testingHandles takes every testing handle and returns one.
func testingHandles(t *testing.T, b *testing.B, f *testing.F, m *testing.M, pb *testing.PB, tb testing.TB) *testing.T { // want `vow\[nil-decl\]: vow:nil leaves parameter t, parameter b, parameter f, parameter m, parameter pb, parameter tb, return 1 without a nullness decl`
	_, _, _, _, _ = b, f, m, pb, tb
	return t
}

// testingHandleLookalikes reports every position. t's type only shares
// the name T. ft's type also comes from a package named testing, but
// not the one at import path `testing`. ts is a slice of handles, and
// a slice can be nil.
func testingHandleLookalikes(t *T, ft *faketesting.T, ts []*testing.T) { // want `vow\[nil-decl\]: vow:nil leaves parameter t, parameter ft, parameter ts without a nullness decl`
	_, _, _ = t, ft, ts
}

// pointersToConventionTypes reports every position: a pointer adds a
// layer of nilness that no convention settles, even when it points at
// `error`, `context.Context`, or a testing handle.
func pointersToConventionTypes(tp **testing.T, tbp *testing.TB, errp *error, ctxp *context.Context) { // want `vow\[nil-decl\]: vow:nil leaves parameter tp, parameter tbp, parameter errp, parameter ctxp without a nullness decl`
	_, _, _, _ = tp, tbp, errp, ctxp
}

// typeParameterPositions stays silent because an unsubstituted type
// parameter's constraint decides whether nil is representable at all.
func typeParameterPositions[T any](in T) T { return in }

// nonNillableKinds stays silent: no position can hold nil.
func nonNillableKinds(n int, r Record) (string, bool) { return r.Name, n > 0 }

// unnamedPositions falls back to the 1-based index for the unnamed
// parameter and the blank identifier.
func unnamedPositions(*Record, map[string]int) []byte { // want `vow\[nil-decl\]: vow:nil leaves parameter 1, parameter 2, return 1 without a nullness decl`
	return nil
}

// namedReturns names return positions by their identifier when the
// signature declares one.
func namedReturns() (found *Record, tags []string) { // want `vow\[nil-decl\]: vow:nil leaves return found, return tags without a nullness decl`
	return nil, nil
}

// suppressedByMarker pins the escape hatch: the completeness
// diagnostic flows through vow:suppress like the nil-safety
// diagnostics do.
//
// vow:suppress: the contract for this boundary is still being decided
func suppressedByMarker(r *Record) *Record { return r }

// subjectScopedMarker retargets its contract at the callback rather
// than at this signature, so the enclosing positions stay out of the
// rule's reach.
//
// vow:nil[cb] (!)
func subjectScopedMarker(cb func(r *Record)) { cb(&Record{}) }

// Store pins the rule on the interface-method surface.
type Store interface {
	// Load carries no marker, so both positions report.
	Load(key *string) *Record // want `vow\[nil-decl\]: vow:nil leaves parameter key, return 1 without a nullness decl`

	// Save declares its pointer parameter; the error return needs no
	// decl because the convention already settles it.
	//
	// vow:nil (!)
	Save(r *Record) error // want Save:"nilDecl\\(!\\)"
}

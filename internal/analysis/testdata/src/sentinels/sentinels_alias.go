package sentinels

import "errors"

// aliasLeak demonstrates the canonical variable-flow case: a local
// variable is short-form-defined to a tracked sentinel, then returned
// without authorization. The classifier follows the single-define
// alias from `x` back to `ErrNotFound` and reports the leak at the
// return identifier.
func aliasLeak() error {
	x := ErrNotFound
	return x // want `vow\[sentinel-error\]: sentinel error ErrNotFound leaked: needs observation or explicit propagation`
}

// aliasObservedByIs proves the alias resolver participates in the
// observe class as well: `x := ErrNotFound` followed by
// `errors.Is(err, x)` inside an if condition discharges the
// obligation just like a direct reference would.
func aliasObservedByIs(err error) error {
	x := ErrNotFound
	if errors.Is(err, x) {
		return nil
	}
	return nil
}

// aliasNonSubjectSilent is the negative baseline: `x` is defined to
// `errors.New(...)`, which is a fresh non-tracked error value. The
// alias resolver finds an RHS that is a CallExpr (not an Ident), so
// the resolver returns nil and the analyzer leaves the reference
// alone.
func aliasNonSubjectSilent() error {
	x := errors.New("local")
	return x
}

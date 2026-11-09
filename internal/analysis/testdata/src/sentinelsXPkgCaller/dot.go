package sentinelsXPkgCaller

import . "sentinelsXPkgCallee"

// leakDotImported returns the imported sentinel through a dot import,
// where the reference spells it unqualified.
func leakDotImported() error {
	return ErrShared // want `vow\[sentinel-error\]: sentinel error ErrShared leaked: needs observation or explicit propagation`
}

// chainedAliasDotImported reaches the return through two aliases, and
// the leak still names the subject unqualified, as the reference spells
// it.
func chainedAliasDotImported() error {
	x := ErrShared
	y := x
	return y // want `vow\[sentinel-error\]: sentinel error ErrShared leaked: needs observation or explicit propagation`
}

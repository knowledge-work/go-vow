package sentinelsXPkgCaller

import shared "sentinelsXPkgCallee"

// leakRenamed returns the imported sentinel through a renamed import.
// The subject takes the local name this file gives the package.
func leakRenamed() error {
	return shared.ErrShared // want `vow\[sentinel-error\]: sentinel error shared.ErrShared leaked: needs observation or explicit propagation`
}

// chainedAliasRenamed reaches the return through two aliases, and the
// leak still names the subject with the local name the reference uses.
func chainedAliasRenamed() error {
	x := shared.ErrShared
	y := x
	return y // want `vow\[sentinel-error\]: sentinel error shared.ErrShared leaked: needs observation or explicit propagation`
}

// emitAuthorisedRenamed authorises the propagation under the local name
// this file uses.
//
// vow:emit shared.ErrShared
func emitAuthorisedRenamed() error {
	x := shared.ErrShared
	y := x
	return y
}

// condImportPathRenamed lists the sentinel by its import path, which
// matches whatever local name the file gives the package, so the
// return both passes the sum and is authorised.
//
// vow:cond * -> sentinelsXPkgCallee.ErrShared | nil
func condImportPathRenamed() error {
	return shared.ErrShared
}

// Package use is the only package a run over this fixture targets, so
// the leaks in it are reported only if decl's fact reaches it.
package use

import "sentinelxpkg/decl"

// Fetch returns the sentinel that package decl declares.
func Fetch() error {
	return decl.ErrGone
}

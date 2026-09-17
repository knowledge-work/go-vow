// Package concept exercises the vow:define concept-declaration
// surface. The Sentinel concept is built-in and behaves identically
// to the bare sentinel marker; other concepts are forward-compat
// surface — the analyzer parses the declaration and emits an info-
// level diagnostic so authors learn that no behaviour is registered
// for those concepts.
package concept

import (
	"errors"
)

// ErrFoo is a Sentinel-concept member. Built-in behaviour applies:
// the must-consume rule tracks references to it like any other
// sentinel declared in the package.
//
// vow:define @Sentinel
var ErrFoo = errors.New("foo")

// LockHandle is a placeholder for a future @Resource concept. The
// declaration is accepted but the analyzer notes that no behaviour
// is registered — the diagnostic anchors at the var so the
// pending nature is visible from the declaration site rather than
// from a separate audit step.
//
// vow:define @Resource
var LockHandle = struct{}{} // want `vow\[concept\]: vow:define @Resource: concept declared but no behavior registered \(preset configuration pending\)`

// emitsFoo returns ErrFoo and authorises propagation on its
// signature. The Sentinel concept routes through the same chain-
// auth machinery the bare sentinel marker uses, so the chain-
// author surface continues to work without change.
//
// vow:cond * -> ErrFoo | nil
func emitsFoo(branch int) error {
	if branch == 0 {
		return ErrFoo
	}
	return nil
}

// leaksFoo returns ErrFoo without a matching signature. The must-
// consume rule fires because ErrFoo is a Sentinel-concept member;
// the diagnostic is identical to what the bare marker produces.
func leaksFoo() error {
	return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
}

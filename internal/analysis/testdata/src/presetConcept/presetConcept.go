// Package presetConcept is the analysistest fixture for a concept a
// preset loaded through vow.yaml gives behaviour to: the vow.yaml next
// to it names miss.yaml, which applies must-consume to `@Miss` with
// misslib.IsMiss as the consumer, and wraponly.yaml, which selects
// `@WrapOnly` but only registers wrap signatures. analysistest fails on
// any diagnostic a want does not expect, so the fixture also pins that
// the vow:define line of a loaded concept draws no report of its own.
package presetConcept

import (
	"errors"

	"example.com/misslib"
)

// ErrMiss belongs to the loaded concept, so it carries no
// pending-configuration note.
//
// vow:define @Miss
var ErrMiss = errors.New("miss") // want ErrMiss:"subject\\(vow:define @Miss\\)"

// Unregistered belongs to a concept no preset selects, so the note
// still applies.
//
// vow:define @Unregistered
var Unregistered = struct{}{} // want `vow\[concept\]: vow:define @Unregistered: concept declared but no behavior registered \(preset configuration pending\)`

// WrapOnly belongs to a concept only a passthrough-emit preset selects,
// which registers wrap signatures but no must-consume obligation, so the
// note still applies.
//
// vow:define @WrapOnly
var WrapOnly = errors.New("wrap only") // want WrapOnly:"subject\\(vow:define @WrapOnly\\)" `vow\[concept\]: vow:define @WrapOnly: concept declared but no behavior registered \(preset configuration pending\)`

// observed hands ErrMiss to the consumer the preset lists.
func observed(err error) bool {
	if misslib.IsMiss(err, ErrMiss) {
		return true
	}
	return false
}

// leaks returns ErrMiss without an observation or a propagation
// contract, so the loaded preset reports it under its own name.
func leaks() error {
	return ErrMiss // want `vow\[miss\]: sentinel error ErrMiss leaked: needs observation or explicit propagation`
}

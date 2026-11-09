// Package presetAddsBesideBuiltin is the analysistest fixture for a
// loaded preset under a name of its own: the vow.yaml next to it names
// extra.yaml, which also applies must-consume to `@Sentinel`, and it
// runs beside the built-in sentinel-error preset rather than in its
// place.
package presetAddsBesideBuiltin

import (
	"errors"

	"example.com/misslib"
)

// ErrGone is a sentinel both presets track.
//
// vow:define @Sentinel
var ErrGone = errors.New("gone") // want ErrGone:"subject\\(vow:define @Sentinel\\)"

// observed satisfies the added preset, whose consumers include
// misslib.IsMiss, but not the built-in one.
func observed(err error) bool {
	if misslib.IsMiss(err, ErrGone) { // want `vow\[sentinel-error\]: sentinel error ErrGone leaked`
		return true
	}
	return false
}

// leaks is reported by each preset under its own name.
func leaks() error {
	return ErrGone // want `vow\[sentinel-error\]: sentinel error ErrGone leaked` `vow\[extra-sentinel\]: sentinel error ErrGone leaked`
}

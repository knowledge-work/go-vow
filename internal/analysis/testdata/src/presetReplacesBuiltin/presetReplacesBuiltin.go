// Package presetReplacesBuiltin is the analysistest fixture for a
// loaded preset named like a built-in one: the vow.yaml next to it names
// sentinel.yaml, a sentinel-error preset that adds misslib.IsMiss to
// the consumers, and it replaces the built-in sentinel-error preset.
package presetReplacesBuiltin

import (
	"errors"

	"example.com/misslib"
)

// ErrGone is a sentinel the replacing preset tracks.
//
// vow:define @Sentinel
var ErrGone = errors.New("gone") // want ErrGone:"subject\\(vow:define @Sentinel\\)"

// observed hands ErrGone to the consumer only the replacing preset
// lists. The built-in preset would report the reference as a leak.
func observed(err error) bool {
	if misslib.IsMiss(err, ErrGone) {
		return true
	}
	return false
}

// leaks is reported once: the replaced preset no longer runs beside
// the one that replaced it.
func leaks() error {
	return ErrGone // want `vow\[sentinel-error\]: sentinel error ErrGone leaked`
}

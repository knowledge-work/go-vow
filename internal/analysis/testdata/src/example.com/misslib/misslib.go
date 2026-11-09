// Package misslib is the helper the preset fixtures name as a
// must-consume consumer, so that a call to it counts as an observation
// only when a loaded preset lists it.
package misslib

import "errors"

// IsMiss reports whether err matches target, as errors.Is does.
func IsMiss(err, target error) bool {
	return errors.Is(err, target)
}

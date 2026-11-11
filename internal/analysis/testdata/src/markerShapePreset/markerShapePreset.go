// Package markerShapePreset is the analysistest fixture for a marker
// that a preset subject selects on rather than one the analyzer
// reads itself. The marker is read, so it is not reported as unknown.
package markerShapePreset

import "errors"

// vow:tracked
var ErrTracked = errors.New("tracked") // want ErrTracked:"subject\\(vow:tracked\\)"

func Leak() error { return ErrTracked } // want `sentinel error ErrTracked leaked`

// vow:trackedd // want `vow\[annotation\]: vow:trackedd is not a marker the analyzer reads; the markers are .*, vow:tracked`
var ErrMisspelt = errors.New("misspelt")

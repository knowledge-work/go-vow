// Package presetMissing is the analysistest fixture for a vow.yaml
// naming a preset file that does not exist. The analyzer reports the
// file and runs with the built-in presets, so the sentinel below is
// still checked.
package presetMissing // want `vow\[config\]: .*/presetMissing/vow\.yaml: presets: open .*/presetMissing/missing\.yaml: no such file or directory; the built-in defaults apply instead`

import "errors"

// ErrGone is checked by the built-in sentinel-error preset.
//
// vow:define @Sentinel
var ErrGone = errors.New("gone")

func leaks() error {
	return ErrGone // want `vow\[sentinel-error\]: sentinel error ErrGone leaked`
}

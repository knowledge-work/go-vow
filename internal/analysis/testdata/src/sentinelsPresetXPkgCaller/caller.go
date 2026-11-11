// Package sentinelsPresetXPkgCaller returns each var of
// sentinelsPresetXPkgCallee once. Each leak is reported under the name of
// the preset whose marker the var carries, and only under that one.
package sentinelsPresetXPkgCaller

import "sentinelsPresetXPkgCallee"

func leakSentinel() error {
	return sentinelsPresetXPkgCallee.ErrSentinel // want `vow\[sentinel-error\]: sentinel error sentinelsPresetXPkgCallee.ErrSentinel leaked`
}

func leakRetry() error {
	return sentinelsPresetXPkgCallee.ErrRetry // want `vow\[retry\]: sentinel error sentinelsPresetXPkgCallee.ErrRetry leaked`
}

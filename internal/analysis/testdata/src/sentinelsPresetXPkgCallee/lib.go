// Package sentinelsPresetXPkgCallee declares one var per must-consume
// preset for the test that runs a second such preset beside the
// built-in ones. Each var's fact lists the marker it carries, so the
// caller adds the var to the subject set of that preset alone.
package sentinelsPresetXPkgCallee

import "errors"

// ErrSentinel carries the built-in sentinel-error preset's marker.
//
// vow:define @Sentinel
var ErrSentinel = errors.New("sentinel") // want ErrSentinel:"subject\\(vow:define @Sentinel\\)"

// ErrRetry carries the marker of the test's second preset.
//
// vow:define @Retry
var ErrRetry = errors.New("retry") // want ErrRetry:"subject\\(vow:define @Retry\\)"

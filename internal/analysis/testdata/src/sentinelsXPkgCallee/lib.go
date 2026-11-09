// Package sentinelsXPkgCallee declares the vars that the cross-package
// must-consume fixture (sentinelsXPkgCaller) refers to. ErrShared
// carries the marker, so the analyzer exports a fact on it; ErrPlain
// carries none, so references to it in the caller stay unchecked.
package sentinelsXPkgCallee

import "errors"

// ErrShared is the subject the caller refers to, through a plain, a
// renamed, and a dot import.
//
// vow:define @Sentinel
var ErrShared = errors.New("shared") // want ErrShared:"subject\\(vow:define @Sentinel\\)"

// ErrPlain is an error var without the marker. The caller returns it
// to show that an unmarked var in another package is not a subject.
var ErrPlain = errors.New("plain")

// LeakHere returns ErrShared inside the declaring package, where the
// leak is reported as it is in the caller.
func LeakHere() error {
	return ErrShared // want `vow\[sentinel-error\]: sentinel error ErrShared leaked: needs observation or explicit propagation`
}

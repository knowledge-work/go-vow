// Package importPkgScope exercises the package-wide reach of
// `vow:import`. This is the only file with a package doc comment —
// the shape a linted codebase has — so its alias has to serve the
// siblings.
//
// vow:import result "preset/std/result"
package importPkgScope

import "errors"

// vow:define @Sentinel
var OkMarker = errors.New("ok")

// vow:define @Sentinel
var ErrNotReady = errors.New("not ready")

// declaringFileResolves pins that the declaring file keeps
// resolving its own alias.
//
// vow:cond @result.OkErr[OkMarker, ErrNotReady]
func declaringFileResolves(ready bool) (error, error) {
	if ready {
		return OkMarker, nil
	}
	return nil, ErrNotReady
}

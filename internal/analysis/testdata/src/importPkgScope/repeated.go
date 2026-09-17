// This file repeats the marker the package doc file already carries.
// An identical rebinding is what code written against the per-file
// scope looks like, so it stays quiet.
//
// vow:import result "preset/std/result"
package importPkgScope

// vow:cond @result.OkErr[OkMarker, ErrNotReady]
func repeatedBindingResolves(ready bool) (error, error) {
	if ready {
		return OkMarker, nil
	}
	return nil, ErrNotReady
}

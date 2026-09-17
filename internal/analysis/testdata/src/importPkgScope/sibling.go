package importPkgScope

import "errors"

// siblingFileResolves carries no package doc comment of its own and
// resolves `@result` from the sibling's marker. Under a per-file
// scope this reported "alias \"result\" is not in scope".
//
// vow:cond @result.OkErr[OkMarker, ErrNotReady]
func siblingFileResolves(ready bool) (error, error) {
	if ready {
		return OkMarker, nil
	}
	return nil, ErrNotReady
}

// siblingFileEnforces pins that the borrowed rule enforces rather
// than merely parses: the second position is a CallExpr the
// analyzer cannot resolve, so the return matches no declared tuple.
//
// vow:cond @result.OkErr[OkMarker, ErrNotReady]
func siblingFileEnforces() (error, error) {
	return nil, errors.New("other") // want `vow\[sentinel-error\]: return values do not match any tuple of the declared sum`
}

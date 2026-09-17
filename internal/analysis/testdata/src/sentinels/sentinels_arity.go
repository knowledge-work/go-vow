package sentinels

// --- arity mismatch ---

// arityTooManyPositions declares two positions but the signature
// returns one value.
//
// vow:cond * -> nonzero error, error?
func arityTooManyPositions() error { // want `vow\[sentinel-error\]: vow:cond declares 2 return positions but the function signature returns 1 value`
	return nil
}

// arityTooFewPositions declares one position but the signature
// returns two values.
//
// vow:cond * -> nonzero error
func arityTooFewPositions() (int, error) { // want `vow\[sentinel-error\]: vow:cond declares 1 return position but the function signature returns 2 values`
	return 0, nil
}

// arityTupleMismatch declares a tuple-sum of arity 3 but the
// signature returns two values.
//
// vow:cond * -> (1, nil, false) | (0, "", true)
func arityTupleMismatch() (int, error) { // want `vow\[sentinel-error\]: vow:cond declares 3 return positions but the function signature returns 2 values`
	return 0, nil
}

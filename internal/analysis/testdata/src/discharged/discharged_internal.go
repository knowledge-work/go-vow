package discharged

// dischargedInternalLeaks declares the barrier but its own body still
// leaks the subject. The vow:discharged marker is a caller-side
// suppression; the function remains responsible for the contract
// declared at its own boundary, so internal verification still
// fires on this unauthorized return.
// vow:discharged
func dischargedInternalLeaks() error {
	return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
}

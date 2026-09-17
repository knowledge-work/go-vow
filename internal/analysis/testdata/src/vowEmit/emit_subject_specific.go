package vowEmit

// emitFooOnly declares chain authorisation for ErrFoo only. Returning
// ErrBar (which is not in the vow:emit set) leaks because the callee
// has not declared it as part of the propagation surface.
//
// vow:emit ErrFoo
func emitFooOnly(branch int) error {
	if branch == 0 {
		return ErrFoo
	}
	return ErrBar // want `vow\[sentinel-error\]: sentinel error ErrBar leaked: needs observation or explicit propagation`
}

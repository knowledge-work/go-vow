package vowEmit

// emitDeclaredButNeverReturned declares vow:emit ErrFoo but its body
// never returns ErrFoo. The self-validation pass catches the empty-
// emission declaration and emits a diagnostic so the contract is
// not silently vacuous.
//
// vow:emit ErrFoo
func emitDeclaredButNeverReturned() error { // want `vow\[sentinel-error\]: vow:emit ErrFoo declaration is not self-validated: function body never returns ErrFoo`
	return nil
}

// emitDeclaredViaShorthandButNeverReturned exercises the same
// invariant against the vow:emit shorthand. The shorthand lifts
// the payload into the same vow:emit declaration, so the self-
// validation pass reads off the same diagnostic.
//
// vow:emit ErrFoo
func emitDeclaredViaShorthandButNeverReturned() error { // want `vow\[sentinel-error\]: vow:emit ErrFoo declaration is not self-validated: function body never returns ErrFoo`
	return nil
}

// emitMultipleOneMissing declares both ErrFoo and ErrBar through
// the shorthand. The body returns ErrFoo but never ErrBar, so the
// self-validation pass diagnoses the missing subject independently
// per declared name.
//
// vow:emit ErrFoo, ErrBar
func emitMultipleOneMissing(branch int) error { // want `vow\[sentinel-error\]: vow:emit ErrBar declaration is not self-validated: function body never returns ErrBar`
	if branch == 0 {
		return ErrFoo
	}
	return nil
}

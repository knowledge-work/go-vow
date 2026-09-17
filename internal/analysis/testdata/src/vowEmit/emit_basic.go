package vowEmit

// emitsFoo declares the vow:emit ErrFoo atom-rule explicitly and
// returns ErrFoo from one of its branches. The callee self-
// validation accepts the declaration and the caller-side chain
// authorisation routes through the same atom-rule, so callers can
// propagate the result without a must-consume diagnostic firing on
// their own chain.
//
// vow:emit ErrFoo
func emitsFoo(branch int) error {
	if branch == 0 {
		return ErrFoo
	}
	return nil
}

// emitsFooViaShorthand uses the vow:emit shorthand. The marker
// lifts to the same atom-rule as the explicit form above, so the
// callee self-validation and caller-side chain authorisation read
// off the same path.
//
// vow:emit ErrFoo
func emitsFooViaShorthand() error {
	return ErrFoo
}

// usesEmitChainAuth propagates the result of an vow:emit ErrFoo
// callee. Because the callee declared the subject as a chain-auth
// source, the caller does not need its own return signature to
// propagate ErrFoo — the analyzer credits the chain through the
// callee's vow:emit declaration.
//
// vow:emit ErrFoo
func usesEmitChainAuth(branch int) error {
	return emitsFoo(branch)
}

// usesEmitChainAuthViaShorthand pins that the shorthand surface
// flows through the same caller-side chain-auth path as the
// explicit vow:emit declaration. The caller declares its propagation
// surface with `vow:emit` and propagates the vow:emit-declared callee;
// the analyzer credits ErrFoo through both ends of the chain
// without forcing the explicit-form spelling on either side.
//
// vow:emit ErrFoo
func usesEmitChainAuthViaShorthand() error {
	return emitsFooViaShorthand()
}

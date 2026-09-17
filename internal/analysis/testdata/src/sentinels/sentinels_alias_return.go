package sentinels

// aliasReturnMatchesAnnotation exercises variable-flow inside the
// vow:cond * -> validator: `x := ErrFoo` then `return x` resolves to
// the same shape as `return ErrFoo`, and ErrFoo is listed in the sum,
// so the position is silent.
//
// vow:cond * -> ErrFoo | nil
func aliasReturnMatchesAnnotation() error {
	x := ErrFoo
	return x
}

// aliasReturnMismatch defines `x := ErrBaz` then `return x`; ErrBaz
// is not in the declared sum, so the alias-aware checker reports the
// position. The must-consume rule also reports a leak because the
// annotation does not authorize ErrBaz.
//
// vow:cond * -> ErrFoo | ErrBar
func aliasReturnMismatch() error {
	x := ErrBaz
	return x // want `vow\[sentinel-error\]: return position 0: returning ErrBaz but the position only accepts ErrFoo \| ErrBar` `vow\[sentinel-error\]: sentinel error ErrBaz leaked: needs observation or explicit propagation`
}

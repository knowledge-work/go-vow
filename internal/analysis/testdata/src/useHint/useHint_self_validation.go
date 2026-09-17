package useHint

// declaredButNeverDischarges declares vow:use ErrFoo on the function
// doc but its body neither references ErrFoo in a conditional, calls
// errors.Is/As on it, hands it off to another vow:use callee, nor
// carries a statement-scope vow:use marker for it. The self-
// validation pass surfaces the contract violation at the function
// declaration so the author sees the declaration-vs-reality mismatch
// instead of relying on the silent caller-side suppression.
//
// vow:use ErrFoo
func declaredButNeverDischarges() error { // want `vow\[sentinel-error\]: vow:use ErrFoo declaration is not self-validated: function body has no discharge site for ErrFoo`
	return nil
}

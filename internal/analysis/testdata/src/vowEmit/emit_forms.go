package vowEmit

// Fixtures for the three vow:emit marker forms that coexist on the
// surface — the name-based subject form (preserved), the
// positional discrimination form (`$N subject`), and the
// passthrough mapping form (`name -> $N`). Each fixture exercises
// the parser shape and the resolution path the form drives so a
// regression on the form-dispatch boundary surfaces at the
// silent-baseline / diagnose pair.

// emitPositionalSilent declares that the function emits ErrFoo at
// the first return slot. The function returns ErrFoo from one of
// its branches so the callee self-validation accepts the
// declaration; the positional discrimination pins the binding to
// position 1 even though the signature exposes a single error
// return.
//
// vow:emit $1 ErrFoo
func emitPositionalSilent(branch int) error {
	if branch == 0 {
		return ErrFoo
	}
	return nil
}

// emitPositionalMultiSilent declares two positional emits — one
// per return slot — on a function whose signature exposes two
// error returns. Each spec carries its own subject identifier and
// its own slot index, so the comma-separated list reads as two
// independent positional specs.
//
// vow:emit $1 ErrFoo, $2 ErrBar
func emitPositionalMultiSilent() (error, error) {
	return ErrFoo, ErrBar
}

// emitPassthroughSilent declares that the function wraps the
// `err` parameter through its first return slot. The passthrough
// shape does not pin a specific subject — the parameter binding
// carries the propagation surface — so the silent baseline
// exercises the parser dispatch and the parameter-resolution path
// only.
//
// vow:emit err -> $1
func emitPassthroughSilent(err error) error {
	return err
}

// emitPassthroughPairSilent declares two passthrough specs on a
// function whose signature exposes two parameter-to-return-slot
// wrappings. Each comma-separated entry parses as its own
// passthrough spec.
//
// vow:emit err1 -> $1, err2 -> $2
func emitPassthroughPairSilent(err1, err2 error) (error, error) {
	return err1, err2
}

// emitPositionalMissingSubjectDiagnose declares a positional spec
// without a paired subject identifier. The parser rejects the
// entry as malformed and the empty-payload diagnostic surfaces at
// the function declaration.
//
// vow:emit $1
func emitPositionalMissingSubjectDiagnose() error { return nil } // want `vow\[sentinel-error\]: vow:emit requires at least one subject name`

// emitInvalidSlotDiagnose declares a positional spec with a
// zero-valued slot index. The parser rejects the entry because
// the positional grammar requires a 1-based slot, and the
// empty-payload diagnostic surfaces at the function declaration.
//
// vow:emit $0 ErrFoo
func emitInvalidSlotDiagnose() error { return nil } // want `vow\[sentinel-error\]: vow:emit requires at least one subject name`

// emitPassthroughEmptyLHSDiagnose declares a passthrough spec
// whose left-hand side is empty. The parser rejects the entry
// because the passthrough form needs a named parameter on the
// arrow's left, and the empty-payload diagnostic surfaces at the
// function declaration.
//
// vow:emit -> $1
func emitPassthroughEmptyLHSDiagnose(err error) error { _ = err; return nil } // want `vow\[sentinel-error\]: vow:emit requires at least one subject name`

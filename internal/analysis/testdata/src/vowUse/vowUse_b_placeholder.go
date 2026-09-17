package vowUse

// transducerPlaceholder documents the transducer discharge path
// that becomes load-bearing once the std rule for transducer
// recognition is not covered. The current analyzer
// has no transducer-aware path, so this fixture exercises the
// direct discharge form to keep the function self-validated.
// vow:use ErrFoo
func transducerPlaceholder(err error) {
	if err == ErrFoo {
		return
	}
}

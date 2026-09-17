package useHint

// useWithSuppressBothPresent pairs a vow:suppress marker (must-
// consume drop, debt) with a vow:use marker (subject-specific
// drop, assertion) on the same line. The vowReport filter chain
// runs suppress before use-assertion, so the diagnostic is dropped
// by suppress and the use-assertion path never observes it. The
// fixture exists to pin that no extra diagnostic surfaces for the
// "double drop" shape — both markers coexist legibly in source.
func useWithSuppressBothPresent() error {
	return ErrFoo // vow:suppress: use assertion is not wired here
}

// useHintForWrongSubjectKeepsLeak asserts discharge for a subject
// the leak is not about. The mismatched subject means the use-
// assertion filter declines to drop the diagnostic, so ErrFoo's
// leak still surfaces. The hint is on a leading comment so the
// trailing-vs-leading resolution claims the next line, and the
// claim is targeted (ErrBar) while the actual leak is on a
// different subject (ErrFoo).
func useHintForWrongSubjectKeepsLeak() error {
	// vow:use ErrBar
	return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
}

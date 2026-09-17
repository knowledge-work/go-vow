package useHint

// bareUseMarkerOnFunction surfaces the parser diagnostic for an
// authored-but-empty vow:use marker on a function doc. The error
// anchors at the function declaration so the want regex sits next
// to the offending site. The leak below is not silenced — the
// malformed marker registers no asserted subjects.
//
// vow:use
func bareUseMarkerOnFunction() error { // want `vow\[sentinel-error\]: vow:use requires at least one subject name`
	return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
}

// bareUseMarkerOnLine surfaces the same parser diagnostic when the
// authored-but-empty vow:use sits on a line comment that would
// otherwise have anchored the assertion. The diagnostic anchors at
// the enclosing function declaration so the want regex is
// reachable without having to predict the comment's exact column.
func bareUseMarkerOnLine() error { // want `vow\[sentinel-error\]: vow:use requires at least one subject name`
	// vow:use
	return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
}

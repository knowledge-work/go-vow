package useHint

import "errors"

// extraWhitespaceInPayload exercises the relaxed payload split:
// surplus interior whitespace and empty positions in the comma
// list (e.g. a trailing comma, doubled commas) collapse to the
// underlying subject set. The body discharges both subjects
// through errors.Is so the callee self-validation accepts the
// declaration; the trailing leaks are silenced by the function-
// level assertion.
//
// vow:use   ErrFoo  ,  , ErrBar  ,
func extraWhitespaceInPayload(probe error, branch int) error {
	if errors.Is(probe, ErrFoo) || errors.Is(probe, ErrBar) {
		return nil
	}
	if branch == 0 {
		return ErrFoo
	}
	return ErrBar
}

// orphanLeadingHintDropsSilently leaves a vow:use comment on the
// last line of the function body, after the only statement. The
// trailing-vs-leading resolver finds no statement on the comment's
// own line nor on the line below (there is none), so the marker
// anchors nowhere and the earlier leak surfaces unchanged. The
// orphan path is silent — no parser diagnostic — by design.
func orphanLeadingHintDropsSilently() error {
	return ErrFoo // want `vow\[sentinel-error\]: sentinel error ErrFoo leaked: needs observation or explicit propagation`
	// vow:use ErrFoo
}

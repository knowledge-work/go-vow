package vowUse

import "errors"

// ErrBar is a second subject used to exercise the multi-rule
// discharge surface.
// vow:define @Sentinel
var ErrBar = errors.New("bar")

// multiSubject declares two discharge contracts through the
// ';'-separated multi-rule grammar. Each subject is validated
// independently: dropping the discharge site for either subject
// would surface a per-subject diagnostic.
// vow:use ErrFoo, ErrBar
func multiSubject(a, b error) {
	if a == ErrFoo {
		return
	}
	if b == ErrBar {
		return
	}
}

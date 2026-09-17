// Package dep is the marker-bearing dependency of the scope-mode
// fixture. The scoped loader must promote it to a load pattern —
// its vow:nil fact is what the root package's caller-side check
// reads — while the stdlib import below stays unparsed.
package dep

import "strings"

// RequireNonNil declares its parameter non-nil so a caller that
// passes a statically-nil argument owes a diagnostic. The fact
// must survive the scoped load for the cross-package check to
// fire.
//
// vow:nil (!)
func RequireNonNil(p *int) {}

// Join keeps a dependency outside the fixture's scope prefix in
// the closure so the scoped load has something to leave unparsed.
func Join(parts []string) string { return strings.Join(parts, ",") }

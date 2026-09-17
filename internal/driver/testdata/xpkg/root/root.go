// Package root sits at the top of the graph as a non-imported
// caller-side package. Splitting the graph into "the root the
// driver visits" and "the leaf whose data must survive because a
// root reads it" lets the release test observe a non-root job
// whose result and syntax are freed while a root job's result
// stays.
package root

import "vowdrivertest.example/xpkg/caller"

// EntryUses invokes caller.Use so the caller package participates
// as a dependency of this root.
func EntryUses() { caller.Use() }

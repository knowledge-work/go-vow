// Package condLifecyclePairXPkgCaller is the caller-side fixture
// for the cross-package Closer / paired-resource lifecycle pair
// trace. The import target exports a Closable signature carrying
// a logical-arrow rule (see condLifecyclePairXPkgCallee). The
// analyzer reads the fact through pass.ImportObjectFact, observes
// the lifecycle pair shape on the imported signature, and inserts
// the `lifecycle pair` qualifier into the call-site diagnostic so
// the cross-package narrative matches the same-package path.
package condLifecyclePairXPkgCaller

import "condLifecyclePairXPkgCallee"

// localCloser is the cross-package fixture's Closer realisation:
// a pointer to a struct whose method set satisfies io.Closer.
type localCloser struct{}

func (c *localCloser) Close() error { return nil }

// callXPkgPairViolation drives the cross-package lifecycle pair
// contract: the first argument is a literal address-of expression
// over a Closer-shaped value and the second is the bare nil
// literal, so the implication's left-hand side holds and the
// right-hand side fails. The diagnostic surfaces at the call
// expression with the lifecycle-pair qualifier driven by the
// imported fact rather than by the callee's AST.
func callXPkgPairViolation() {
	condLifecyclePairXPkgCallee.AttachAcross(&localCloser{}, nil) // want `vow\[sentinel-error\]: call violates lifecycle pair vow:cond closer != nil => resource != nil: closer != nil holds at this call but resource != nil does not`
}

// callXPkgPairSilent supplies a non-nil resource alongside the
// non-nil closer so the implication holds at the call site and
// the cross-package check stays silent.
func callXPkgPairSilent() {
	condLifecyclePairXPkgCallee.AttachAcross(&localCloser{}, &condLifecyclePairXPkgCallee.Resource{Name: "live"})
}

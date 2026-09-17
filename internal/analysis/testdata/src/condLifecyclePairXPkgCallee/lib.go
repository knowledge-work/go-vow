// Package condLifecyclePairXPkgCallee is the callee-side fixture
// for the cross-package Closer / paired-resource lifecycle pair
// trace. The exported function carries a logical-arrow vow:cond
// rule whose operands bind to a Closable parameter and a
// reference-like paired-resource parameter. The analyzer exports
// a conditionLogicalFact for the entry so an importing caller's
// evaluator recognises the lifecycle pair shape from the imported
// signature and inserts the `lifecycle pair` qualifier into the
// call-site diagnostic.
package condLifecyclePairXPkgCallee

import "io"

// Resource is the paired-resource type the cross-package fixture
// pairs with an io.Closer parameter.
type Resource struct{ Name string }

// AttachAcross declares a forward implication that pins the
// lifecycle pair invariant on a Closer / paired-resource pair.
// The fact carries the rule across the package boundary so an
// importing call site that supplies a non-nil closer with a nil
// resource surfaces the lifecycle-pair diagnostic.
//
// vow:cond closer != nil => resource != nil
func AttachAcross(closer io.Closer, resource *Resource) { _ = closer; _ = resource } // want AttachAcross:"vow:cond\\(closer != nil => resource != nil\\)"

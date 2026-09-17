// Package condLifecyclePair is the analysistest fixture for the
// Closer / paired-resource lifecycle specialisation of the
// logical-arrow vow:cond evaluator. A rule whose operands bind to
// two signature positions — one Closable, one reference-like —
// surfaces a `lifecycle pair` qualifier in the call-site diagnostic
// so the author distinguishes a pair-lifecycle offence from a
// generic logical-arrow contradiction. Fixtures pair the same rule
// shape with call sites that satisfy the invariant (silent
// baselines) and call sites whose literal nil shape contradicts it
// (diagnose cases). The pair specialisation is additive on top of
// the generic call-site evaluator: rules whose operand types do
// not form a Closable / reference-like pair fall back to the
// generic diagnostic, and the Closable acquisition tracking that
// the lifecycle_* fixtures cover stays on its own axis.
package condLifecyclePair

import "io"

// Resource is the paired-resource type held alongside a Closer in
// the lifecycle pair invariant. The fixture uses a pointer to
// Resource as the paired operand so the analyzer recognises the
// reference-like shape on the non-Closer side.
type Resource struct{ Name string }

// attachStrict declares a forward implication: a non-nil closer
// must be paired with a non-nil resource. The rule expresses the
// canonical Closer / paired-resource lifecycle shape.
//
// vow:cond closer != nil => resource != nil
func attachStrict(closer io.Closer, resource *Resource) { _ = closer; _ = resource }

// attachEquiv declares a bidirectional pair invariant: closer is
// non-nil exactly when resource is non-nil. Either side missing
// while the other is supplied breaks the equivalence.
//
// vow:cond closer != nil <=> resource != nil
func attachEquiv(closer io.Closer, resource *Resource) { _ = closer; _ = resource }

// genericPairImply pairs two pointer arguments with a forward
// implication. Neither operand resolves to a Closable type, so the
// call-site evaluator falls back to the generic logical-arrow
// diagnostic without the lifecycle-pair qualifier.
//
// vow:cond x != nil => y != nil
func genericPairImply(x, y *int) { _ = x; _ = y }

// vow:define @Closable
type localConn struct{}

func (c *localConn) Close() error { return nil }

func openLocalConn() *localConn { return &localConn{} }

// callBothNonNil is the silent baseline for the forward
// implication: both operands non-nil satisfy the rule. The
// argument shapes leave the literal evaluator undecided, so the
// rule defers silently rather than committing.
func callBothNonNil(c io.Closer, r *Resource) {
	attachStrict(c, r)
}

// callImplyVacuous is the silent baseline for the implication's
// vacuous case: closer nil leaves the implication trivially
// satisfied regardless of resource.
func callImplyVacuous() {
	attachStrict(nil, nil)
	attachStrict(nil, &Resource{Name: "vacuous"})
}

// callImplyViolation triggers the lifecycle-pair diagnostic at the
// call site: closer is a literal non-nil Closable value but
// resource is literal nil, contradicting the forward implication.
func callImplyViolation() {
	attachStrict(&localConn{}, nil) // want `vow\[sentinel-error\]: call violates lifecycle pair vow:cond closer != nil => resource != nil: closer != nil holds at this call but resource != nil does not`
}

// callEquivBothNil is the silent baseline for the equivalence's
// both-nil branch: both operands nil satisfy the biconditional.
func callEquivBothNil() {
	attachEquiv(nil, nil)
}

// callEquivBothNonNil is the silent baseline for the equivalence's
// both-non-nil branch: both operands non-nil satisfy the
// biconditional through literal address-of shapes.
func callEquivBothNonNil() {
	attachEquiv(&localConn{}, &Resource{Name: "live"})
}

// callEquivCloserViolation triggers the lifecycle-pair diagnostic
// at the call site: closer literal non-nil but resource literal
// nil contradicts the equivalence.
func callEquivCloserViolation() {
	attachEquiv(&localConn{}, nil) // want `vow\[sentinel-error\]: call violates lifecycle pair vow:cond closer != nil <=> resource != nil: closer != nil holds at this call but resource != nil does not`
}

// callEquivResourceViolation triggers the lifecycle-pair diagnostic
// through the derived contrapositive: resource literal non-nil but
// closer literal nil contradicts the equivalence.
func callEquivResourceViolation() {
	attachEquiv(nil, &Resource{Name: "orphan"}) // want `vow\[sentinel-error\]: call violates lifecycle pair vow:cond closer != nil <=> resource != nil: resource != nil holds at this call but closer != nil does not`
}

// callGenericPairViolation triggers the generic logical-arrow
// diagnostic at the call site. Neither operand resolves to a
// Closable type, so the call-site evaluator does not insert the
// lifecycle-pair qualifier.
func callGenericPairViolation() {
	a := 0
	genericPairImply(&a, nil) // want `vow\[sentinel-error\]: call violates vow:cond x != nil => y != nil: x != nil holds at this call but y != nil does not`
}

// callWithDeferClose pairs the lifecycle-pair invariant with the
// Closable acquisition tracking that lifecycle_defer covers. The
// acquisition is discharged with `defer c.Close()` and the call
// site supplies a literal non-nil resource, so the two axes stay
// silent independently.
func callWithDeferClose() {
	c := openLocalConn()
	defer c.Close()
	attachStrict(c, &Resource{Name: "live"})
}

// tracker hosts the receiver-binding fixture: the lifecycle pair
// recogniser resolves a rule operand against the method receiver
// when the operand's identifier names the declared receiver. The
// type carries the vow:define @Closable marker so the recogniser
// recognises the receiver as a Closable signature position.
//
// vow:define @Closable
type tracker struct{ closer io.Closer }

// Close releases the tracker's underlying closer; the method
// promotes tracker to the Closable lifecycle pipeline so the
// receiver-binding fixture's pair recogniser can resolve the
// receiver type as Closable.
func (t *tracker) Close() error { return nil }

// attachOnTracker declares a forward implication whose left operand
// binds to the method receiver. The lifecycle pair recogniser
// inspects the receiver type alongside the parameter type so a
// receiver-typed Closer earns the specialised diagnostic the same
// way a parameter-typed Closer does.
//
// vow:cond t != nil => resource != nil
func (t *tracker) attachOnTracker(resource *Resource) { _ = t; _ = resource }

// callReceiverViolation triggers the lifecycle-pair diagnostic
// through the receiver-binding path: the receiver expression is a
// literal address-of (non-nil) but the resource argument is nil.
// The deferred Close discharges the Closable acquisition tracking
// so the receiver-binding fixture isolates the lifecycle-pair
// axis from the acquisition pipeline.
func callReceiverViolation() {
	t := &tracker{}
	defer t.Close()
	t.attachOnTracker(nil) // want `vow\[sentinel-error\]: call violates lifecycle pair vow:cond t != nil => resource != nil: t != nil holds at this call but resource != nil does not`
}

// attachMap declares a lifecycle pair where the paired-resource
// operand is a map-typed argument. Map types carry a nil zero
// value so the analyzer recognises the reference-like shape.
//
// vow:cond closer != nil => resources != nil
func attachMap(closer io.Closer, resources map[string]*Resource) { _ = closer; _ = resources }

// callMapPairViolation triggers the lifecycle-pair diagnostic at
// the call site through the map paired-resource shape.
func callMapPairViolation() {
	attachMap(&localConn{}, nil) // want `vow\[sentinel-error\]: call violates lifecycle pair vow:cond closer != nil => resources != nil: closer != nil holds at this call but resources != nil does not`
}

// attachInterface declares a lifecycle pair where the paired-
// resource operand is an interface-typed argument. Interfaces
// carry a nil zero value so the analyzer recognises the
// reference-like shape.
//
// vow:cond closer != nil => sink != nil
func attachInterface(closer io.Closer, sink interface{ Write([]byte) (int, error) }) {
	_ = closer
	_ = sink
}

// callInterfacePairViolation triggers the lifecycle-pair diagnostic
// at the call site through the interface paired-resource shape.
func callInterfacePairViolation() {
	attachInterface(&localConn{}, nil) // want `vow\[sentinel-error\]: call violates lifecycle pair vow:cond closer != nil => sink != nil: closer != nil holds at this call but sink != nil does not`
}

// attachStructValue declares a paired-shape rule where the paired
// operand is a value-typed struct. Struct values carry no nil
// representation, so the lifecycle pair recogniser falls through
// to the generic logical-arrow diagnostic. The fixture pins the
// boundary: rules that miss the nil-bearing shape on the paired
// leg ride the generic path.
//
// vow:cond closer != nil => info != nil
func attachStructValue(closer io.Closer, info ResourceInfo) { _ = closer; _ = info }

// ResourceInfo is the value-typed paired resource that exercises
// the boundary fall-through. The struct has no nil representation
// even though it holds a pointer field.
type ResourceInfo struct{ Ref *Resource }

// callStructValueViolation never fires because nil does not
// type-check against a value-typed struct argument; the fixture
// keeps the rule shape visible for the parser without offering a
// violation site, and the boundary documentation lives in the
// recogniser comment.
func callStructValueViolation(c io.Closer) {
	attachStructValue(c, ResourceInfo{})
}

// attachStructuralAndLogical carries both a structural rule and a
// logical-arrow rule on the same function. The structural rule
// rides the pre-existing parameter / return pipeline and the
// logical rule rides the lifecycle pair specialisation; the
// fixture pins that the two layers coexist without interfering.
//
// vow:cond nonnil closer -> *
// vow:cond closer != nil => resource != nil
func attachStructuralAndLogical(closer io.Closer, resource *Resource) error {
	_ = closer
	_ = resource
	return nil
}

// callStructuralAndLogicalViolation triggers the lifecycle-pair
// diagnostic through the logical rule while the structural rule
// stays silent at the call site.
func callStructuralAndLogicalViolation() {
	_ = attachStructuralAndLogical(&localConn{}, nil) // want `vow\[sentinel-error\]: call violates lifecycle pair vow:cond closer != nil => resource != nil: closer != nil holds at this call but resource != nil does not`
}

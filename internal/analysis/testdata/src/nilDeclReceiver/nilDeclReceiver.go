// Package nilDeclReceiver pins the caller-side receiver-nil check
// for methods whose vow:nil signature carries a receiver decl.
// Three branches reach the call site:
//
//   - The signature pins the receiver as non-nil (`!.()`) — a
//     statically-nil receiver surfaces a strict diagnostic that
//     quotes the contract.
//   - The signature pins the receiver as nillable (`?.()`) or omits
//     the receiver layer altogether — the call stays silent
//     because the author has not claimed a non-nil-receiver
//     constraint.
//   - The method carries no vow declaration — proven-nil receiver
//     surfaces the default-warn diagnostic; nil-receiver method
//     calls are Go's most common runtime panic source.
//
// The same nilDeclForFunc resolver feeds both this fixture (same-
// package callees) and the cross-package fixture
// (nilDeclXPkgCaller), so the per-branch behaviour stays aligned
// regardless of where the callee's declaration sits.
package nilDeclReceiver

// StrictService carries the receiver type whose `!.()`-declared
// methods pin the strict receiver branch.
type StrictService struct{}

// strictMethod pins the receiver as non-nil through a recv-
// prefixed signature decl. A statically-nil receiver at the call
// site contradicts the contract.
//
// vow:nil !.()
func (s *StrictService) strictMethod() {} // want strictMethod:"nilDecl!\\.\\(\\)"

// NillableService carries the receiver type whose `?.()`-declared
// methods pin the silent branch on a nillable-receiver call site.
type NillableService struct{}

// nillableMethod admits a nil receiver explicitly through the
// recv decl. A statically-nil receiver at the call site is
// consistent with the contract; the caller-side check stays
// silent.
//
// vow:nil ?.()
func (s *NillableService) nillableMethod() {} // want nillableMethod:"nilDecl\\?\\.\\(\\)"

// VowOnlyService carries a method that uses vow:cond without
// a vow:nil recv decl. The author has acknowledged vow's surface
// but has not claimed a non-nil-receiver constraint; the caller-
// side check stays silent.
type VowOnlyService struct{}

// declaredButNoRecvDecl carries a vow:cond (here, an arrow
// rule that pins one argument as non-nil) without a recv-
// prefixed vow:nil decl. The caller-side receiver check sees no
// recv decl and falls into the silent branch.
//
// vow:cond nonnil x -> *
func (s *VowOnlyService) declaredButNoRecvDecl(x *int) error { return nil }

// PlainService carries an undeclared method so the default-warn
// branch can be exercised.
type PlainService struct{}

// unannotated has no vow declaration. A statically-nil receiver
// surfaces the default-warn diagnostic.
func (s *PlainService) unannotated() {}

// strictReceiverDiagnoses pins the strict diagnostic: the
// receiver is the typed-nil conversion `(*StrictService)(nil)`
// and the method's vow:nil decl pins `!.()`.
func strictReceiverDiagnoses() {
	(*StrictService)(nil).strictMethod() // want `vow\[nil-safety\]: receiver of strictMethod is nil; callee declared receiver as non-nil via vow:nil`
}

// nillableReceiverSilent pins the silent branch: the receiver
// decl is `?.()` so the caller-side check stays silent on a
// statically-nil receiver.
func nillableReceiverSilent() {
	(*NillableService)(nil).nillableMethod()
}

// vowDeclaredSilent pins the silent branch on a vow:cond
// only method: no recv decl on a vow:nil signature, so the
// caller-side receiver check stays silent.
func vowDeclaredSilent() {
	a := 1
	_ = (*VowOnlyService)(nil).declaredButNoRecvDecl(&a)
}

// vowDeclaredArgSilent pins the silent baseline on a
// vow:cond-only method: the `nonnil x` prereq is a
// sentinel-narrowing predicate (with a body-leading dead-guard
// recogniser), not a caller-side argument check, so a nil
// argument at the call site stays silent. The receiver axis
// also stays silent because the callee carries no vow:nil
// recv decl.
func vowDeclaredArgSilent() {
	_ = (*VowOnlyService)(nil).declaredButNoRecvDecl(nil)
}

// vowNilParamsOnlySilent pins the silent baseline for a
// vow:nil signature that declares parameter positions but omits
// the receiver prefix. The author has not claimed a
// non-nil-receiver constraint, so a statically-nil receiver at
// the call site stays silent even though the per-parameter `!`
// tokens authorise other diagnostics on the same signature.
type ParamOnlyService struct{}

// paramOnlyMethod carries a vow:nil signature whose parameter
// list pins one position as `!` but whose receiver layer is
// omitted. The caller-side receiver check must fall into the
// silent branch.
//
// vow:nil (!)
func (s *ParamOnlyService) paramOnlyMethod(x *int) {} // want paramOnlyMethod:"nilDecl\\(!\\)"

func vowNilParamsOnlySilent() {
	a := 1
	(*ParamOnlyService)(nil).paramOnlyMethod(&a)
}

// suppressedStrictReceiver pins that vow:suppress drops the
// strict-receiver diagnostic at the function-level scope; the
// suppression valve is shared with the must-consume surface, so
// the receiver check rides the same filter chain.
//
// vow:suppress: regression net for the suppress filter
func suppressedStrictReceiver() {
	(*StrictService)(nil).strictMethod()
}

// suppressedDefaultWarn pins the same filter on the default-warn
// branch so both reported branches stay under one suppression
// surface.
//
// vow:suppress: regression net for the suppress filter
func suppressedDefaultWarn() {
	(*PlainService)(nil).unannotated()
}

// defaultWarnFiresWithoutVow pins the default-warn branch: the
// method carries no vow declaration, so the analyzer surfaces
// the generic nil-receiver-panic warning.
func defaultWarnFiresWithoutVow() {
	(*PlainService)(nil).unannotated() // want `vow\[nil-safety\]: receiver of unannotated is nil; method call panics on a nil receiver`
}

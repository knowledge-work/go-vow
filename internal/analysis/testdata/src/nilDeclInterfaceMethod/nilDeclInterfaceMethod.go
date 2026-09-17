// Package nilDeclInterfaceMethod is the analysistest fixture for
// the interface-method surface of the signature-mirror `vow:nil`
// marker: an interface method's doc comment carries the same
// grammar a free function or a concrete method accepts, and the
// parsed signature exports a signatureNilFact at the method's
// type-checker Object exactly as a concrete declaration does. The
// existing caller-side checks (argument and receiver) read that
// fact through the same TypesInfo.ObjectOf resolution they already
// use for concrete calls, so an interface-typed call site is
// covered without any change to the caller-side pass.
package nilDeclInterfaceMethod

// Doer pins the interface-method marker surface. Do's `!` marker
// exports a signatureNilFact at Do's type-checker Object; DoRecv's
// `!.()` marker exercises the same export on the receiver-decl
// layer.
type Doer interface {
	// vow:nil (!)
	Do(p *int) // want Do:"nilDecl\\(!\\)"

	// vow:nil !.()
	DoRecv() // want DoRecv:"nilDecl!\\.\\(\\)"

	// NoMarker carries no vow:nil marker, so it stays at platform
	// and the caller-side check never lifts it into a proof.
	NoMarker(p *int)
}

// impl is a concrete type satisfying Doer. Its methods carry no
// marker of their own; the fixture drives every check through the
// interface method's fact rather than through impl's declarations.
type impl struct{}

func (impl) Do(p *int)       {}
func (impl) DoRecv()         {}
func (impl) NoMarker(p *int) {}

// callArgWithNilLiteral exercises the caller-side argument check
// through an interface-typed parameter. The call resolves through
// TypesInfo.ObjectOf to Doer.Do's Object, and the signatureNilFact
// exported there pins the first parameter as non-nil, so a literal
// nil argument surfaces the same diagnostic a concrete-callee call
// would.
func callArgWithNilLiteral(d Doer) {
	d.Do(nil) // want `vow\[nil-safety\]: argument 1 \(p\) is nil`
}

// callArgWithAddress pins the silent baseline: a non-nil argument
// at the same interface-typed call site never surfaces a
// diagnostic.
func callArgWithAddress(d Doer) {
	x := 1
	d.Do(&x)
}

// callArgAtPlatformPosition pins the silent baseline for the
// unmarked method: a literal nil argument at NoMarker's position
// stays silent because the interface method carries no vow:nil
// marker at all.
func callArgAtPlatformPosition(d Doer) {
	d.NoMarker(nil)
}

// callReceiverWithNil exercises the caller-side receiver check
// through a typed-nil conversion to the interface type. Doer is
// nillable (every interface is), so the conversion satisfies
// argumentIsStaticallyNil, and DoRecv's `!.()` marker on the
// interface method's fact drives the strict receiver diagnostic.
func callReceiverWithNil() {
	Doer(nil).DoRecv() // want `vow\[nil-safety\]: receiver of DoRecv is nil; callee declared receiver as non-nil via vow:nil`
}

// arityOver declares one more parameter position than Bad's
// signature exposes. The analyzer surfaces a diagnostic at the
// interface method's own position, mirroring the function-level
// arity check.
type arityOverIface interface {
	// vow:nil (!,!)
	Bad(p *int) // want Bad:"nilDecl\\(!,!\\)" `vow\[nil-safety\]: vow:nil declares 2 parameter positions but the signature has 1`
}

// arityUnder declares fewer parameter positions than BadUnder's
// signature exposes. Same authoring bug as the over case,
// caught by the same arity gate on the interface method surface.
type arityUnderIface interface {
	// vow:nil (!)
	BadUnder(p, q *int) // want BadUnder:"nilDecl\\(!\\)" `vow\[nil-safety\]: vow:nil declares 1 parameter positions but the signature has 2`
}

// arityUnifiedIface pins the empty-parens exception on the
// interface method surface: `()` is the unified canonical for a
// 0- or 1-param method, mirroring the function-level exception.
type arityUnifiedIface interface {
	// vow:nil () !
	OneParam(p *int) *int // want OneParam:"nilDecl\\(\\) !"
}

// arityUnifiedReturnIface pins the return-axis half of the same
// unification on the interface method surface: an empty return
// expression covers a single-return method.
type arityUnifiedReturnIface interface {
	// vow:nil (!)
	OneReturn(p *int) string // want OneReturn:"nilDecl\\(!\\)"
}

// duplicateMarkerIface carries two vow:nil lines on the same
// method. The first parsed signature wins; the second emits a
// duplicate-marker diagnostic anchored at the method.
type duplicateMarkerIface interface {
	// vow:nil (!)
	// vow:nil (?)
	Dup(p *int) // want Dup:"nilDecl\\(!\\)" `vow\[nil-safety\]: interface method carries more than one vow:nil marker`
}

// malformedDeclIface carries an unrecognised token in a position
// slot. The parser rejects the payload and the diagnostic anchors
// at the method.
type malformedDeclIface interface {
	// vow:nil (xyz)
	Malformed(p *int) // want `vow\[nil-safety\]: vow:nil: vow:nil .*: param 0: expected .*got "xyz"`
}

// subjectScopedIface carries a `[subject]` scope qualifier, which
// the interface-method surface does not support (there is no
// function body to resolve the named callback against). The
// marker is rejected with a diagnostic rather than silently
// dropped.
type subjectScopedIface interface {
	// vow:nil[cb] (!)
	Scoped(cb func(*int)) // want `vow\[nil-safety\]: vow:nil: subject-scoped markers are not supported on interface methods`
}

// embeddedOnly embeds another interface without naming a method of
// its own. The embedded element carries no method name, so the
// discovery walk skips it without a panic and without exporting a
// fact for it.
type embeddedOnly interface {
	Doer
}

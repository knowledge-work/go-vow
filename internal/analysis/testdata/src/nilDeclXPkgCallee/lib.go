// Package nilDeclXPkgCallee is the callee-side fixture for the
// cross-package vow:nil trace. The declarations export Go analysis
// Facts at the callee objects (signatureNilFact at the function
// level, fieldNilFact at the struct-field level); the caller-side
// fixture (nilDeclXPkgCaller) imports the same callees and the
// analyzer reads the facts through pass.ImportObjectFact when it
// visits the caller package, so each vow:nil per-position contract
// carries across the package boundary without the caller-side
// check needing the callee's AST in scope.
package nilDeclXPkgCallee

// RequireNonNil declares both regular parameters as non-nil through
// a signature-mirror vow:nil decl. The cross-package fact pins the
// per-position contract so an importing call site that supplies a
// statically-nil argument at either slot surfaces a diagnostic.
//
// vow:nil (!,!)
func RequireNonNil(p *int, q *int) {} // want RequireNonNil:"nilDecl\\(!,!\\)"

// FirstOnly pins only the first parameter as non-nil. The second
// parameter is left at platform (empty slot), so the cross-package
// caller-side check stays silent on it even when the argument is
// the literal nil.
//
// vow:nil (!,)
func FirstOnly(p *int, q *int) {} // want FirstOnly:"nilDecl\\(!,\\)"

// NillableOK marks the parameter as nillable. The cross-package
// caller-side check stays silent on nillable positions because the
// contract explicitly admits a nil argument.
//
// vow:nil (?)
func NillableOK(p *int) {} // want NillableOK:"nilDecl\\(\\?\\)"

// Customer pins the cross-package struct-field surface. The field
// decls each export a fieldNilFact at the field's type-checker
// Object so an importing package's caller-side check resolves the
// declared nullness without re-parsing the field's doc-comment.
type Customer struct {
	// vow:nil !
	ID *string // want ID:"fieldNil\\(!\\)"

	// vow:nil ?
	Email *string // want Email:"fieldNil\\(\\?\\)"

	// Phone has no vow:nil marker so the field stays at platform
	// and the caller-side argument check never lifts it into a
	// declared-nillable proof.
	Phone *string
}

// Consume pins both regular parameters as non-nil. The cross-
// package caller-side check pulls the signatureNilFact at the
// callee and the fieldNilFact at each field-access argument; a
// `?`-declared field flowing into a `!`-required slot surfaces the
// nillable-field diagnostic.
//
// vow:nil (!,!)
func Consume(id, email *string) {} // want Consume:"nilDecl\\(!,!\\)"

// ReturnNonNil declares the only return slot as non-nil through a
// signature-mirror vow:nil decl. The fact carries the Returns
// shape across the package boundary; the cross-package caller-
// side return-position check is not currently covered, but
// the fixture pins that the Returns field of NilSignature
// round-trips through gob without being silently dropped at the
// boundary.
//
// vow:nil () !
func ReturnNonNil() *int { // want ReturnNonNil:"nilDecl\\(\\) !"
	x := 1
	return &x
}

// Service is the receiver type whose vow:nil recv-prefixed
// methods pin the cross-package recv-decl shape on the exported
// fact. The receiver-side caller check is wired through a
// separate path that the cross-package recv decl does not
// drive, so the fixture's role is to pin the gob round-trip of
// the Recv field on NilSignature.
type Service struct{}

// RecvNonNil declares the receiver as non-nil through a recv-
// prefixed signature-mirror decl. The fact attaches at the
// method's type-checker Object; the want comment pins the
// rendered Recv prefix so a future regression that drops the
// Recv field would surface here.
//
// vow:nil !.()
func (s *Service) RecvNonNil() {} // want RecvNonNil:"nilDecl!\\.\\(\\)"

// RecvNillable declares the receiver as nillable through a recv-
// prefixed signature-mirror decl. Same intent as RecvNonNil for
// the `?` token: the round-trip of the Recv field is the surface
// under check.
//
// vow:nil ?.()
func (s *Service) RecvNillable() {} // want RecvNillable:"nilDecl\\?\\.\\(\\)"

// Doer is the cross-package interface-method fixture. Do's marker
// exports a signatureNilFact at the method's type-checker Object
// exactly as a free function's marker does, so an importing
// package's caller-side check resolves the per-position contract
// through pass.ImportObjectFact without the interface's AST in
// scope.
type Doer interface {
	// vow:nil (!)
	Do(p *int) // want Do:"nilDecl\\(!\\)"
}

// GenBox pins the cross-package field surface on a generic type. The
// marker sits on the field of the generic type, so the exported fact
// is keyed by that field's Object; an importing package reaching the
// field through an instantiation sees a substituted Object and has to
// resolve it back to this one to read the declaration.
type GenBox[T any] struct {
	// vow:nil ?
	Maybe *T // want Maybe:"fieldNil\\(\\?\\)"
}

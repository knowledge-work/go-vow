// Package nilDeclField pins the struct-field surface for vow:nil:
// each field carries its own optional decl in the doc-comment block
// directly above the field, and the caller-side argument check
// reads the declaration when a field-access expression rides into a
// non-nil required position at the call site. Fields with no marker
// stay at platform and the analyzer does not track them.
package nilDeclField

// Customer pins the three field-level nullness states the surface
// supports : non-nil (`!`), nillable (`?`), and the platform
// state (no marker, no tracking).
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

// consume pins both regular parameters as non-nil through a
// signature-mirror declaration. Caller-side checks at the call
// sites below read the param contract and then consult each
// argument's field-level decl before deciding whether to surface a
// diagnostic.
//
// vow:nil (!,!)
func consume(id, email *string) {} // want consume:"nilDecl\\(!,!\\)"

// silentField pins the reading side of a `!` field decl: the
// construction side owes the field a non-nil member, and that
// obligation is what lets a reader hand the field to a `!`-required
// parameter with no guard of its own. The selector expression rides
// into both parameters directly so the declared-`!` branch of
// argumentIsDeclaredNillableField is exercised here.
//
// The `var c Customer` above leaves ID nil, which is a violation of
// the construction side rather than of the read this function pins.
// Catching it needs definite-assignment reasoning over the local, so
// the construction checks that read a literal do not cover it.
func silentField() {
	var c Customer
	consume(c.ID, c.ID)
}

// nillableFieldArg pins the path where a `?`-declared field is
// passed into a `!`-required slot. The caller-side check pulls the
// field decl through fieldNilSet and reports the position as
// potentially nil so the author either rephrases the call site or
// proves the value non-nil before the call.
func nillableFieldArg() {
	var c Customer
	consume(c.Email, c.ID)    // want `vow\[nil-safety\]: argument 1 \(id\) is potentially nil \(field Email declared nillable via vow:nil\)`
	consume(c.ID, c.Email)    // want `vow\[nil-safety\]: argument 2 \(email\) is potentially nil \(field Email declared nillable via vow:nil\)`
	consume(c.Email, c.Email) // want `vow\[nil-safety\]: argument 1 \(id\) is potentially nil \(field Email declared nillable via vow:nil\)` `vow\[nil-safety\]: argument 2 \(email\) is potentially nil \(field Email declared nillable via vow:nil\)`
}

// platformFieldArg pins the path where a platform-state field rides
// into a `!`-required slot. The caller-side check stays silent
// because platform fields contribute no proof; the user keeps the
// option to upgrade the field to `!` or `?` without disturbing
// the silent baseline.
func platformFieldArg() {
	var c Customer
	consume(c.Phone, c.Phone)
}

// MalformedField has a vow:nil marker whose payload does not parse.
// The analyzer surfaces a diagnostic at the field so the author
// notices the typo. The platform-state on the field is preserved
// (the bad parse does not register a decl), so caller-side checks
// stay silent at every call site that reads MalformedField.Bad.
type MalformedField struct {
	// vow:nil (xyz)
	Bad *string // want `vow\[nil-safety\]: vow:nil: vow:nil .*: field: expected .*got "\(xyz\)"`
}

// DuplicateField carries two vow:nil markers on the same field.
// The first parsed decl wins; the second emits a duplicate-marker
// diagnostic so the author resolves the redundancy.
type DuplicateField struct {
	// vow:nil !
	// vow:nil ?
	Dup *string // want Dup:"fieldNil\\(!\\)" `vow\[nil-safety\]: field carries more than one vow:nil marker`
}

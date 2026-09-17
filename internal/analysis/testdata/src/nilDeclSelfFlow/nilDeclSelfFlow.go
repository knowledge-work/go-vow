// Package nilDeclSelfFlow pins the transitive nil-flow checks that
// land on call-argument and return-value sites. A value that the
// AST-only self-validation pass cannot prove nil (alias chains,
// phi merges, a `?`-declared field read) is lifted into a flow
// proof by the SSA walk and surfaces as a diagnostic at the use
// site when the position the value rides into pins `!`.
package nilDeclSelfFlow

// Container carries a `?` field so a field read can seed a
// nillable flow without writing a nil literal at the call site.
type Container struct {
	// vow:nil ?
	Maybe *int // want Maybe:"fieldNil\\(\\?\\)"
}

// consume pins both regular parameters as non-nil; the flow walk
// must surface a diagnostic on any nillable argument that rides
// into either position.
//
// vow:nil (!,!)
func consume(a, b *int) {} // want consume:"nilDecl\\(!,!\\)"

// produceNonNil pins the return slot as `!`. The flow walk must
// surface a diagnostic on any nillable return expression at that
// slot.
//
// vow:nil () !
func produceNonNil() *int { // want produceNonNil:"nilDecl\\(\\) !"
	x := 1
	return &x
}

// callArgFromField pins a field-read flow: c.Maybe is declared
// `?`, and the alias `m` carries the nillable state into the call
// site. The AST-only pass cannot prove the chain — the SSA walk
// does.
func callArgFromField(c *Container) {
	m := c.Maybe
	consume(m, m) // want `vow\[nil-safety\]: argument 1 \(a\) may be nil through flow` `vow\[nil-safety\]: argument 2 \(b\) may be nil through flow`
}

// returnFromNillableField pins a field-read flow into the return
// slot: the function's own signature decl pins `() !`, and the
// returned alias is a `?`-declared field load.
//
// vow:nil () !
func returnFromNillableField(c *Container) *int { // want returnFromNillableField:"nilDecl\\(\\) !"
	m := c.Maybe
	return m // want `vow\[nil-safety\]: return position 1 may be nil through flow`
}

// silentFromNonNilFlow pins the silent path where the value
// reaching the call is provably non-nil through an address-of
// expression. The flow walk classifies the value as NonNil and
// stays silent on both arguments.
func silentFromNonNilFlow() {
	x := 1
	consume(&x, &x)
}

// silentFromUnknownFlow pins the silent path where the flow walk
// returns Unknown for the source. A plain local without a tracked
// initialiser stays Unknown so the conservative resolver does not
// fire a diagnostic on it.
func silentFromUnknownFlow(p *int) {
	consume(p, p)
}

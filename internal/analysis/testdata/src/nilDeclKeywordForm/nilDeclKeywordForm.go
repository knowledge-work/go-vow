// Package nilDeclKeywordForm pins the analyzer's reaction to the
// keyword surface (`nonnil` / `nil`) on the signature-mirror
// `vow:nil` marker. Both surfaces parse to the same nullness
// state, so the caller-side checks must fire on the keyword form
// exactly as they do on the symbol form. The fixture covers a
// pure-keyword callee, a mixed-surface callee, and the silent
// path that a nillable slot leaves at the call site.
package nilDeclKeywordForm

// Request is the argument shape used by the demo callees.
type Request struct{}

// allKeyword pins every parameter as non-nil through the keyword
// alias. The parsed signature is identical to the symbol form
// `vow:nil (!, !)`; the printer continues to surface the symbol
// because the symbol is the canonical mirror.
//
// vow:nil (nonnil,nonnil)
func allKeyword(p *int, q *Request) {} // want allKeyword:"nilDecl\\(!,!\\)"

// mixedSurface combines the keyword form on one slot with the
// symbol form on the other. Mixing is supported because each
// slot is parsed in isolation; the canonical printout remains in
// the symbol form.
//
// vow:nil (nonnil,?)
func mixedSurface(p *int, q *Request) {} // want mixedSurface:"nilDecl\\(!,\\?\\)"

// nilSlot uses the keyword `nil` on a single parameter to spell
// the nillable state. The caller-side check stays silent because
// the slot explicitly admits a nil argument.
//
// vow:nil (nil)
func nilSlot(p *int) {} // want nilSlot:"nilDecl\\(\\?\\)"

// okCalls walks the silent path: every argument is either an
// explicit address-of or sits in a slot the callee left at the
// nillable state.
func okCalls() {
	a := 1
	b := Request{}
	allKeyword(&a, &b)
	mixedSurface(&a, nil)
	nilSlot(nil)
}

// nilLiteralArgs exercises the literal-nil proof path against
// keyword-form declarations. The diagnostic surface is identical
// to the symbol form so a downstream tool that scrapes the
// message does not need to learn the keyword.
func nilLiteralArgs() {
	allKeyword(nil, nil)   // want `vow\[nil-safety\]: argument 1 \(p\) is nil` `vow\[nil-safety\]: argument 2 \(q\) is nil`
	mixedSurface(nil, nil) // want `vow\[nil-safety\]: argument 1 \(p\) is nil`
}

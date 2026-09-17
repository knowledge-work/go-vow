package analysis

import (
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// signatureNilFact is the analysis.Fact exported for every function
// the current package declares with a `vow:nil` signature. The fact
// carries the parsed nullness shape so importing packages can drive
// the caller-side argument and receiver nil checks without the
// callee's AST in scope. The same fact lookup surfaces a per-
// position contract only when the callee's signature mirror pinned
// it explicitly (`!` or `?`); platform slots stay unconstrained.
//
// The payload reuses dsl.NilSignature directly so the encoder and
// decoder round-trip the same shape the same-package map carries.
// A future receiver / return-position payload extension would land
// inside dsl.NilSignature; the fact shape would not need to change.
type signatureNilFact struct {
	Signature *dsl.NilSignature
}

// AFact admits signatureNilFact to the Go analysis Facts mechanism.
// The empty body matches the convention every analysis.Fact
// implementation uses.
func (*signatureNilFact) AFact() {}

// String renders the fact in cache dumps and diagnostic reports.
// The rendering mirrors the source-level vow:nil payload so a
// reader who recognises the marker grammar recognises the fact.
func (f *signatureNilFact) String() string {
	if f == nil || f.Signature == nil {
		return "nilDecl()"
	}
	return "nilDecl" + renderNilSignature(f.Signature)
}

// renderNilSignature builds a compact one-line rendering of sig so
// the cache dump and the analysistest want-comments stay readable.
// The rendering follows the same surface form ParseNilSignature
// accepts: an optional receiver prefix, a parenthesised parameter
// list, and an optional return list separated by a single space.
func renderNilSignature(sig *dsl.NilSignature) string {
	var b strings.Builder
	if sig.Recv != nil {
		b.WriteString(renderPositionDecl(sig.Recv))
		b.WriteByte('.')
	}
	b.WriteByte('(')
	renderPositionDeclList(&b, sig.Params)
	b.WriteByte(')')
	if len(sig.Returns) > 0 {
		b.WriteByte(' ')
		renderPositionDeclList(&b, sig.Returns)
	}
	return b.String()
}

// renderPositionDeclList writes a comma-separated rendering of
// decls into b. The parameter and return list share the same
// shape (an index-aligned slice of optional decls) so the rendering
// folds into one helper used twice — once for the parenthesised
// parameter list, once for the trailing return list.
func renderPositionDeclList(b *strings.Builder, decls []*dsl.PositionDecl) {
	for i, d := range decls {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(renderPositionDecl(d))
	}
}

// renderPositionDecl returns the surface tokens a vow:nil author
// writes for d's layers, outermost first. A nil pointer renders to
// the empty string so an unconstrained (platform) slot prints as the
// empty token in the rendered list, matching the source-level form.
func renderPositionDecl(d *dsl.PositionDecl) string {
	if d == nil {
		return ""
	}
	out := d.Nullness.String()
	for _, layer := range d.InnerLayers {
		out += layer.String()
	}
	return out
}

// exportSignatureNilFacts publishes a signatureNilFact for every
// function whose vow:nil signature the same-package discovery
// built. The fact goes out once per object so importers that
// traverse the same package multiple times observe a stable shape.
// Functions whose discovery yielded a nil pointer (parse failure
// already surfaced a diagnostic at the function declaration) are
// skipped so a malformed marker never leaks an empty fact.
func exportSignatureNilFacts(pass *analysis.Pass, state *passState) {
	for obj, sig := range state.nilDeclSet(pass) {
		if sig == nil {
			continue
		}
		pass.ExportObjectFact(obj, &signatureNilFact{Signature: sig})
	}
}

// nilDeclForFunc returns the vow:nil signature for obj. The same-
// package map is consulted first (it is the source of truth during
// the current pass); otherwise the helper falls back to the
// imported fact so cross-package callees stay recognised under the
// same predicate. The two paths are kept symmetrical so a future
// fact-payload extension can land without rewriting the caller-
// side dispatch.
func nilDeclForFunc(pass *analysis.Pass, state *passState, obj types.Object) *dsl.NilSignature {
	if obj == nil {
		return nil
	}
	if sig, ok := state.nilDeclSet(pass)[obj]; ok && sig != nil {
		return sig
	}
	var fact signatureNilFact
	if pass.ImportObjectFact(obj, &fact) {
		return fact.Signature
	}
	return nil
}

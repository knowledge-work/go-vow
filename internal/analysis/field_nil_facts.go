package analysis

import (
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// fieldNilFact is the analysis.Fact exported for every struct
// field the current package declares with a `vow:nil` field
// marker. The fact carries the parsed PositionDecl so importing
// packages can read the declared nullness off the field's
// type-checker Object even when the field's *ast.Field is out of
// scope. The cross-package trace mirrors the signature-level
// signatureNilFact path at field granularity: the same fact lookup
// surfaces a nillable-field proof only when the declaring package
// pinned the field as `?`.
//
// Only fields declared on an exported struct type and named with
// an exported identifier are eligible to carry a Go analysis Fact
// (ExportObjectFact silently drops unexported objects). The same
// constraint applies to signatureNilFact for unexported functions;
// the caller-side check degrades to the platform fallback in those
// cases, which matches what an unexported field can be reached
// from in the importing package.
type fieldNilFact struct {
	Decl *dsl.PositionDecl
}

// AFact admits fieldNilFact to the Go analysis Facts mechanism.
// The empty body matches the convention every analysis.Fact
// implementation uses.
func (*fieldNilFact) AFact() {}

// String renders the fact in cache dumps and diagnostic reports.
// The rendering wraps the surface nullness token so a reader who
// recognises the vow:nil field grammar recognises the fact.
func (f *fieldNilFact) String() string {
	if f == nil || f.Decl == nil {
		return "fieldNil()"
	}
	return "fieldNil(" + renderPositionDecl(f.Decl) + ")"
}

// exportFieldNilFacts publishes a fieldNilFact for every struct
// field whose vow:nil decl the same-package discovery built. The
// fact goes out once per object so importers that traverse the
// same package multiple times observe a stable shape. Fields whose
// discovery yielded a nil pointer (parse failure already surfaced
// a diagnostic at the field) are skipped so a malformed marker
// never leaks an empty fact.
func exportFieldNilFacts(pass *analysis.Pass, state *passState) {
	for obj, decl := range state.fieldNilSet(pass) {
		if decl == nil {
			continue
		}
		pass.ExportObjectFact(obj, &fieldNilFact{Decl: decl})
	}
}

// fieldNilForField returns the vow:nil field decl for obj. The
// same-package map is consulted first (it is the source of truth
// during the current pass); otherwise the helper falls back to
// the imported fact so cross-package field accesses stay
// recognised under the same predicate.
//
// A field reached through an instantiated generic type is a distinct
// Object from the one the declaration was registered against: the
// type checker substitutes the type arguments and hands back a
// separate *types.Var per instantiation. Neither the map nor the
// fact is keyed by those, so the lookup retries against the generic
// origin, which is the Object the declaration walk saw. The retry
// runs last so a decl registered directly against obj always wins.
func fieldNilForField(pass *analysis.Pass, state *passState, obj types.Object) *dsl.PositionDecl {
	if obj == nil {
		return nil
	}
	if decl, ok := state.fieldNilSet(pass)[obj]; ok && decl != nil {
		return decl
	}
	var fact fieldNilFact
	if pass.ImportObjectFact(obj, &fact) {
		return fact.Decl
	}
	return fieldNilForGenericOrigin(pass, state, obj)
}

// fieldNilForGenericOrigin resolves obj's generic origin and returns
// the decl registered against it, or nil when obj is not an
// instantiated field. Origin returns the receiver itself for a field
// that carries no instantiation, so the identity comparison is what
// stops the recursion.
func fieldNilForGenericOrigin(pass *analysis.Pass, state *passState, obj types.Object) *dsl.PositionDecl {
	field, ok := obj.(*types.Var)
	if !ok {
		return nil
	}
	origin := field.Origin()
	if origin == nil || origin == obj {
		return nil
	}
	return fieldNilForField(pass, state, origin)
}

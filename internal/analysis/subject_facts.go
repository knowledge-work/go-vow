package analysis

import (
	"go/types"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
	"github.com/knowledge-work/go-vow/internal/seq"
)

// subjectFact is the analysis.Fact exported on an exported
// package-level var whose doc carries a preset's annotation marker,
// such as `vow:define @Sentinel`. Markers lists the markers the
// declaration carries, so a package that refers to the var adds it to
// the subject set of each preset whose marker is listed, and checks
// its references the same way the declaring package checks its own.
type subjectFact struct {
	Markers []string
}

// AFact admits subjectFact to the Go analysis Facts mechanism.
func (*subjectFact) AFact() {}

// String renders the fact for the analysistest fact expectations.
func (f *subjectFact) String() string {
	return "subject(" + strings.Join(f.Markers, ", ") + ")"
}

// exportSubjectFacts publishes a subjectFact on every exported
// package-level var whose doc carries an annotation marker of one of
// presets. An unexported var cannot be named from another package,
// so it needs no fact.
func exportSubjectFacts(pass *analysis.Pass, presets []*dsl.Preset) {
	markers := seq.ChainOf(presets...).
		FlatMap(func(p *dsl.Preset) seq.Chain[string] { return seq.ChainOf(annotationMarkers(p)...) }).
		UniqFunc(func(m string) string { return m }).
		ToSlice()
	if len(markers) == 0 {
		return
	}
	for obj, carried := range declaredSubjectMarkers(pass, markers) {
		if !obj.Exported() {
			continue
		}
		pass.ExportObjectFact(obj, &subjectFact{Markers: carried})
	}
}

// importedSubjects returns the vars from other packages that the
// package refers to and whose subjectFact lists one of markers.
func importedSubjects(pass *analysis.Pass, markers []string) subjectSet {
	return seq.NewChain(maps.Values(pass.TypesInfo.Uses)).
		Filter(func(obj types.Object) bool {
			v, ok := obj.(*types.Var)
			return ok && !v.IsField() && v.Pkg() != pass.Pkg
		}).
		UniqFunc(func(obj types.Object) types.Object { return obj }).
		Filter(func(obj types.Object) bool {
			var fact subjectFact
			if !pass.ImportObjectFact(obj, &fact) {
				return false
			}
			return slices.ContainsFunc(fact.Markers, func(m string) bool {
				return slices.Contains(markers, m)
			})
		}).
		ToSet(func(obj types.Object) types.Object { return obj })
}

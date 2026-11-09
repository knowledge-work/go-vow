package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/knowledge-work/go-vow/internal/dsl"
	"github.com/knowledge-work/go-vow/internal/seq"
)

// subjectSet is a set of types.Object values that match at least one of
// a preset's subject selectors.
type subjectSet map[types.Object]struct{}

func (s subjectSet) contains(obj types.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := s[obj]
	return ok
}

// findSubjects returns the set of objects that satisfy one of preset's
// Subjects: the package's own package-level vars whose doc carries one
// of the preset's annotation markers, and the vars from other packages
// that the package refers to and whose subjectFact lists one of those
// markers.
func findSubjects(pass *analysis.Pass, preset *dsl.Preset) subjectSet {
	markers := annotationMarkers(preset)
	if len(markers) == 0 {
		return nil
	}
	found := importedSubjects(pass, markers)
	for obj := range declaredSubjectMarkers(pass, markers) {
		found[obj] = struct{}{}
	}
	return found
}

// declaredSubjectMarkers maps each of the package's own package-level
// vars whose doc carries one of markers to the markers it carries.
func declaredSubjectMarkers(pass *analysis.Pass, markers []string) map[*types.Var][]string {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	found := map[*types.Var][]string{}
	filter := []ast.Node{(*ast.GenDecl)(nil)}

	insp.Preorder(filter, func(n ast.Node) {
		gd := n.(*ast.GenDecl)
		if gd.Tok != token.VAR {
			return
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			carried := carriedMarkers(docOf(gd, vs), markers)
			if len(carried) == 0 {
				continue
			}
			for _, ident := range vs.Names {
				obj, ok := pass.TypesInfo.Defs[ident].(*types.Var)
				if !ok || obj == nil {
					continue
				}
				if !isPackageLevel(obj) {
					continue
				}
				found[obj] = carried
			}
		}
	})

	return found
}

func annotationMarkers(preset *dsl.Preset) []string {
	return seq.ChainOf(preset.Subjects...).
		FilterMap(func(s dsl.Subject) (string, bool) {
			marker := s.AnnotationMarker()
			return marker, marker != ""
		}).
		ToSlice()
}

// docOf returns the doc comment attached to a var declaration, falling
// back from the GenDecl's group comment to the ValueSpec's own comment
// (used when multiple vars share a `var ( ... )` block).
func docOf(gd *ast.GenDecl, vs *ast.ValueSpec) *ast.CommentGroup {
	if gd.Doc != nil {
		return gd.Doc
	}
	return vs.Doc
}

// carriedMarkers returns the members of markers that begin a line of
// doc, in the order markers lists them. Anchoring to the start of a
// line — rather than matching anywhere inside the comment text — keeps
// the rule from misfiring on prose that merely mentions a marker (e.g.
// "this variable has no vow:define @Sentinel annotation").
func carriedMarkers(doc *ast.CommentGroup, markers []string) []string {
	if doc == nil {
		return nil
	}
	lines := seq.ChainOf(strings.Split(doc.Text(), "\n")...).Map(strings.TrimSpace).ToSlice()
	return seq.ChainOf(markers...).
		Filter(func(m string) bool {
			return slices.ContainsFunc(lines, func(line string) bool {
				return markerLineMatches(line, m)
			})
		}).
		ToSlice()
}

// markerLineMatches reports whether line carries the configured
// marker. A line matches when it is exactly the marker or when the
// marker is followed by a space — the trailing-content prefix form
// the rest of the analyzer treats as a payload boundary. A run of
// spaces or tabs counts as one space, so `vow:define  @Sentinel`
// selects the variable as the Closable reader does for its types.
func markerLineMatches(line, marker string) bool {
	line = strings.Join(strings.Fields(line), " ")
	marker = strings.Join(strings.Fields(marker), " ")
	return line == marker || strings.HasPrefix(line, marker+" ")
}

func isPackageLevel(obj *types.Var) bool {
	pkg := obj.Pkg()
	if pkg == nil {
		return false
	}
	return obj.Parent() == pkg.Scope()
}

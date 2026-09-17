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

// findSubjects walks the package and returns the set of objects that
// satisfy one of preset's Subjects. The current implementation
// recognises annotation markers on package-level var declarations.
func findSubjects(pass *analysis.Pass, preset *dsl.Preset) subjectSet {
	markers := annotationMarkers(preset)
	if len(markers) == 0 {
		return nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	found := subjectSet{}
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
			if !hasAnyMarker(docOf(gd, vs), markers) {
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
				found[obj] = struct{}{}
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

// hasAnyMarker reports whether the doc comment contains a line that
// begins with one of the requested markers. Anchoring to the start
// of a line — rather than matching anywhere inside the comment
// text — keeps the rule from misfiring on prose that merely mentions
// a marker (e.g. "this variable has no vow:define @Sentinel
// annotation").
func hasAnyMarker(doc *ast.CommentGroup, markers []string) bool {
	if doc == nil {
		return false
	}
	for _, line := range strings.Split(doc.Text(), "\n") {
		line = strings.TrimSpace(line)
		if slices.ContainsFunc(markers, func(m string) bool {
			return markerLineMatches(line, m)
		}) {
			return true
		}
	}
	return false
}

// markerLineMatches reports whether line carries the configured
// marker. A line matches when it is exactly the marker or when the
// marker is followed by a space — the trailing-content prefix form
// the rest of the analyzer treats as a payload boundary.
func markerLineMatches(line, marker string) bool {
	return line == marker || strings.HasPrefix(line, marker+" ")
}

func isPackageLevel(obj *types.Var) bool {
	pkg := obj.Pkg()
	if pkg == nil {
		return false
	}
	return obj.Parent() == pkg.Scope()
}

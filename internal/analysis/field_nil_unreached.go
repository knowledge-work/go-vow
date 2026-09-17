package analysis

import (
	"go/ast"
	"slices"

	"golang.org/x/tools/go/analysis"
)

// validateFieldNilMarkerReach reports a `vow:nil` field marker the decl
// collection never reads: no fact is exported and no check consults it.
//
// The two positions get separate diagnostics because the way out
// differs — naming an anonymous struct type brings its fields in, while
// an embedded field has no name to key a decl against and its nullness
// belongs to the embedded type's own fields.
//
// Reachability is asked of collectedStructTypes, the helper the
// collection itself walks, so the two cannot disagree about what is
// tracked.
func validateFieldNilMarkerReach(pass *analysis.Pass, state *passState) {
	collected := collectedStructTypes(pass)
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			reportUnreachedMarkers(pass, st, collected[st])
			return true
		})
	}
}

// reportUnreachedMarkers emits a diagnostic for every field of st that
// carries a nil marker the collection will not read.
func reportUnreachedMarkers(pass *analysis.Pass, st *ast.StructType, stCollected bool) {
	for _, field := range st.Fields.List {
		if !fieldCarriesNilMarker(field) {
			continue
		}
		if len(field.Names) == 0 {
			vowReportNilSafetyf(
				pass,
				field.Pos(),
				"vow[nil-safety]: %s on an embedded field is not tracked; declare the nullness on the embedded type's own fields",
				nilDeclMarker,
			)
			continue
		}
		if stCollected {
			continue
		}
		vowReportNilSafetyf(
			pass,
			field.Pos(),
			"vow[nil-safety]: %s on a field of an anonymous struct type is not tracked; give the struct type a name to declare on its fields",
			nilDeclMarker,
		)
	}
}

// fieldCarriesNilMarker reports whether field's comments hold a nil
// marker line. The payload is not parsed: a malformed marker at an
// unreached position should surface the reach diagnostic alone, since a
// parse error would name a contract nothing was going to read.
func fieldCarriesNilMarker(field *ast.Field) bool {
	return slices.ContainsFunc(fieldCommentLines(field), func(comment string) bool {
		_, ok := stripMarkerPrefix(trimCommentMarker(comment), nilDeclMarker)
		return ok
	})
}

package analysis

import (
	"go/token"
	"path/filepath"
	"slices"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/cli"
)

// shouldDropByNarrowScope reports whether diag falls outside the narrow
// scope published via --changed-files / --with-callers. When no narrow
// scope is in effect (empty ChangedFileSet) every diagnostic passes through
// unchanged. Otherwise the filter keeps a diagnostic that originates inside
// a changed file, any diagnostic inside a 1-hop importer package, and a
// diagnostic at a call site whose callee is declared in a changed file when
// it sits in an unchanged file of that package.
func shouldDropByNarrowScope(pass *analysis.Pass, diag analysis.Diagnostic) bool {
	set := cli.Active()
	if set.Empty() {
		return false
	}
	diagFile := normalizedFile(pass, diag.Pos)
	if matchesAny(set.Files, diagFile) {
		return false
	}
	inCallerPkg := matchesAny(set.CallerPackages, pass.Pkg.Path())
	if !inCallerPkg && !matchesAny(set.ChangedPackages, pass.Pkg.Path()) {
		return true
	}
	call := enclosingCallExpr(pass, diag.Pos)
	if call == nil {
		// A 1-hop importer entered the scope only by calling into a changed
		// file, so everything it carries is in scope. The package owning the
		// changed file would be loaded regardless, so an unchanged file there
		// keeps only what the per-call-site rule below matches.
		return !inCallerPkg
	}
	callee := calleeObject(pass, call)
	if callee == nil {
		return true
	}
	calleeFile := normalizedFile(pass, callee.Pos())
	if matchesAny(set.Files, calleeFile) {
		return false
	}
	return true
}

// narrowAnnotationPrefix leads the related-information entry that
// carries a narrow-scope input. A driver reading the entry matches on
// this prefix; vow's own output never prints related information, so the
// entry is invisible to a reader of the diagnostics.
const narrowAnnotationPrefix = "vow:narrow "

// narrowAnnotation returns the narrow-scope inputs for diag as a
// related-information message. The inputs are the ones
// shouldDropByNarrowScope consults beyond the diagnostic's own position
// and package: whether the diagnostic sits inside a call, and where the
// callee it names is declared. Neither depends on the changed-file set,
// so a driver can cache a package's diagnostics once and still decide
// the scope question per run.
func narrowAnnotation(pass *analysis.Pass, diag analysis.Diagnostic) string {
	call := enclosingCallExpr(pass, diag.Pos)
	if call == nil {
		return narrowAnnotationPrefix + "call=none"
	}
	callee := calleeObject(pass, call)
	if callee == nil {
		return narrowAnnotationPrefix + "callee=unknown"
	}
	return narrowAnnotationPrefix + "callee-file=" + normalizedFile(pass, callee.Pos())
}

// normalizedFile returns the cleaned filename associated with pos, or an
// empty string when pos has no recorded source location.
func normalizedFile(pass *analysis.Pass, pos token.Pos) string {
	position := pass.Fset.Position(pos)
	if position.Filename == "" {
		return ""
	}
	return filepath.Clean(position.Filename)
}

// matchesAny reports whether candidate equals any string in paths.
func matchesAny(paths []string, candidate string) bool {
	if candidate == "" {
		return false
	}
	return slices.Contains(paths, candidate)
}

package analysis

import (
	"errors"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateCondAnnotationResolution reports a vow:cond line whose
// rule reference does not resolve. The annotation pipeline drops
// a line it cannot turn into a condition so the rest of the file
// still analyses; an unresolved `@alias.Rule[...]` therefore
// leaves the author with a contract that reads as enforced while
// nothing checks it, and a clean run says nothing about the gap.
// The alias most often goes missing because the vow:import
// marker sits in a comment group the package clause does not
// own.
//
// Only a payload carrying a reference is inspected, so a marker
// name at the start of a prose line (see the authoring tip in
// docs/annotations.md) stays as silent as it is today.
//
// The check runs on the raw doc line rather than the cached
// conditions because a line that fails to parse never becomes a
// condition to inspect. Diagnostics anchor at the declaration,
// matching the subject-scope surface that reports the sibling
// typo shape.
func validateCondAnnotationResolution(pass *analysis.Pass, state *passState) {
	for _, file := range pass.Files {
		scope := state.ruleScope(pass)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}
			for _, c := range fn.Doc.List {
				line := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
				marker, subject, payload, ok := splitDocMarker(line)
				if !ok || !strings.Contains(payload, "@") {
					continue
				}
				_, err := parsePayloadByMarker(marker, payload, scope)
				if !errors.Is(err, dsl.ErrUnknownReference) {
					continue
				}
				vowReportf(
					pass,
					fn.Pos(),
					"vow[sentinel-error]: %s%s payload %q was not applied: %v",
					marker,
					formatSubjectChain(subject),
					payload,
					err,
				)
			}
		}
	}
}

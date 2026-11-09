package analysis

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateCondAnnotationResolution reports a vow:cond line that
// does not parse or whose rule reference does not resolve. The
// annotation pipeline drops a line it cannot turn into a condition
// so the rest of the file still analyses; such a line therefore
// leaves the author with a contract that reads as enforced while
// nothing checks it, and a clean run says nothing about the gap.
// An alias most often goes missing because the vow:import marker
// sits in a comment group the package clause does not own.
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
				// A bare marker is also a line that does not parse, so
				// match the marker with or without a payload.
				chain, payload, ok := stripMarkerScopeChain(line, rulesMarker)
				if !ok {
					continue
				}
				_, err := parsePayloadByMarker(rulesMarker, payload, scope)
				if err == nil {
					continue
				}
				vowReportf(
					pass,
					fn.Pos(),
					"vow[sentinel-error]: %s%s payload %q was not applied: %v",
					rulesMarker,
					formatSubjectChain(dsl.SubjectChain(chain)),
					payload,
					err,
				)
			}
		}
	}
}

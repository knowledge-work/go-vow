package analysis

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// reportMustConsume implements the "must-consume" obligation type.
//
// Every subject identifier reference inside the package is classified
// against the AST stack via classifyReference. References classified
// as observe — discharged through errors.Is / errors.As / equality /
// switch case inside a conditional — are silent. References classified
// as chain — sitting inside a ReturnStmt whose enclosing function
// authorises propagation via vow:cond — are also silent. Every
// other reference is reported, so leaks are distinguished from
// deliberate propagation.
func reportMustConsume(pass *analysis.Pass, preset *dsl.Preset, subjects subjectSet, obligation dsl.Obligation, state *passState) {
	if len(subjects) == 0 {
		return
	}
	observerNames := obligation.MustConsumeConsumers()
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	filter := []ast.Node{(*ast.Ident)(nil)}
	insp.WithStack(filter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return false
		}
		ident := n.(*ast.Ident)
		// Skip the declaration site itself: the sentinel introduces the
		// obligation here, it does not consume or escape one.
		if pass.TypesInfo.Defs[ident] != nil {
			return true
		}
		// Skip subject references that sit on the RHS of a short-form
		// alias binding (`x := ErrFoo`). The reference is not a real
		// use of the sentinel — it just names the value the alias
		// resolver will follow when classifying the alias's own uses.
		// Diagnosing here would double-count the leak (once at the
		// binding, once at the eventual return).
		if isAliasBindingRHS(stack) {
			return true
		}
		subjectName, isSubject := resolveSubject(pass, stack, ident, subjects)
		if !isSubject {
			return true
		}
		if classifyReference(pass, stack, observerNames, subjectName, state) == classLeak {
			vowReportMustConsumef(
				pass,
				ident.Pos(),
				subjectName,
				"vow[%s]: sentinel error %s leaked: needs observation or explicit propagation",
				preset.Name,
				subjectName,
			)
		}
		return true
	})
}

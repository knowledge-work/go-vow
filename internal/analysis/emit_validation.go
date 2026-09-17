package analysis

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// userEmit walks every FuncDecl in pass.Files and returns a map
// from each function's type-checker Object to the subjects it
// declares it may emit via the vow:emit marker. A function whose
// doc carries multiple vow:emit lines contributes the union of
// declared subjects; duplicates fold so downstream consumers
// iterate without de-duplicating.
//
// The discovery reads the marker payload directly; vow:emit is the
// primary declarative surface for emission and does not route
// through the atom-rule resolver.
func userEmit(pass *analysis.Pass) map[types.Object][]string {
	result := map[types.Object][]string{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}
			subjects := collectEmitSubjects(pass, fn)
			if len(subjects) == 0 {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			if obj == nil {
				continue
			}
			result[obj] = subjects
		}
	}
	return result
}

// collectEmitSubjects walks fn's doc comments for vow:emit markers
// and returns the union of declared subjects. An authored-but-
// empty or malformed marker surfaces a diagnostic at fn.Pos so the
// author sees the missing payload alongside the contract
// declaration. Markers that carry a `[<scope>]` qualifier whose
// subject does not name a receiver, parameter, or named return of
// fn surface a chain-unresolved diagnostic at fn.Pos for the same
// reason.
func collectEmitSubjects(pass *analysis.Pass, fn *ast.FuncDecl) []string {
	var subjects []string
	seen := map[string]struct{}{}
	for _, c := range fn.Doc.List {
		text, isLine := lineCommentBody(c)
		if !isLine {
			continue
		}
		scope, specs, present := parseEmitMarkerLine(text)
		if !present {
			continue
		}
		if len(specs) == 0 {
			vowReportf(pass, fn.Pos(),
				"vow[sentinel-error]: vow:emit requires at least one subject name (use vow:emit X[, Y, ...])")
			continue
		}
		if len(scope) > 0 {
			_, failedAt, status := resolveSignatureSubjectChain(pass, fn, scope)
			switch status {
			case chainResolveFailed:
				reportChainSubjectUnresolved(pass, fn.Pos(), emitMarker, scope, failedAt)
			}
			// Subject-scoped declarations describe a higher-order
			// contract a layer wires to executable
			// semantics; folding the names into the function-level
			// emission set would misroute them as the enclosing
			// function's own chain-auth sources. The chain-step
			// diagnostic (intermediate non-callback) and the
			// final-not-callback diagnostic are surfaced by
			// validateSubjectScopedEmits so this collector keeps
			// its responsibility narrow.
			continue
		}
		for _, spec := range specs {
			if spec.Kind == EmitPassthrough {
				// The passthrough form declares a wrap relationship
				// (`name -> $N`) without naming a specific subject.
				// The caller-side destination walk consumes the
				// wrap-through shape; the function-level emission
				// set stays focused on named subjects.
				continue
			}
			if _, dup := seen[spec.Subject]; dup {
				continue
			}
			seen[spec.Subject] = struct{}{}
			subjects = append(subjects, spec.Subject)
		}
	}
	return subjects
}

// validateEmitCallees emits a diagnostic for every vow:emit X
// function whose body never returns a value that resolves to
// subject X. A site counts as an emission when a return-statement
// carries an identifier whose name matches X (including one-hop
// local aliases) or when a return forwards the result of a callee
// whose vow:emit declaration covers X. Functions whose body is nil
// (interface declarations, externally-implemented functions) are
// skipped — the declaration is vacuously a contract on someone
// else's body.
func validateEmitCallees(pass *analysis.Pass, state *passState) {
	emit := state.emitSet(pass)
	if len(emit) == 0 {
		return
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			subjects, ok := emit[obj]
			if !ok {
				continue
			}
			for _, subject := range subjects {
				if functionEmitsSubject(pass, fn, subject, emit) {
					continue
				}
				vowReportf(
					pass,
					fn.Pos(),
					"vow[sentinel-error]: vow:emit %s declaration is not self-validated: function body never returns %s",
					subject,
					subject,
				)
			}
		}
	}
}

// functionEmitsSubject reports whether fn.Body contains at least
// one return statement that emits subject. Two shapes count: a
// return result that is the subject identifier itself (or a local
// alias chain to it), and a return whose call expression resolves
// to a callee whose vow:emit declaration covers subject. The walk
// returns at the first match so a single qualifying return
// satisfies the contract.
func functionEmitsSubject(pass *analysis.Pass, fn *ast.FuncDecl, subject string, emit map[types.Object][]string) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range ret.Results {
			if expressionEmitsSubject(pass, fn, result, subject, emit) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// expressionEmitsSubject reports whether expr resolves to subject
// either as a direct identifier (or through a one-hop local alias)
// or as a call to a function whose vow:emit declaration covers
// subject. The recognition is deliberately narrow: dynamic
// dispatch, struct literals, and arbitrary composite expressions
// return false so the self-validation only credits emissions the
// analyzer can statically trace.
func expressionEmitsSubject(pass *analysis.Pass, fn *ast.FuncDecl, expr ast.Expr, subject string, emit map[types.Object][]string) bool {
	expr = resolveExprAlias(fn, expr)
	switch v := expr.(type) {
	case *ast.Ident:
		if v.Name == subject {
			return true
		}
	case *ast.SelectorExpr:
		if v.Sel != nil && v.Sel.Name == subject {
			return true
		}
	case *ast.CallExpr:
		obj := calleeObject(pass, v)
		if obj == nil {
			return false
		}
		subjects, ok := emit[obj]
		if !ok {
			return false
		}
		return containsSubject(subjects, subject)
	}
	return false
}

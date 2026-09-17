package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
)

// transducerSpec captures the subjects a `vow:use X`-annotated
// function transduces to bool. A function may list multiple
// subjects on its `vow:use` line; each subject lands in this
// list, so the classifier can decide whether a call-site
// discharges a specific reference.
type transducerSpec struct {
	// subjects holds the identifier names the transducer reports on.
	// Each entry is matched against the subject's display name at
	// classification time.
	subjects []string
}

// observes reports whether spec transduces the named subject. The
// caller asks "does this vow:use function report on the subject I
// am classifying?" — only when the answer is yes does the call site
// count as a (b) discharge for that subject.
func (s transducerSpec) observes(subjectName string) bool {
	return slices.Contains(s.subjects, subjectName)
}

// userUseTransducers walks every FuncDecl in pass.Files and
// returns the subset of vow:use declarations that also satisfy
// the single-bool-return signature requirement. The pair of
// `vow:use X` and a bool return promotes the function to
// transducer status for X: a call in conditional context counts
// as a (b) discharge for the matching subject. Multi-subject
// markers (`vow:use ErrFoo, ErrBar`) register the function as a
// transducer for every listed subject so a single call site can
// discharge any one of them depending on the reference. A
// declaration whose signature is not a single bool is silently
// skipped here; the callee-side discharge contract still runs
// through userUse, so a non-bool `vow:use X` keeps its self-
// validation obligation without contributing to the transducer
// set.
func userUseTransducers(pass *analysis.Pass) map[types.Object]transducerSpec {
	result := map[types.Object]transducerSpec{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}
			subjects := collectUseSubjects(pass, fn)
			if len(subjects) == 0 {
				continue
			}
			if !returnsSingleBool(pass, fn) {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			if obj == nil {
				continue
			}
			result[obj] = transducerSpec{subjects: subjects}
		}
	}
	return result
}

// userUse walks every FuncDecl in pass.Files and returns a map
// from each function's type-checker Object to the subjects its
// function-doc vow:use marker declares for callee-side discharge.
// A function whose doc carries multiple vow:use lines contributes
// the union of declared subjects; duplicates fold so downstream
// consumers iterate without de-duplicating.
//
// The discovery reads the marker payload directly; vow:use is the
// primary declarative surface for callee-side discharge and does
// not route through any atom-rule resolver.
func userUse(pass *analysis.Pass) map[types.Object][]string {
	result := map[types.Object][]string{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}
			subjects := collectUseSubjects(pass, fn)
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

// collectUseSubjects walks fn's doc comments for vow:use markers
// and returns the union of declared subjects. Authored-but-empty
// markers and subject-scope resolution diagnostics are surfaced
// through the dedicated discoverUseHints pass, so this collector
// stays silent on those shapes to keep the diagnostic
// responsibility in one place. Markers that carry a `[<scope>]`
// qualifier never contribute to the function-level discharge set;
// the contract belongs to the layer that wires the scoped
// assertion through to executable semantics.
func collectUseSubjects(_ *analysis.Pass, fn *ast.FuncDecl) []string {
	var subjects []string
	seen := map[string]struct{}{}
	for _, c := range fn.Doc.List {
		text, isLine := lineCommentBody(c)
		if !isLine {
			continue
		}
		scope, names, present := parseUseHintLine(text)
		if !present || len(names) == 0 {
			continue
		}
		if len(scope) > 0 {
			continue
		}
		for _, name := range names {
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			subjects = append(subjects, name)
		}
	}
	return subjects
}

// validateUseCallees emits a diagnostic for every vow:use X
// function whose body lacks any discharge site for X. A site counts
// as a discharge when the subject identifier sits in a conditional
// position, when it appears as an argument to errors.Is/As, when
// the function hands the subject off to another vow:use function,
// or when a statement-scope vow:use marker inside the body
// declares the same subject.
func validateUseCallees(pass *analysis.Pass, state *passState) {
	use := state.useSet(pass)
	if len(use) == 0 {
		return
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			subjects, ok := use[obj]
			if !ok {
				continue
			}
			for _, subject := range subjects {
				if functionDischargesSubject(pass, state, fn, subject, use) {
					continue
				}
				vowReportf(
					pass,
					fn.Pos(),
					"vow[sentinel-error]: vow:use %s declaration is not self-validated: function body has no discharge site for %s",
					subject,
					subject,
				)
			}
		}
	}
}

// functionDischargesSubject reports whether fn.Body contains at
// least one site that discharges subject. The walk returns at the
// first match so a single qualifying site satisfies the contract.
// Four recognised sites: an identifier reference in conditional
// context, an errors.Is/As call carrying the subject, a handoff
// call to another `vow:use subject` function, or a statement-scope
// `vow:use` marker that names the subject.
func functionDischargesSubject(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, subject string, use map[types.Object][]string) bool {
	if stmtScopeUseAssertsSubject(pass, state, fn, subject) {
		return true
	}
	found := false
	var stack []ast.Node
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return true
		}
		stack = append(stack, n)
		if found {
			return false
		}
		switch v := n.(type) {
		case *ast.Ident:
			if v.Name == subject && identIsInConditional(stack) {
				found = true
				return false
			}
		case *ast.CallExpr:
			if calleeIsUseFor(pass, v, subject, use) {
				found = true
				return false
			}
			if callIsErrorsIsAsFor(pass, v, subject) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// identIsInConditional reports whether the identifier at the top
// of stack sits in the cond / init / tag position of an enclosing
// if or switch. Boolean glue (parens, !, &&, ||, ==, !=, IndexExpr,
// TypeAssertExpr, AssignStmt Init) is walked through so a subject
// reference buried in a comparison still counts. Call expressions
// intentionally do not transit; a subject reference nested inside
// a call argument requires a transducer, handoff, or errors.Is/As
// recognition to count.
func identIsInConditional(stack []ast.Node) bool {
	for i := len(stack) - 2; i >= 0; i-- {
		parent := stack[i]
		child := stack[i+1]
		switch x := parent.(type) {
		case *ast.IfStmt:
			return child == x.Cond || child == x.Init
		case *ast.SwitchStmt:
			return child == x.Tag || child == x.Init
		case *ast.CaseClause:
			return slices.ContainsFunc(x.List, func(e ast.Expr) bool {
				return child == e
			})
		case *ast.ParenExpr, *ast.BinaryExpr, *ast.UnaryExpr, *ast.IndexExpr, *ast.TypeAssertExpr, *ast.AssignStmt:
			_ = x
			continue
		default:
			return false
		}
	}
	return false
}

// calleeIsUseFor reports whether call's callee is a function
// annotated `vow:use subject`. The handoff site is sufficient for
// self-validation: the callee owns the discharge contract for
// subject regardless of which value the caller supplies for the
// subject-typed argument, so the enclosing function does not need
// a direct reference to the subject identifier in its body.
func calleeIsUseFor(pass *analysis.Pass, call *ast.CallExpr, subject string, use map[types.Object][]string) bool {
	obj := calleeObject(pass, call)
	if obj == nil {
		return false
	}
	subjects, ok := use[obj]
	if !ok {
		return false
	}
	return containsSubject(subjects, subject)
}

// callIsErrorsIsAsFor reports whether call is `errors.Is(_, X)` or
// `errors.As(_, &X)` where the subject-shaped argument matches
// subject. The package binding is verified through types.Info so a
// dot-imported or shadowed `errors` package does not match
// accidentally. Calls with fewer than two arguments fall through —
// `errors.Is(err)` is a static error elsewhere; here it simply
// fails the discharge gate.
func callIsErrorsIsAsFor(pass *analysis.Pass, call *ast.CallExpr, subject string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return false
	}
	if sel.Sel.Name != "Is" && sel.Sel.Name != "As" {
		return false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	pkgName, ok := pass.TypesInfo.Uses[pkgIdent].(*types.PkgName)
	if !ok || pkgName.Imported().Path() != "errors" {
		return false
	}
	if len(call.Args) < 2 {
		return false
	}
	return argReferencesSubject(call.Args[1], subject)
}

// argReferencesSubject reports whether expr names subject either
// directly, through a selector tail, or through a single address-of
// (`&X`). The recognition mirrors the surface forms
// `errors.Is(err, ErrFoo)` and `errors.As(err, &target)` use; deeper
// expression shapes are not credited because the analyzer cannot
// statically confirm they resolve to the named subject.
func argReferencesSubject(expr ast.Expr, subject string) bool {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name == subject
	case *ast.SelectorExpr:
		return v.Sel != nil && v.Sel.Name == subject
	case *ast.UnaryExpr:
		if v.Op != token.AND {
			return false
		}
		return argReferencesSubject(v.X, subject)
	}
	return false
}

// stmtScopeUseAssertsSubject reports whether any statement-scope
// `vow:use` marker inside fn.Body names subject. The discoverUseHints
// pass already registers every line-scoped vow:use into
// passState.useAssertedLines; this helper scans the body's line
// range for a matching entry instead of re-walking comments.
func stmtScopeUseAssertsSubject(pass *analysis.Pass, state *passState, fn *ast.FuncDecl, subject string) bool {
	if fn.Body == nil {
		return false
	}
	startPos := pass.Fset.Position(fn.Body.Pos())
	endPos := pass.Fset.Position(fn.Body.End())
	return state.lineRangeAssertsUseOf(startPos.Filename, startPos.Line, endPos.Line, subject)
}

// calleeObject extracts the type-checker Object of a CallExpr's
// callee, supporting both bare-identifier calls and selector calls.
// Returns nil for dynamic dispatch or unresolved selectors so the
// caller can treat the call as opaque.
func calleeObject(pass *analysis.Pass, call *ast.CallExpr) types.Object {
	var ident *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		ident = fun.Sel
	}
	if ident == nil {
		return nil
	}
	return pass.TypesInfo.ObjectOf(ident)
}

// containsSubject reports whether subjects contains name. The
// slice is tiny in practice (typically one or two entries) so a
// linear scan stays faster than building a set.
func containsSubject(subjects []string, name string) bool {
	return slices.Contains(subjects, name)
}

// returnsSingleBool reports whether fn's signature is exactly one
// return value whose Go type resolves to the predeclared bool kind.
// The check is types.Info-driven rather than purely syntactic, so a
// defined boolean type (`type Matched bool`) or a true alias
// (`type Matched = bool`) is accepted — call sites that recognise
// the predicate by Object lookup still match, and rejecting them
// here would leave call-site recognition and registration
// disagreeing on which functions qualify.
func returnsSingleBool(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	if fn.Type == nil || fn.Type.Results == nil {
		return false
	}
	list := fn.Type.Results.List
	if len(list) != 1 {
		return false
	}
	field := list[0]
	if len(field.Names) > 1 {
		return false
	}
	t := pass.TypesInfo.TypeOf(field.Type)
	if t == nil {
		return false
	}
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

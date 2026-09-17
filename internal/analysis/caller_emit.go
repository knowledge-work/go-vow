package analysis

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// emitDestination tags the AST context an authored statement-scope
// `vow:emit X` marker sits in. Authors place the marker next to the
// site that hands the Closable value off; the inferred destination
// drives the diagnostic wording when the analyzer cannot match a
// recognised hand-off shape.
type emitDestination int

const (
	emitDestinationUnknown emitDestination = iota
	emitDestinationGoroutine
	emitDestinationCallback
	emitDestinationStorage
	emitDestinationChannel
	emitDestinationWrappedReturn
)

// discoverCallerEmit walks pass.Files for statement-scope vow:emit
// markers and registers the asserted subject names plus the
// inferred destination context. The function-doc vow:emit form
// remains the callee-side declaration and is handled by std_emit;
// this pass focuses on the caller-side hand-off surface that
// discharges a Closable acquisition.
func discoverCallerEmit(pass *analysis.Pass, state *passState) {
	for _, file := range pass.Files {
		discoverCallerEmitInFile(pass, state, file)
	}
}

func discoverCallerEmitInFile(pass *analysis.Pass, state *passState, file *ast.File) {
	docComments := collectFuncDocComments(file)
	stmtStartLines := indexStatementStartLines(pass.Fset, file)
	for _, group := range file.Comments {
		for _, c := range group.List {
			if _, isFuncDoc := docComments[c]; isFuncDoc {
				continue
			}
			text, isLine := lineCommentBody(c)
			if !isLine {
				continue
			}
			scope, subjects, present := parseEmitMarkerLine(text)
			if !present {
				continue
			}
			if len(subjects) == 0 {
				vowReportf(pass, c.Pos(), "vow[closable]: vow:emit requires at least one subject name (use vow:emit X[, Y, ...])")
				continue
			}
			if len(scope) > 0 {
				enclosing := enclosingFuncDeclAt(file, c.Pos())
				if enclosing == nil {
					vowReportf(pass, c.Pos(), "vow[closable]: %s%s is anchored outside a function declaration", emitMarker, formatSubjectChain(scope))
				} else if _, failedAt, status := resolveSignatureSubjectChain(pass, enclosing, scope); status == chainResolveFailed {
					reportCallerEmitChainUnresolved(pass, c.Pos(), scope, failedAt)
				}
				// chainStepFailed and the final-not-callback case
				// surface through validateStatementScopeEmitSubjects
				// under `vow[sentinel-error]`, so this discovery
				// site stays focused on the unresolved-chain path
				// it owns under `vow[closable]`.
				// Subject-scoped emission belongs to the named
				// callback, not the anchored statement, so the
				// destination inference below would surface a
				// second diagnostic for a line whose handoff
				// shape is irrelevant to the scoped contract.
				continue
			}
			commentPos := pass.Fset.Position(c.Pos())
			targetLine, anchored := resolveSuppressLine(stmtStartLines, commentPos.Line)
			if !anchored {
				continue
			}
			destination := inferEmitDestination(pass, state, file, targetLine)
			if destination == emitDestinationUnknown {
				vowReportf(pass, c.Pos(), "vow[closable]: vow:emit destination could not be inferred; place the marker next to a goroutine launch, callback argument, storage assignment, channel send, or a wrapped return registered through a passthrough-emit preset")
			}
		}
	}
}

// inferEmitDestination returns the destination context for a
// statement-scope vow:emit marker by inspecting the AST node that
// owns the marker's anchored line. The priority order — goroutine,
// callback, storage, channel, wrapped return — keeps the most
// specific destination from being masked by a broader match (for
// example, a goroutine that also writes to shared state still
// resolves as goroutine). The wrapped-return shape sits at the
// bottom of the order because the four AST-only shapes always
// reach a syntactic decision; the wrapped-return shape needs the
// passthrough-emit preset registration to commit, so the priority
// keeps a goroutine launch that happens to return a wrapped error
// reading as a goroutine. emitDestinationUnknown indicates none
// of the recognised shapes covered the site, which the discovery
// walker reports as a warning so the author either adjusts the
// placement or annotates the suppression explicitly.
func inferEmitDestination(pass *analysis.Pass, state *passState, file *ast.File, line int) emitDestination {
	enclosing := findStmtAtLine(pass, file, line)
	if enclosing == nil {
		return emitDestinationUnknown
	}
	if _, isGo := enclosing.(*ast.GoStmt); isGo {
		return emitDestinationGoroutine
	}
	if stmtIsInsideGoroutine(state, file, enclosing) {
		return emitDestinationGoroutine
	}
	if stmtContainsCallableArgCall(pass, enclosing) {
		return emitDestinationCallback
	}
	if call, ok := closableEnclosingCallExpr(state, file, enclosing); ok && callPassesCallable(pass, call) {
		return emitDestinationCallback
	}
	if stmtIsStorageWrite(enclosing) {
		return emitDestinationStorage
	}
	if _, ok := enclosing.(*ast.SendStmt); ok {
		return emitDestinationChannel
	}
	if stmtIsWrappedReturn(pass, state, enclosing) {
		return emitDestinationWrappedReturn
	}
	return emitDestinationUnknown
}

// stmtIsWrappedReturn reports whether the enclosing statement is
// a return whose return expression is a call to a function
// registered through the passthrough-emit obligation of a loaded
// preset. The matcher walks the return's result expressions
// (which may be a single call or one of several positions) and
// matches each call's fully-qualified function name against the
// registered patterns.
func stmtIsWrappedReturn(pass *analysis.Pass, state *passState, stmt ast.Stmt) bool {
	ret, ok := stmt.(*ast.ReturnStmt)
	if !ok {
		return false
	}
	wraps := passthroughEmitFunctions(state)
	if len(wraps) == 0 {
		return false
	}
	return slices.ContainsFunc(ret.Results, func(result ast.Expr) bool {
		return callMatchesWrapFunction(pass, result, wraps)
	})
}

// callMatchesWrapFunction reports whether expr is a CallExpr whose
// callee resolves to a function whose fully-qualified name is on
// the wrap list. Non-call expressions and calls whose callee does
// not resolve through the type checker fall through unmatched.
func callMatchesWrapFunction(pass *analysis.Pass, expr ast.Expr, wraps []dsl.PassthroughEmitFunction) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	fqn := calleeFullName(pass, call)
	if fqn == "" {
		return false
	}
	return slices.ContainsFunc(wraps, func(w dsl.PassthroughEmitFunction) bool {
		return w.Pattern == fqn
	})
}

// calleeFullName returns the `<import-path>.Name` string that
// identifies a call's callee, or "" when the callee does not
// resolve to a top-level function (for example, a method call
// through a value, a function literal invocation, or a call
// whose type information is missing).
func calleeFullName(pass *analysis.Pass, call *ast.CallExpr) string {
	obj := calleeObject(pass, call)
	if obj == nil {
		return ""
	}
	pkg := obj.Pkg()
	if pkg == nil {
		return ""
	}
	return pkg.Path() + "." + obj.Name()
}

// passthroughEmitFunctions returns the union of wrap signatures
// registered across every passthrough-emit obligation on the
// loaded presets. Each preset that drops a passthrough-emit
// obligation in contributes its entries to the same list so the
// resolver treats every loaded preset uniformly.
func passthroughEmitFunctions(state *passState) []dsl.PassthroughEmitFunction {
	if state == nil {
		return nil
	}
	var out []dsl.PassthroughEmitFunction
	for _, p := range state.presets {
		for _, o := range p.Obligations {
			out = append(out, o.PassthroughEmitFunctions()...)
		}
	}
	return out
}

// stmtContainsCallableArgCall reports whether stmt contains a
// CallExpr whose argument list passes a callable expression. The
// statement-scope vow:emit marker can sit on the same line as an
// expression-statement wrapping the call, so the destination
// inference walks down from the statement to find the contained
// CallExpr before checking arguments.
func stmtContainsCallableArgCall(pass *analysis.Pass, stmt ast.Stmt) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if callPassesCallable(pass, call) {
			found = true
			return false
		}
		return true
	})
	return found
}

// findStmtAtLine returns the smallest statement in file whose
// position falls on line. The marker resolver already mapped the
// comment to its anchored statement line, so a missing statement
// here means the comment anchored to a non-statement source line
// (rare; reported as unknown destination by the caller).
func findStmtAtLine(pass *analysis.Pass, file *ast.File, line int) ast.Stmt {
	var match ast.Stmt
	ast.Inspect(file, func(n ast.Node) bool {
		stmt, ok := n.(ast.Stmt)
		if !ok || stmt == nil {
			return true
		}
		pos := pass.Fset.Position(stmt.Pos())
		if pos.Line == line {
			match = stmt
			return false
		}
		return true
	})
	return match
}

// stmtIsInsideGoroutine reports whether stmt sits inside a GoStmt's
// function-literal body. The walk traverses file ancestors via the
// shared parent map state.parentMap caches per file, so two
// statement-scope checks on the same file share a single walk.
func stmtIsInsideGoroutine(state *passState, file *ast.File, stmt ast.Stmt) bool {
	parents := state.parentMap(file)
	current := ast.Node(stmt)
	for current != nil {
		parent, ok := parents[current]
		if !ok {
			return false
		}
		if _, isGo := parent.(*ast.GoStmt); isGo {
			return true
		}
		current = parent
	}
	return false
}

// closableEnclosingCallExpr returns the nearest CallExpr that contains
// stmt. Statement-scope vow:emit markers next to a function call
// receive a callback destination when the call passes a callable
// expression as one of its arguments. The walk reads the same
// parent map state.parentMap caches, so paired calls from
// inferEmitDestination on the same file share the walk result with
// stmtIsInsideGoroutine.
func closableEnclosingCallExpr(state *passState, file *ast.File, stmt ast.Stmt) (*ast.CallExpr, bool) {
	parents := state.parentMap(file)
	current := ast.Node(stmt)
	for current != nil {
		parent, ok := parents[current]
		if !ok {
			return nil, false
		}
		if call, ok := parent.(*ast.CallExpr); ok {
			return call, true
		}
		current = parent
	}
	return nil, false
}

// callPassesCallable reports whether call has at least one argument
// whose static type is a function. The check uses pass.TypesInfo so
// method values, function literals, and named callable types all
// qualify as callbacks; non-callable arguments leave the call
// unmatched and the caller continues with the next destination.
func callPassesCallable(pass *analysis.Pass, call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		argType := pass.TypesInfo.TypeOf(arg)
		if argType == nil {
			continue
		}
		if _, isSig := argType.Underlying().(*types.Signature); isSig {
			return true
		}
	}
	return false
}

// stmtIsStorageWrite reports whether stmt writes a value into
// shared state. The recognised shapes are an AssignStmt whose LHS
// is a selector expression (struct field) or an index expression
// (slice/map element). Other AssignStmt shapes belong to the
// general-purpose discharge path and do not signal hand-off.
func stmtIsStorageWrite(stmt ast.Stmt) bool {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok {
		return false
	}
	for _, lhs := range assign.Lhs {
		switch lhs.(type) {
		case *ast.SelectorExpr, *ast.IndexExpr:
			return true
		}
	}
	return false
}

// buildParentMap returns a map from every AST node in file to its
// immediate parent. The walker is a single pass over the file's
// AST; callers typically reach it through `state.parentMap` so the
// same file's parent map is built once per pass.
func buildParentMap(file *ast.File) map[ast.Node]ast.Node {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return true
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return parents
}

// emitDischargesForFunction returns the set of identifier names
// that statement-scope vow:emit markers inside fn.Body assert as
// discharged. The Closable lifecycle check uses this set in
// parallel with the deferred Close receiver set so any of defer
// Close, vow:use, or vow:emit clears the obligation.
//
// The walk anchors each candidate comment through the same
// resolveSuppressLine path the discovery pass uses: a comment
// counts only when it sits on a statement-start line (trailing)
// or directly above one (leading), and the resolved statement
// line must itself fall inside fn.Body's range. A bare comment
// that happens to land in fn.Body's line range without anchoring
// to a statement, or a comment that lives in a different source
// file whose line numbering accidentally overlaps fn's, no
// longer credits a discharge. Function-doc comments and
// subject-scoped vow:emit markers are skipped so the function-
// level discharge set covers only the body-anchored emissions
// that target the enclosing function.
func emitDischargesForFunction(pass *analysis.Pass, fn *ast.FuncDecl) map[string]struct{} {
	out := map[string]struct{}{}
	if fn.Body == nil {
		return out
	}
	file := fileContainingDecl(pass, fn)
	if file == nil {
		return out
	}
	bodyStartLine := pass.Fset.Position(fn.Body.Pos()).Line
	bodyEndLine := pass.Fset.Position(fn.Body.End()).Line
	docComments := collectFuncDocComments(file)
	stmtStartLines := indexStatementStartLines(pass.Fset, file)
	for _, group := range file.Comments {
		for _, c := range group.List {
			if _, isFuncDoc := docComments[c]; isFuncDoc {
				continue
			}
			commentPos := pass.Fset.Position(c.Pos())
			if commentPos.Line < bodyStartLine || commentPos.Line > bodyEndLine {
				continue
			}
			text, isLine := lineCommentBody(c)
			if !isLine {
				continue
			}
			scope, subjects, present := parseEmitMarkerLine(text)
			if !present {
				continue
			}
			if len(scope) > 0 {
				// Subject-scoped emission threads through the
				// named callback rather than the enclosing
				// function, so folding the names here would
				// silence the wrong obligation.
				continue
			}
			targetLine, anchored := resolveSuppressLine(stmtStartLines, commentPos.Line)
			if !anchored {
				continue
			}
			if targetLine < bodyStartLine || targetLine > bodyEndLine {
				continue
			}
			for _, spec := range subjects {
				if spec.Kind == EmitPassthrough {
					continue
				}
				name := strings.TrimSpace(spec.Subject)
				if name == "" {
					continue
				}
				out[name] = struct{}{}
			}
		}
	}
	return out
}

// fileContainingDecl returns the *ast.File that owns decl, or nil
// when no file in pass.Files contains the declaration. The
// matching anchors on decl's positions against each file's
// half-open extent so a function-level helper can read comments
// or statement-start lines without scanning unrelated files.
func fileContainingDecl(pass *analysis.Pass, decl ast.Decl) *ast.File {
	if decl == nil {
		return nil
	}
	for _, file := range pass.Files {
		if file.Pos() <= decl.Pos() && decl.End() <= file.End() {
			return file
		}
	}
	return nil
}

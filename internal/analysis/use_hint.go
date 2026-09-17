package analysis

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
	"github.com/knowledge-work/go-vow/internal/seq"
)

// useHintMarker is the doc-comment marker that asserts a caller-side
// discharge for one or more named subjects. The author claims "X was
// used here" so the analyzer drops the must-consume diagnostic for X
// at the asserted scope without consulting the (a)/(b)/(c) discharge
// chain. The marker is reason-less by design: an assertion declares
// a fact about behaviour the analyzer cannot statically observe, not
// a debt the codebase has to repay.
const useHintMarker = "vow:use"

// parseUseHintLine inspects a trimmed comment-body line and reports
// the subject names a vow:use assertion claims, when present. The
// first return value is the optional signature-scope chain
// qualifier; an empty chain means the marker carried no `[scope]`
// and the assertion applies to the enclosing function in the usual
// way. The marker accepts a comma-separated list so a single
// comment can assert discharge for several subjects in one hop.
// Whitespace-only or empty payloads carry no subjects and are
// reported as present=true with subjects=nil so the caller can
// surface a diagnostic against an authored-but-empty marker
// instead of silently skipping it. Return order follows Go
// convention: value, presence.
func parseUseHintLine(line string) (scope dsl.SubjectChain, subjects []string, present bool) {
	chain, body, ok := stripMarkerScopeChain(line, useHintMarker)
	if !ok {
		return nil, nil, false
	}
	scope = dsl.SubjectChain(chain)
	if body == "" {
		return scope, nil, true
	}
	parts := strings.Split(body, ",")
	out := seq.ChainOf(parts...).
		FilterMap(func(p string) (string, bool) {
			name := strings.TrimSpace(p)
			return name, name != ""
		}).
		ToSlice()
	if len(out) == 0 {
		return scope, nil, true
	}
	return scope, out, true
}

// discoverUseHints walks pass.Files for vow:use markers and
// registers the subject assertions in state. Function-doc occurrences
// register a function-scoped assertion; line comments anchored to a
// statement (trailing or leading, mirroring vow:suppress) register
// a line-scoped assertion. Authored-but-empty markers (`// vow:use`
// with no subjects) surface a parser diagnostic at the enclosing
// function or comment so the author sees the malformed claim
// instead of seeing the original leak reappear.
func discoverUseHints(pass *analysis.Pass, state *passState) {
	for _, file := range pass.Files {
		discoverUseHintsInFile(pass, state, file)
	}
}

func discoverUseHintsInFile(pass *analysis.Pass, state *passState, file *ast.File) {
	docComments := collectFuncDocComments(file)
	stmtStartLines := indexStatementStartLines(pass.Fset, file)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		for _, c := range fn.Doc.List {
			text, isLine := lineCommentBody(c)
			if !isLine {
				continue
			}
			scope, subjects, present := parseUseHintLine(text)
			if !present {
				continue
			}
			if len(subjects) == 0 {
				vowReportf(pass, fn.Pos(), "vow[sentinel-error]: vow:use requires at least one subject name (use vow:use X[, Y, ...])")
				continue
			}
			if len(scope) > 0 {
				_, failedAt, status := resolveSignatureSubjectChain(pass, fn, scope)
				if status == chainResolveFailed {
					reportChainSubjectUnresolved(pass, fn.Pos(), useHintMarker, scope, failedAt)
				}
				// Subject-scoped assertions defer their executable
				// semantics to validateSubjectScopedUseHints, which
				// surfaces the chain-step and final-not-callback
				// diagnostics; the function-level discharge set
				// stays unchanged so the scoped callsite carries
				// its own obligation.
				continue
			}
			state.markFunctionUseAsserted(fn, subjects)
		}
	}
	for _, group := range file.Comments {
		for _, c := range group.List {
			if _, isFuncDoc := docComments[c]; isFuncDoc {
				continue
			}
			text, isLine := lineCommentBody(c)
			if !isLine {
				continue
			}
			scope, subjects, present := parseUseHintLine(text)
			if !present {
				continue
			}
			if len(subjects) == 0 {
				reportPos := c.Pos()
				if fn := enclosingFuncDeclAt(file, c.Pos()); fn != nil {
					reportPos = fn.Pos()
				}
				vowReportf(pass, reportPos, "vow[sentinel-error]: vow:use requires at least one subject name (use vow:use X[, Y, ...])")
				continue
			}
			if len(scope) > 0 {
				enclosing := enclosingFuncDeclAt(file, c.Pos())
				if enclosing == nil {
					vowReportf(pass, c.Pos(), "vow[sentinel-error]: %s%s is anchored outside a function declaration", useHintMarker, formatSubjectChain(scope))
				} else {
					_, failedAt, status := resolveSignatureSubjectChain(pass, enclosing, scope)
					if status == chainResolveFailed {
						reportChainSubjectUnresolved(pass, c.Pos(), useHintMarker, scope, failedAt)
					}
				}
				// Subject-scoped line markers describe a higher-
				// order discharge a layer wires to
				// executable semantics; registering them in the
				// line-level discharge set would silence diagnostics
				// on the anchored statement instead of the scoped
				// callsite.
				continue
			}
			commentPos := pass.Fset.Position(c.Pos())
			targetLine, anchored := resolveSuppressLine(stmtStartLines, commentPos.Line)
			if !anchored {
				continue
			}
			state.markLineUseAsserted(commentPos.Filename, targetLine, subjects)
		}
	}
}

// shouldDropByUseAssertion reports whether the diagnostic should be
// dropped by the caller-side vow:use assertion filter. Eligibility
// is narrow: only must-consume diagnostics whose Category encodes a
// subject pass the gate, so condition-shape and annotation-parser
// diagnostics are never silenced through this path. A match against
// the subject is required as well — `// vow:use ErrFoo` near a
// statement that also leaks ErrBar drops the ErrFoo diagnostic and
// leaves the ErrBar diagnostic in place.
func shouldDropByUseAssertion(pass *analysis.Pass, state *passState, diag analysis.Diagnostic) bool {
	if !isMustConsumeCategory(diag.Category) {
		return false
	}
	subject := subjectFromMustConsumeCategory(diag.Category)
	if subject == "" {
		return false
	}
	if state.lineAssertsUseOf(pass, diag.Pos, subject) {
		return true
	}
	return state.functionAssertsUseOf(diag.Pos, subject)
}

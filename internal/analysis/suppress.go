package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// suppressMarker is the doc-comment marker that drops a must-consume
// diagnostic from vowReport's emission path. The marker is caller-
// authored so the author who reads the leak chooses to silence it,
// and a reason is mandatory so the silenced site documents why the
// obligation is being deferred.
const suppressMarker = "vow:suppress"

// categoryMustConsume tags every must-consume diagnostic so the
// suppress filter can drop them without affecting other diagnostic
// categories (return-position, annotation-shape, parser errors).
// Authors who suppress a function still see annotation typos and
// return-position violations in the same body — suppress is the
// narrow "I read the leak, I accept the obligation" valve, not a
// blanket vow shutoff for the function.
const categoryMustConsume = "must-consume"

// categoryNilSafety tags every caller-side nil-safety diagnostic so
// the suppress filter can drop them alongside must-consume. Authors
// that read a nil-safety leak and choose to defer the fix surface a
// single `vow:suppress: <reason>` marker; routing every vow-emitted
// "I read the leak" diagnostic through the same valve keeps the
// suppression surface uniform across the lint categories the
// analyzer tracks.
const categoryNilSafety = "nil-safety"

// categoryNilDecl tags every declaration-completeness diagnostic —
// the ones that name a nillable position no `vow:nil` decl covers.
// The category is distinct from nil-safety so a CI integration can
// separate "vow proved a nil reaches here" from "the contract at this
// position is unwritten", which an adopter mid-migration wants to
// treat as advisory while the safety violations stay blocking. The
// separation lives in the category rather than in a severity level
// because go/analysis carries no severity axis.
const categoryNilDecl = "nil-decl"

// suppressLineKey identifies a source line for line-level suppression.
// Filename plus line number is the smallest pair that uniquely
// identifies a source line across the per-pass FileSet without
// retaining the column / offset noise that token.Position carries.
type suppressLineKey struct {
	filename string
	line     int
}

// suppressFuncRange identifies a function declaration for function-
// level suppression. The pair of token.Pos values is the half-open
// AST range; a diagnostic position that lies inside the range
// inherits the suppression, including positions inside nested
// closures since the spec leaves "cascading into nested closures"
// to the simplest interpretation of "all diagnostics in this
// function body".
type suppressFuncRange struct {
	start token.Pos
	end   token.Pos
}

// parseSuppressLine inspects a trimmed comment-body line and reports
// whether it carries a vow:suppress marker. A well-formed marker
// returns the extracted reason text. A malformed marker (bare, or
// missing reason body) returns an ErrSuppressMissingReason-wrapped
// error so the caller can emit a parser diagnostic without
// registering the site as suppressed. The present return
// distinguishes "this line is unrelated to suppress" from "this line
// attempted to suppress but failed", so trailing typos like
// `vow:suppressX` flow through the analyzer untouched while a real
// `vow:suppress` marker is always either accepted or diagnosed.
// Return order follows Go convention: value, presence, error.
func parseSuppressLine(line string) (reason string, present bool, err error) {
	if line == suppressMarker {
		return "", true, suppressReasonRequiredErr()
	}
	if !strings.HasPrefix(line, suppressMarker+":") {
		return "", false, nil
	}
	body := strings.TrimSpace(line[len(suppressMarker)+1:])
	if body == "" {
		return "", true, suppressReasonRequiredErr()
	}
	return body, true, nil
}

// suppressReasonRequiredErr wraps ErrSuppressMissingReason with the
// surface text the analyzer prints for malformed markers. Wrapping
// keeps errors.Is(err, ErrSuppressMissingReason) working for callers
// that want to discriminate this failure mode programmatically.
// The function always returns a non-nil wrapped error, so the
// `| nil` in the annotation is purely the form chain-authorisation
// expects (a sum) rather than a possible runtime shape.
//
// vow:cond * -> ErrSuppressMissingReason | nil
func suppressReasonRequiredErr() error {
	return fmt.Errorf("%w: use vow:suppress: <reason>", ErrSuppressMissingReason)
}

// discoverSuppress walks pass.Files for vow:suppress markers and
// registers function-level / line-level suppressions in state.
// Bare or empty-reason markers surface a parser diagnostic at the
// enclosing function (or the comment itself, for orphan markers)
// without registering the site as suppressed. The parser error
// itself flows through vowReport untouched because the suppress
// filter only drops must-consume diagnostics, so the bare-marker
// diagnostic is never recursively swallowed.
func discoverSuppress(pass *analysis.Pass, state *passState) {
	for _, file := range pass.Files {
		discoverSuppressInFile(pass, state, file)
	}
}

func discoverSuppressInFile(pass *analysis.Pass, state *passState, file *ast.File) {
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
			_, present, err := parseSuppressLine(text)
			if !present {
				continue
			}
			if err != nil {
				vowReportf(pass, fn.Pos(), "vow[sentinel-error]: %s", err.Error())
				continue
			}
			state.markFunctionSuppressed(fn)
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
			_, present, err := parseSuppressLine(text)
			if !present {
				continue
			}
			if err != nil {
				reportPos := c.Pos()
				if fn := enclosingFuncDeclAt(file, c.Pos()); fn != nil {
					reportPos = fn.Pos()
				}
				vowReportf(pass, reportPos, "vow[sentinel-error]: %s", err.Error())
				continue
			}
			commentPos := pass.Fset.Position(c.Pos())
			targetLine, anchored := resolveSuppressLine(stmtStartLines, commentPos.Line)
			if !anchored {
				continue
			}
			state.markLineSuppressed(commentPos.Filename, targetLine)
		}
	}
}

// lineCommentBody returns the trimmed body of a `//`-prefixed comment
// with the slashes stripped. Block comments (`/* */`) are out of
// scope for vow:suppress: the marker family is restricted to line
// comments so trailing-on-statement detection has a single,
// unambiguous source position. Callers see isLine=false for block
// comments and move on without parsing.
func lineCommentBody(c *ast.Comment) (body string, isLine bool) {
	if !strings.HasPrefix(c.Text, "//") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(c.Text, "//")), true
}

// collectFuncDocComments returns the set of *ast.Comment entries that
// participate in function declaration doc comments. The line-level
// scan consults this set to skip comments already processed by the
// function-level loop, which keeps a single bare-marker comment from
// surfacing two parser diagnostics (one per loop).
func collectFuncDocComments(file *ast.File) map[*ast.Comment]struct{} {
	out := map[*ast.Comment]struct{}{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		for _, c := range fn.Doc.List {
			out[c] = struct{}{}
		}
	}
	return out
}

// indexStatementStartLines returns the set of source lines that host
// the start of an ast.Stmt in file. The set drives the trailing-vs-
// leading resolution for line-level suppress: a comment on line L
// is trailing when L is a statement start line, and leading when
// L+1 is. A comment with neither is treated as orphan and registers
// no suppression.
func indexStatementStartLines(fset *token.FileSet, file *ast.File) map[int]struct{} {
	out := map[int]struct{}{}
	ast.Inspect(file, func(n ast.Node) bool {
		stmt, ok := n.(ast.Stmt)
		if !ok {
			return true
		}
		line := fset.Position(stmt.Pos()).Line
		out[line] = struct{}{}
		return true
	})
	return out
}

// resolveSuppressLine converts a comment's line number to the source
// line whose diagnostics it suppresses. A trailing comment (statement
// starts on the same line) suppresses that line; a leading comment
// (statement starts on the next line) suppresses the next line. A
// comment with no statement on either line is treated as an orphan
// and the boolean reports false so the caller drops it silently.
func resolveSuppressLine(stmtStartLines map[int]struct{}, commentLine int) (int, bool) {
	if _, trailing := stmtStartLines[commentLine]; trailing {
		return commentLine, true
	}
	if _, leading := stmtStartLines[commentLine+1]; leading {
		return commentLine + 1, true
	}
	return 0, false
}

// enclosingFuncDeclAt returns the FuncDecl whose extent (doc + body)
// contains pos, or nil when pos lies outside every function in the
// file. The doc range is included because a bare-marker comment may
// sit in the doc block; anchoring the diagnostic at fn.Pos() in
// that case keeps `analysistest` `want` comments target-able on the
// declaration line.
func enclosingFuncDeclAt(file *ast.File, pos token.Pos) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		start := fn.Pos()
		if fn.Doc != nil {
			start = fn.Doc.Pos()
		}
		if start <= pos && pos <= fn.End() {
			return fn
		}
	}
	return nil
}

// shouldSuppressBySuppress reports whether the diagnostic should be
// dropped by the vow:suppress filter. Two diagnostic categories are
// eligible: must-consume (the original "I read the leak" valve) and
// nil-safety (caller-side argument and receiver nil checks). Other
// categories — condition-shape, annotation-arity, parser errors —
// flow through untouched so an author who suppresses a leak still
// sees typos or return-position violations in the same function.
func shouldSuppressBySuppress(pass *analysis.Pass, state *passState, diag analysis.Diagnostic) bool {
	if !isSuppressibleCategory(diag.Category) {
		return false
	}
	if state.lineIsSuppressed(pass, diag.Pos) {
		return true
	}
	return state.functionIsSuppressed(diag.Pos)
}

// isSuppressibleCategory reports whether cat is one of the
// diagnostic categories vow:suppress is allowed to drop. Adding a
// new category to the suppression surface is a one-line edit here;
// the filter chain and the eligibility gate stay decoupled from the
// per-category emitter helpers so a new category lands without
// touching the dispatcher.
func isSuppressibleCategory(cat string) bool {
	if isMustConsumeCategory(cat) {
		return true
	}
	return cat == categoryNilSafety || cat == categoryNilDecl
}

package analysis

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// dischargedMarker declares the doc-comment annotation that turns a
// function into an all-verification barrier: caller-side checks
// (must-consume, return-position validation, chain authorisation)
// stop at every static call to a discharged callee. The marker takes
// no payload — its presence on a function doc comment is the entire
// signal.
const dischargedMarker = "vow:discharged"

// userDischarged walks every FuncDecl in pass.Files and returns the
// type-checker Object of each function whose doc comment carries a
// well-formed vow:discharged marker. A marker with a payload is
// rejected with a diagnostic and the function is omitted from the
// set so downstream barriers cannot rely on a half-trusted entry.
func userDischarged(pass *analysis.Pass) map[types.Object]bool {
	set := map[types.Object]bool{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn.Doc == nil {
				continue
			}
			if !docCarriesMarker(fn.Doc, dischargedMarker) {
				continue
			}
			if !dischargedDocIsBare(fn.Doc) {
				vowReportf(
					pass,
					fn.Pos(),
					"vow[sentinel-error]: %s takes no payload; declare it as a bare marker line",
					dischargedMarker,
				)
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			if obj == nil {
				continue
			}
			set[obj] = true
		}
	}
	return set
}

// dischargedDocIsBare returns true when every vow:discharged line in doc
// is the bare marker with no trailing tokens. The marker is
// documented as payload-less; extra tokens usually reflect a
// confusion with vow:cond and must surface as an authoring
// error rather than be silently ignored.
func dischargedDocIsBare(doc *ast.CommentGroup) bool {
	for _, line := range strings.Split(doc.Text(), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, dischargedMarker) {
			continue
		}
		rest := strings.TrimSpace(trimmed[len(dischargedMarker):])
		if rest != "" {
			return false
		}
	}
	return true
}

// reportLineLevelDischarged emits a diagnostic for every vow:discharged
// occurrence that is not anchored to a FuncDecl doc comment. The
// marker is function-scoped by design (see the design notes); a
// line-level or non-function declaration occurrence would be
// silently ignored by the doc scan, so we surface it explicitly to
// prevent authoring drift.
func reportLineLevelDischarged(pass *analysis.Pass) {
	docComments := map[*ast.CommentGroup]struct{}{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn.Doc != nil {
				docComments[fn.Doc] = struct{}{}
			}
		}
	}
	for _, file := range pass.Files {
		for _, cg := range file.Comments {
			if _, isFuncDoc := docComments[cg]; isFuncDoc {
				continue
			}
			for _, c := range cg.List {
				text := commentLineBody(c.Text)
				if !strings.HasPrefix(text, dischargedMarker) {
					continue
				}
				vowReportf(
					pass,
					c.Pos(),
					"vow[sentinel-error]: %s is only legal in a function doc comment",
					dischargedMarker,
				)
			}
		}
	}
}

// commentLineBody strips the // or /* */ wrappers and returns the
// trimmed body of a single comment line so callers can match
// marker prefixes without re-implementing the unwrap.
func commentLineBody(raw string) string {
	text := strings.TrimPrefix(raw, "//")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")
	return strings.TrimSpace(text)
}

// isDischargedSSACall reports whether call is a static call to a
// function authored with the vow:discharged marker. The SSA backward
// walk consults this to stop at the discharged boundary so values
// originating in a discharged callee never feed a leak diagnostic.
func isDischargedSSACall(call *ssa.Call, pass *analysis.Pass, state *passState) bool {
	fn := call.Call.StaticCallee()
	if fn == nil {
		return false
	}
	obj := fn.Object()
	if obj == nil {
		return false
	}
	return state.dischargedSet(pass)[obj]
}

// isDischargedASTCall reports whether expr is a CallExpr whose callee
// resolves to a vow:discharged function. The AST consume path and
// the return-position validator query this to skip caller-side
// checks for values originating in a discharged callee.
func isDischargedASTCall(expr ast.Expr, pass *analysis.Pass, state *passState) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	var ident *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		ident = fun.Sel
	}
	if ident == nil {
		return false
	}
	obj := pass.TypesInfo.ObjectOf(ident)
	if obj == nil {
		return false
	}
	return state.dischargedSet(pass)[obj]
}

package analysis

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// reportSSALeaks complements reportMustConsume's per-identifier walk
// with an SSA-driven sweep of return statements. The AST walk in
// consume.go handles direct subject references and a single-hop
// alias (`x := ErrFoo; return x`); the cases below sit outside its
// reach and are where SSA earns its keep:
//
//   - Multi-hop alias chains: `x := ErrFoo; y := x; return y`.
//   - Reassignment after non-subject init: `x := someFn(); x = ErrFoo;
//     return x`.
//   - Branch merges: `var x error; if c { x = ErrA } else { x = ErrB };
//     return x`.
//
// For each return expression that is a bare *ast.Ident the function
// asks SSA which values can flow there. When any of those values
// resolve to a tracked subject — and the AST walk hadn't already
// named that subject at the return site, and the enclosing function
// has not authorized chain propagation — a leak is reported at the
// return expression. Per-assignment reports from the per-Ident walk
// remain unchanged so existing diagnostics stay stable; SSA only
// fills in leaks AST couldn't see.
func reportSSALeaks(pass *analysis.Pass, preset *dsl.Preset, subjects subjectSet, state *passState) {
	if len(subjects) == 0 {
		return
	}
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok || result == nil {
		return
	}
	for _, file := range pass.Files {
		scope := state.ruleScope(pass)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ssaFn := findSSAFunction(result.SrcFuncs, fn)
			if ssaFn == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				ret, isRet := n.(*ast.ReturnStmt)
				if !isRet {
					return true
				}
				for _, expr := range ret.Results {
					ident, isIdent := expr.(*ast.Ident)
					if !isIdent {
						continue
					}
					if _, named := resolveSubjectInFunc(pass, fn, ident, subjects); named {
						continue
					}
					for _, name := range ssaSubjectsAtReturn(ssaFn, ret, expr, subjects, pass, state) {
						if functionAuthorizesPropagation(pass, fn, ret, name, scope, state) {
							continue
						}
						vowReportMustConsumef(
							pass,
							expr.Pos(),
							name,
							"vow[%s]: sentinel error %s leaked: needs observation or explicit propagation",
							preset.Name,
							name,
						)
					}
				}
				return true
			})
		}
	}
}

// ssaSubjectsAtReturn returns the set of tracked subject names that
// flow into the given return result. The SSA representation is
// consulted by locating the *ssa.Return whose source position equals
// the AST ReturnStmt's position and walking the matching Results
// value backward through phi nodes and trivial loads.
func ssaSubjectsAtReturn(ssaFn *ssa.Function, ret *ast.ReturnStmt, expr ast.Expr, subjects subjectSet, pass *analysis.Pass, state *passState) []string {
	resultIdx := -1
	for i, r := range ret.Results {
		if r == expr {
			resultIdx = i
			break
		}
	}
	if resultIdx < 0 {
		return nil
	}
	ssaRet := findSSAReturn(ssaFn, ret.Pos())
	if ssaRet == nil || resultIdx >= len(ssaRet.Results) {
		return nil
	}
	names := map[string]struct{}{}
	visited := map[ssa.Value]struct{}{}
	collectSSASubjects(ssaRet.Results[resultIdx], subjects, names, visited, pass, state)
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	return out
}

// findSSAFunction returns the SSA function whose Syntax matches fn,
// or nil when no SrcFunc owns the AST. Generic instantiations share
// a single Syntax pointer with their generic origin, so this lookup
// is sufficient for the source functions in pass.Files.
func findSSAFunction(funcs []*ssa.Function, fn *ast.FuncDecl) *ssa.Function {
	for _, sf := range funcs {
		if sf.Syntax() == fn {
			return sf
		}
	}
	return nil
}

// findSSAReturn returns the *ssa.Return whose source position
// equals retPos, or nil when no return in fn matches. Returns are
// unique per source position because the parser allocates one
// ReturnStmt per `return` keyword. The walk recurses into
// `fn.AnonFuncs` so a return inside a function literal resolves to
// the closure's SSA return rather than degrading silently to nil.
func findSSAReturn(fn *ssa.Function, retPos token.Pos) *ssa.Return {
	if fn == nil {
		return nil
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			r, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}
			if r.Pos() == retPos {
				return r
			}
		}
	}
	for _, anon := range fn.AnonFuncs {
		if r := findSSAReturn(anon, retPos); r != nil {
			return r
		}
	}
	return nil
}

// collectSSASubjects walks v backward through phi nodes and UnOp
// dereference loads, inserting any *ssa.Global whose object lies in
// the subject set into names. The visited map breaks cycles created
// by phi self-references (typical for loop-carried locals) and
// avoids quadratic work on phis with duplicated edges. A static
// call to a vow:discharged function terminates the walk at that node:
// values originating in a discharged callee are trusted by the caller
// and never count as a leaked subject.
func collectSSASubjects(v ssa.Value, subjects subjectSet, names map[string]struct{}, visited map[ssa.Value]struct{}, pass *analysis.Pass, state *passState) {
	if v == nil {
		return
	}
	if _, seen := visited[v]; seen {
		return
	}
	visited[v] = struct{}{}
	switch n := v.(type) {
	case *ssa.Phi:
		for _, edge := range n.Edges {
			collectSSASubjects(edge, subjects, names, visited, pass, state)
		}
	case *ssa.UnOp:
		if n.Op == token.MUL {
			collectSSASubjects(n.X, subjects, names, visited, pass, state)
		}
	case *ssa.Call:
		if isDischargedSSACall(n, pass, state) {
			return
		}
	case *ssa.Global:
		obj := n.Object()
		if obj != nil && subjects.contains(obj) {
			names[obj.Name()] = struct{}{}
		}
	}
}

package analysis

import (
	"go/ast"
	"go/token"
	"slices"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// class is the result of classifying a single subject reference.
//
// Three classes are recognised: observe (discharged via conditional
// observer / equality / case match), chain (explicit propagation to
// the caller, gated by a listing vow:cond), and leak (everything
// else, which is reported as a diagnostic).
type class int

const (
	classLeak    class = iota // diagnose
	classObserve              // discharged via conditional observer / equality / case match
	classChain                // explicit propagation to caller
)

// classifyReference assigns a class to a subject reference based on
// the AST stack from the file root (index 0) down to the Ident itself
// (last index). observerNames lists the qualified function names that
// can discharge the obligation when invoked inside a conditional;
// subjectName is the simple identifier name of the referenced subject
// (used to consult a direct sum declared via vow:cond).
//
// Classification is ordered: observation (discharges the obligation
// in-place) is preferred over chain (which only postpones the
// obligation to the caller). Anything that is neither observed nor
// returned from a function that explicitly authorizes the subject is
// reported as a leak.
func classifyReference(pass *analysis.Pass, stack []ast.Node, observerNames []string, subjectName string, state *passState) class {
	if isUseHandoff(pass, stack, subjectName, state) {
		return classObserve
	}
	if isObserveUse(pass, stack, observerNames, subjectName, state) {
		return classObserve
	}
	scope := state.ruleScope(pass)
	if isChainUse(pass, stack, subjectName, scope, state) {
		return classChain
	}
	return classLeak
}

// isUseHandoff reports whether the subject reference at the top
// of stack is passed directly as an argument to a function annotated
// `vow:use subject`. Unlike (a) direct-conditional and
// (b) transduced-conditional, the handoff path does not require
// conditional context: the callee declares the discharge and the
// caller credits it on every call that hands the subject off.
func isUseHandoff(pass *analysis.Pass, stack []ast.Node, subjectName string, state *passState) bool {
	if len(stack) < 2 || state == nil {
		return false
	}
	parent, ok := stack[len(stack)-2].(*ast.CallExpr)
	if !ok {
		return false
	}
	ref := stack[len(stack)-1]
	if !nodeAmong(ref, parent.Args) {
		return false
	}
	obj := calleeObject(pass, parent)
	if obj == nil {
		return false
	}
	subjects, ok := state.useSet(pass)[obj]
	if !ok {
		return false
	}
	return containsSubject(subjects, subjectName)
}

// isChainUse reports whether the reference participates in an
// authorized propagation: it sits inside a ReturnStmt belonging
// to a function whose vow:cond signature lists the named subject.
// The enclosing ReturnStmt is captured
// during the stack walk and threaded through the authoriser so
// non-trivial parameter requirements can be evaluated at that
// specific return — the chain authorisation set narrows in step
// with the return-requirement enforcement set.
//
// scope brings the vow:import rule namespace into the authorisation
// check so a chain expressed through `@rule` resolves correctly.
// A return without an authorising signature is *not* a chain — it
// is treated as a leak so the rule still flags unintentional
// propagation.
func isChainUse(pass *analysis.Pass, stack []ast.Node, subjectName string, scope dsl.RuleScope, state *passState) bool {
	var ret *ast.ReturnStmt
	for i := len(stack) - 2; i >= 0; i-- {
		switch n := stack[i].(type) {
		case *ast.ReturnStmt:
			ret = n
		case *ast.FuncDecl:
			if ret == nil {
				return false
			}
			return functionAuthorizesPropagation(pass, n, ret, subjectName, scope, state)
		case *ast.FuncLit:
			if ret == nil {
				return false
			}
			// FuncLit has no doc of its own. Walk outward to find a
			// var/const declaration whose doc may carry the chain
			// authorization, or an enclosing FuncDecl whose
			// annotation applies.
			return funcLitAuthorizesPropagation(pass, stack[:i], ret, subjectName, scope, state)
		}
	}
	return false
}

// isObserveUse reports whether the subject reference participates in
// one of the recognized observation patterns and sits inside a
// conditional evaluation context.
//
// Recognized patterns:
//
//   - Argument of an observer call:        if errors.Is(err, X)  { ... }
//   - Operand of == / !=:                  if err == X           { ... }
//   - Value in a switch case list:         switch err { case X: ... }
//
// Two observer sources are consulted in the call-arg pattern: the
// preset-declared consumer names (`observerNames`, syntactically
// matched against `pkg.Func` callsites) and any package-local
// FuncDecl that carries `vow:use X` on a bool-returning signature
// (matched via types.Info on the call's function identifier). The
// preset path observes any subject because the subject is itself
// a call argument; the transducer path observes only the subjects
// the marker declares for that function, so subjectName is
// consulted when deciding whether the transducer call discharges
// the reference.
func isObserveUse(pass *analysis.Pass, stack []ast.Node, observerNames []string, subjectName string, state *passState) bool {
	for i := len(stack) - 2; i >= 0; i-- {
		parent := stack[i]
		child := stack[i+1]
		switch x := parent.(type) {
		case *ast.CallExpr:
			if isObserverCall(pass, x, observerNames, subjectName, state) && nodeAmong(child, x.Args) {
				return isInConditional(stack, i)
			}
		case *ast.BinaryExpr:
			if (x.Op == token.EQL || x.Op == token.NEQ) && (x.X == child || x.Y == child) {
				return isInConditional(stack, i)
			}
		case *ast.CaseClause:
			if nodeAmong(child, x.List) {
				// case lists are intrinsically a conditional position;
				// no further ancestor check is needed.
				return true
			}
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		}
	}
	return false
}

// isInConditional walks outward from the observation-shape node at
// stack[shapeIdx] and reports whether some ancestor is an if/switch
// with the shape sitting in its Cond / Init / Tag position.
//
// Intermediate "boolean-evaluation glue" is transparently traversed:
//
//   - parens                              `(errors.Is(...))`
//   - unary `!`                           `!errors.Is(...)`
//   - binary `&&` / `||` / `==` / `!=`    `errors.Is(...) && ...`
//   - Init-clause assignment              `if x := errors.Is(...); x`
//   - index expression                    `m[errors.Is(...)]` (rare but legal)
//   - type assertion                      `errors.Is(...).(bool)` (pathological)
//
// Other intermediate shapes (composite literals, function literals,
// arbitrary call expressions, etc.) are intentionally rejected.
// Treating them as glue would let subtle false positives in — e.g.
// counting `struct{B bool}{B: errors.Is(...)}.B` inside an `if` as
// observation even though the sentinel is buried in a value layer.
func isInConditional(stack []ast.Node, shapeIdx int) bool {
	for j := shapeIdx - 1; j >= 0; j-- {
		parent := stack[j]
		child := stack[j+1]
		switch x := parent.(type) {
		case *ast.IfStmt:
			return child == x.Cond || child == x.Init
		case *ast.SwitchStmt:
			return child == x.Tag || child == x.Init
		case *ast.AssignStmt, *ast.ParenExpr, *ast.BinaryExpr, *ast.UnaryExpr,
			*ast.IndexExpr, *ast.TypeAssertExpr:
			// Boolean-evaluation glue; keep walking outward.
		default:
			return false
		}
	}
	return false
}

// isObserverCall reports whether call invokes a recognised observer
// for the named subject. Two recognition paths are tried:
//
//  1. The call's function is a `pkg.Func` selector whose qualified
//     form matches one of the preset-declared `names`. Preset
//     consumers (errors.Is, errors.As, ...) take the subject as a
//     call argument and therefore observe any subject visible at
//     the call site, so subjectName is not consulted on this path.
//  2. The call's function resolves through types.Info to an Object
//     in the pass-wide transducer set, populated from `vow:use X`
//     declarations on bool-returning functions. A call site in
//     conditional context credits a discharge when the
//     transducer's subject matches the reference; references for
//     other subjects fall through to the leak path.
//
// The preset path is purely syntactic so it works against external
// packages without a types.Info entry. The transducer path is
// Object-resolved so it tolerates same-package calls written as
// bare identifiers and also matches across import aliases.
func isObserverCall(pass *analysis.Pass, call *ast.CallExpr, names []string, subjectName string, state *passState) bool {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if pkgIdent, ok := sel.X.(*ast.Ident); ok {
			qualified := pkgIdent.Name + "." + sel.Sel.Name
			if slices.Contains(names, qualified) {
				return true
			}
		}
	}
	obj := resolveExprObject(pass, call.Fun)
	if obj == nil || state == nil {
		return false
	}
	if spec, ok := state.userTransducerSet(pass)[obj]; ok && spec.observes(subjectName) {
		return true
	}
	return false
}

func nodeAmong(target ast.Node, list []ast.Expr) bool {
	return slices.ContainsFunc(list, func(e ast.Expr) bool {
		return e == target
	})
}

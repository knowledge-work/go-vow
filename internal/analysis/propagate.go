package analysis

import (
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// functionAuthorizesPropagation reports whether fn permits the named
// subject to be returned. The single authorisation source is a
// `vow:cond` direct-sum position that lists subjectName. The
// RuleScope is consulted when the annotation references composed
// rules via `@<alias>.<rule>[...]`.
//
// Only FuncDecl supports propagation; FuncLit has no doc and is
// handled by funcLitAuthorizesPropagation walking outward to an
// enclosing declaration. The ret argument is the specific return
// statement that produced the subject reference — passed so the
// per-case parameter-requirement scope can be evaluated at this
// return rather than at the function as a whole.
func functionAuthorizesPropagation(pass *analysis.Pass, fn ast.Node, ret *ast.ReturnStmt, subjectName string, scope dsl.RuleScope, state *passState) bool {
	decl, ok := fn.(*ast.FuncDecl)
	if !ok {
		return false
	}
	return returnAnnotationLists(pass, decl, ret, subjectName, scope, state)
}

// funcLitAuthorizesPropagation walks outward from a *ast.FuncLit
// and reports whether the nearest enclosing FuncDecl carries a
// signature that authorises the named subject at this specific
// return. A ValueSpec encountered along the way terminates the
// search: the FuncLit is bound to a value, and value-spec docs no
// longer convey chain authorisation.
func funcLitAuthorizesPropagation(pass *analysis.Pass, prefix []ast.Node, ret *ast.ReturnStmt, subjectName string, scope dsl.RuleScope, state *passState) bool {
	for j := len(prefix) - 1; j >= 0; j-- {
		switch n := prefix[j].(type) {
		case *ast.ValueSpec:
			return false
		case *ast.FuncDecl:
			return functionAuthorizesPropagation(pass, n, ret, subjectName, scope, state)
		}
	}
	return false
}

// returnAnnotationLists checks whether the parsed Conditions on
// decl authorise propagation of subjectName at the given return
// statement. Authorisation accumulates across cases: a case
// authorises the propagation when its return requirement lists
// the subject (or carries a `*` wildcard) AND its parameter
// requirement holds at this return. A trivial parameter
// requirement (nil Prereq, the `vow:cond` sugar) holds at
// every return; a non-trivial requirement is evaluated via the
// SSA dominator walk in prereqHoldsAtReturn so each return's
// chain authorisation tracks the same narrowed scope the
// return-requirement enforcement uses. A single matching case
// is enough.
func returnAnnotationLists(pass *analysis.Pass, decl *ast.FuncDecl, ret *ast.ReturnStmt, subjectName string, scope dsl.RuleScope, state *passState) bool {
	if obj := pass.TypesInfo.Defs[decl.Name]; obj != nil {
		if subjects, ok := state.emitSet(pass)[obj]; ok && containsSubject(subjects, subjectName) {
			return true
		}
	}
	conds, ok := state.annotation(decl, scope)
	if !ok {
		return false
	}
	ssaFn := findSSAFunction(ssaFunctions(pass), decl)
	var ssaRet *ssa.Return
	if ret != nil {
		ssaRet = findSSAReturnFor(ssaFn, ret.Pos())
	}
	for _, cond := range conds {
		if len(cond.Subject) > 0 {
			// Subject-scoped conditions describe a higher-order
			// contract on the named callback, not the enclosing
			// function, so they cannot authorise the enclosing
			// return's chain credit.
			continue
		}
		if cond.Prereq != nil && !prereqHoldsAtReturn(decl, ssaFn, ssaRet, cond.Prereq) {
			continue
		}
		if conditionAuthorisesSubject(cond.Conseq, subjectName) {
			return true
		}
	}
	return false
}

// conditionAuthorisesSubject reports whether the return
// requirement of a Condition lists subjectName as an admitted value
// — either inside a tuple-sum member, a position-wise direct sum,
// or as a Single `*` wildcard position. The wildcard branch
// honours the admission-set consistency between the matcher and
// the chain-authorisation gate (the wildcard admits every subject
// in both surfaces uniformly).
//
// Only the `*` wildcard authorises at the Single-position level; a
// `_` placeholder Single does not, mirroring its label-aware
// exclusion in the matcher. The `_` vs `*` asymmetry is the
// documented invariant — see sumListsSubject.
func conditionAuthorisesSubject(anno *dsl.ReturnAnnotation, subjectName string) bool {
	if anno == nil {
		return false
	}
	if anno.TupleSum != nil && sumListsSubject(anno.TupleSum, subjectName) {
		return true
	}
	for _, pos := range anno.Positions {
		if pos.Single != nil && pos.Single.Wildcard {
			return true
		}
		if pos.Sum == nil {
			continue
		}
		if sumListsSubject(pos.Sum, subjectName) {
			return true
		}
	}
	return false
}

// sumListsSubject reports whether the named subject is authorised
// by any member (or tuple element) of the sum. A textual Term
// whose Type equals subjectName authorises that specific subject;
// a `*` complete wildcard authorises every subject because the
// surface form's design intent is "match every value, including
// labelled subjects" — propagation follows the same admission set
// as sum-membership matching.
//
// The `_` placeholder is deliberately NOT consulted here: it
// authorises no subject. This preserves the `_` vs `*` invariant
// across both surfaces — `*` admits and authorises every subject,
// while `_` carves out the labelled subset, so its label-aware
// exclusion in validateSumPosition's matcher is mirrored by its
// absence from this chain-authorisation gate. A labelled subject
// reached only through a `_` member therefore leaks, matching the
// fixtures in sentinels_placeholder.go.
func sumListsSubject(sum *dsl.DirectSum, subjectName string) bool {
	for _, m := range sum.Members {
		switch {
		case m.Term != nil:
			if m.Term.Wildcard {
				return true
			}
			if m.Term.Type == subjectName {
				return true
			}
		case m.Tuple != nil:
			for _, e := range m.Tuple.Elements {
				if e.Term == nil {
					continue
				}
				if e.Term.Wildcard {
					return true
				}
				if e.Term.Type == subjectName {
					return true
				}
			}
		}
	}
	return false
}

// docCarriesMarker reports whether doc contains marker on any line.
// The line-anchored check matches the convention used by every other
// `vow:` marker the analyzer recognises.
func docCarriesMarker(doc *ast.CommentGroup, marker string) bool {
	if doc == nil {
		return false
	}
	return slices.ContainsFunc(strings.Split(doc.Text(), "\n"), func(line string) bool {
		line = strings.TrimSpace(line)
		return line == marker || strings.HasPrefix(line, marker+" ")
	})
}

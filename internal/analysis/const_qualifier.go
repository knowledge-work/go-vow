package analysis

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// reportConstWithQualifier diagnoses Terms in anno that name a
// package-level `const` and additionally carry a value-level
// constraint (the `nonzero ErrFoo` predicate form, or the auto-
// converted suffix spelling `ErrFoo!`).
//
// A const's value is fixed at compile time, so layering a "non-nil/
// non-zero" predicate on top is semantically empty — whatever the
// contract says, the result is determined by the const's declared
// value. Per-member constraints are relaxed at the parser layer (so
// rule templates like `{ nonzero L | nonzero R }` can expand cleanly);
// the analyzer narrows that acceptance: types and template
// parameters keep their constraints, but expansions or hand-written
// annotations that land on a const must drop them.
//
// The check runs after `@rule` expansion, so it catches both
// hand-written `{ nonzero ZeroCode | nonzero OneCode }` and the
// post-expansion shape of `@Either[ZeroCode, OneCode]`.
//
// Template parameters (`T`, `E` inside an unexpanded body) are not
// reached by this check — the analyzer only ever sees the expanded
// payload.
func reportConstWithQualifier(pass *analysis.Pass, anno *dsl.ReturnAnnotation, reportPos analysisReportPos) {
	if pass.Pkg == nil {
		return
	}
	scope := pass.Pkg.Scope()
	walkAnnotationTerms(anno, func(term *dsl.Term) {
		if term.Placeholder || term.Type == "" {
			return
		}
		predText, hasPred := termPredicateText(term)
		if !hasPred {
			return
		}
		obj := scope.Lookup(term.Type)
		if obj == nil {
			return
		}
		if _, isConst := obj.(*types.Const); !isConst {
			return
		}
		vowReportf(
			pass,
			reportPos(),
			"vow[sentinel-error]: const %s cannot carry constraint %q; const values are fixed",
			term.Type, predText,
		)
	})
}

// termPredicateText returns the surface form of the term's value-
// level predicate together with a presence flag. The check is a
// single nil test on the Pred field, which the prefix predicates
// (`nonzero T`, `nil`, `nonnil`) populate; a bare type carries no
// predicate.
func termPredicateText(t *dsl.Term) (string, bool) {
	if t.Pred == nil {
		return "", false
	}
	return t.Pred.String(), true
}

// analysisReportPos defers the source position used for the diagnostic
// to the caller so each call site can attribute the report to the
// most specific node available (typically the function declaration's
// Pos, since the rejection is about the whole annotation rather than
// a single expression).
type analysisReportPos func() token.Pos

// walkAnnotationTerms visits every Term reachable from anno —
// position-wise singles, direct-sum members, and tuple elements —
// invoking fn on each.
func walkAnnotationTerms(anno *dsl.ReturnAnnotation, fn func(*dsl.Term)) {
	if anno.TupleSum != nil {
		walkSumTerms(anno.TupleSum, fn)
	}
	for _, pos := range anno.Positions {
		if pos.Single != nil {
			fn(pos.Single)
		}
		if pos.Sum != nil {
			walkSumTerms(pos.Sum, fn)
		}
	}
}

func walkSumTerms(sum *dsl.DirectSum, fn func(*dsl.Term)) {
	for _, m := range sum.Members {
		switch {
		case m.Term != nil:
			fn(m.Term)
		case m.Tuple != nil:
			for _, e := range m.Tuple.Elements {
				if e.Term != nil {
					fn(e.Term)
				}
			}
		}
	}
}

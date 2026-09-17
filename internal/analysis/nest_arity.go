package analysis

import (
	"go/ast"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// checkNestArity reports when a `vow:nil` nest body names a wrong
// number of inner decls for its container shape: a slice that
// carries two inner decls, a map missing its key decl, or a generic
// instantiation whose inner decls do not match the number of type
// arguments. Positions whose Go type the resolver cannot classify
// stay silent so unrelated types are not flagged.
//
// The walker pairs each signature position with the matching
// PositionDecl in argument-index order. Grouped declarations like
// `(a, b *Foo[T])` expand into one position per name; every entry
// shares the same Go type expression so a `[T]` annotation on the
// group applies to every name.
func checkNestArity(pass *analysis.Pass, fn *ast.FuncDecl, sig *dsl.NilSignature) {
	if sig == nil || fn == nil {
		return
	}
	if recvExpr := receiverTypeExpr(fn); recvExpr != nil {
		reportNestArity(pass, fn, "recv", 0, recvExpr, sig.Recv)
	}
	walkFieldListPositions(fn.Type.Params, sig.Params, func(idx int, expr ast.Expr, decl *dsl.PositionDecl) {
		reportNestArity(pass, fn, "param", idx, expr, decl)
	})
	walkFieldListPositions(fn.Type.Results, sig.Returns, func(idx int, expr ast.Expr, decl *dsl.PositionDecl) {
		reportNestArity(pass, fn, "return", idx, expr, decl)
	})
}

// reportNestArity reads the resolved container shape at one
// signature position and emits the arity diagnostic when the inner
// decls list does not match. The position is anchored at fn.Pos()
// so the diagnostic surfaces alongside the function declaration the
// `vow:nil` marker sits above.
func reportNestArity(pass *analysis.Pass, fn *ast.FuncDecl, layer string, idx int, expr ast.Expr, decl *dsl.PositionDecl) {
	if decl == nil || decl.Nest == nil || expr == nil {
		return
	}
	t := pass.TypesInfo.TypeOf(expr)
	kind := resolveNestKind(t)
	if kind == nestKindUnknown {
		return
	}
	low, high := nestArity(t, kind)
	got := len(decl.Nest.InnerDecls)
	if got >= low && got <= high {
		return
	}
	vowReportNilSafetyf(
		pass,
		fn.Pos(),
		"vow[nil-safety]: %s %s position %d %s nest carries %s but the container expects %s",
		nilDeclMarker,
		layer,
		idx,
		nestKindLabel(kind),
		pluralizeCount(got, "inner decl", "inner decls"),
		arityRangeText(kind, low, high),
	)
}

// walkFieldListPositions invokes fn once per declared position in
// fields, paired with the matching PositionDecl from decls. Grouped
// declarations expand into one call per name so the callback's idx
// matches argument-index order, and every name in a group sees the
// same Go type expression.
func walkFieldListPositions(fields *ast.FieldList, decls []*dsl.PositionDecl, fn func(idx int, expr ast.Expr, decl *dsl.PositionDecl)) {
	if fields == nil {
		return
	}
	idx := 0
	for _, field := range fields.List {
		entries := 1
		if len(field.Names) > 0 {
			entries = len(field.Names)
		}
		for i := 0; i < entries; i++ {
			var decl *dsl.PositionDecl
			if idx < len(decls) {
				decl = decls[idx]
			}
			fn(idx, field.Type, decl)
			idx++
		}
	}
}

// receiverTypeExpr returns the type expression of fn's receiver, or
// nil when fn carries no receiver. Receivers always own a single
// position so the helper does not iterate.
func receiverTypeExpr(fn *ast.FuncDecl) ast.Expr {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List) == 0 {
		return nil
	}
	return fn.Recv.List[0].Type
}

// nestKindLabel renders a nestKind for use inside diagnostic
// messages. The labels mirror the surface vocabulary (slice / map /
// generic) authors read in the grammar reference.
func nestKindLabel(kind nestKind) string {
	switch kind {
	case nestKindSlice:
		return "slice"
	case nestKindMap:
		return "map"
	case nestKindGeneric:
		return "generic"
	}
	return ""
}

// arityRangeText renders the expected inner-decls range for a
// container shape. nestArity returns either a fixed count
// (map / generic) or the 0..1 slice range; the helper renders each
// shape in the most natural phrasing.
func arityRangeText(kind nestKind, low, high int) string {
	if low == 0 && high == 1 {
		return "zero or one entry"
	}
	return pluralizeCount(low, "entry", "entries")
}

// pluralizeCount renders a count with an English singular/plural
// noun so the diagnostic reads "1 entry" / "2 entries" without a
// trailing "(s)".
func pluralizeCount(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + plural
}

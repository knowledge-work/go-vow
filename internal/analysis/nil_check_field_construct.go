package analysis

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/seq"
)

// validateFieldNilConstruct reports a package-level var initialised with
// a struct literal that leaves a `!`-declared field at its zero value.
//
// Only package-level declarations are read here. Inside a function body
// the same judgement is made on SSA by validateZeroStructEscape, which
// does not depend on the syntactic shape of the escape; a constant
// package-level initialiser is compiled statically and produces no SSA
// instruction at all, so it is the one position SSA cannot see.
func validateFieldNilConstruct(pass *analysis.Pass, state *passState) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.ValueSpec:
				reportEscapingLiterals(pass, state, escapingValueSpecValues(pass, node))
			}
			return true
		})
	}
}

// escapingValueSpecValues returns the initialisers of spec whose
// declared variable lives at package scope. A package-level var is
// reachable by every reader in the package and by importers of an
// exported one, and no later field write can be paired with the
// declaration, so its initialiser escapes. A function-local `var b =
// Box{}` is the same build-then-fill shape as `b := Box{}` and
// contributes nothing here.
func escapingValueSpecValues(pass *analysis.Pass, spec *ast.ValueSpec) []ast.Expr {
	var out []ast.Expr
	for i, name := range spec.Names {
		if i >= len(spec.Values) {
			continue
		}
		if !identTargetEscapes(pass, name, pass.TypesInfo.Defs[name]) {
			continue
		}
		out = append(out, spec.Values[i])
	}
	return out
}

// identTargetEscapes decides, for both the assignment form and the
// declaration form, whether storing into ident puts the value beyond
// the reach of a later field write. Keeping one predicate for the two
// forms is what stops them disagreeing about the same destination.
//
// Two shapes hold the value back. The blank identifier discards it, so
// no reader observes the omitted field — this is judged on the name
// rather than on the object, because a blank var is not entered into
// any scope and its object carries no parent to compare. A variable
// declared inside a function body stays addressable by the statements
// that follow, which is the build-then-fill shape. Everything else —
// package scope included — escapes.
func identTargetEscapes(pass *analysis.Pass, ident *ast.Ident, obj types.Object) bool {
	if ident.Name == "_" {
		return false
	}
	if obj == nil {
		return false
	}
	return obj.Parent() == nil || obj.Parent() == pass.Pkg.Scope()
}

// reportEscapingLiterals emits a diagnostic for every `!`-declared
// field that a struct literal inside exprs leaves at its zero value.
// The walk steps through parens and address-of operators so `&Box{}`
// is judged as the Box literal it builds, and recurses into each
// element's value so a literal nested inside an escaping literal is
// judged under the same rule as its parent.
func reportEscapingLiterals(pass *analysis.Pass, state *passState, exprs []ast.Expr) {
	for _, expr := range exprs {
		lit, ok := escapingCompositeLit(expr)
		if !ok {
			continue
		}
		reportOmittedNonNilFields(pass, state, lit)
		reportEscapingLiterals(pass, state, compositeLitValues(lit))
	}
}

// escapingCompositeLit unwraps expr down to the composite literal it
// builds, stepping through parenthesises and a leading address-of.
// Shapes that are not a literal at their core report ok=false.
func escapingCompositeLit(expr ast.Expr) (*ast.CompositeLit, bool) {
	switch e := ast.Unparen(expr).(type) {
	case *ast.CompositeLit:
		return e, true
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return escapingCompositeLit(e.X)
		}
	}
	return nil, false
}

// compositeLitValues returns the value expression of every element of
// lit, dropping the keys so a keyed and a positional literal expose
// the same shape to the recursive walk.
func compositeLitValues(lit *ast.CompositeLit) []ast.Expr {
	return seq.ChainOf(lit.Elts...).
		Map(func(elt ast.Expr) ast.Expr {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				return kv.Value
			}
			return elt
		}).
		ToSlice()
}

// reportOmittedNonNilFields emits one diagnostic per `!`-declared
// field of lit's struct type that lit supplies no member for. The
// diagnostic anchors at the literal because the literal is what the
// author edits to satisfy the obligation.
func reportOmittedNonNilFields(pass *analysis.Pass, state *passState, lit *ast.CompositeLit) {
	st := structTypeOfCompositeLit(pass, lit)
	if st == nil {
		return
	}
	for _, field := range omittedNonNilFields(pass, state, lit, st) {
		vowReportNilSafetyf(
			pass,
			lit.Pos(),
			"vow[nil-safety]: field %s left nil by this literal; %s declared ! on this field",
			field,
			nilDeclMarker,
		)
	}
}

// omittedNonNilFields returns the names of st's `!`-declared fields
// that lit supplies no member for, in declaration order so the
// diagnostics come out deterministically.
//
// A positional literal supplies every field by Go's own rule, so it
// omits nothing and returns empty; the nil-write check judges a nil
// that a positional element carries. A keyed literal omits every
// field its keys do not name, and an empty literal omits all of them.
func omittedNonNilFields(pass *analysis.Pass, state *passState, lit *ast.CompositeLit, st *types.Struct) []string {
	if positionalCompositeLit(lit) {
		return nil
	}
	supplied := suppliedFieldNames(lit)
	var out []string
	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		if _, ok := supplied[field.Name()]; ok {
			continue
		}
		name, ok := nonNilFieldName(pass, state, field)
		if !ok {
			continue
		}
		out = append(out, name)
	}
	return out
}

// positionalCompositeLit reports whether lit supplies its members
// positionally. Go rejects a literal that mixes the two forms, so the
// first element decides for the whole literal; an empty literal is
// keyed-shaped because it names nothing.
func positionalCompositeLit(lit *ast.CompositeLit) bool {
	if len(lit.Elts) == 0 {
		return false
	}
	_, keyed := lit.Elts[0].(*ast.KeyValueExpr)
	return !keyed
}

// suppliedFieldNames returns the set of field names lit names through
// keyed elements. Elements whose key is not an identifier contribute
// nothing — they cannot name a struct field.
func suppliedFieldNames(lit *ast.CompositeLit) map[string]struct{} {
	out := map[string]struct{}{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok {
			out[key.Name] = struct{}{}
		}
	}
	return out
}

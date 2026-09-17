package analysis

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateNilDeclReceiverGuard walks every method whose signature
// decl declares the receiver as nillable (`?`) and reports when
// the body dereferences the receiver without a leading nil guard.
// The decl explicitly admits a nil receiver, so the author is
// expected to guard the deref before the first use; an unguarded
// deref is the common nil-receiver panic source the vow:nil decl
// is meant to surface at lint time.
//
// The pass is intentionally AST-only and conservative. The leading
// guard recogniser accepts two shapes: a body-leading
// `if recv == nil { return }` short-circuit, and a body-leading
// `if recv != nil { ... }` positive guard. Either shape silences
// the diagnostic regardless of where later derefs sit; bodies
// without such a leading guard surface one diagnostic at the
// first receiver deref. Richer SSA-based guard detection (a guard
// buried after a non-guard prelude, a deref dominated by a
// non-leading branch, indexed-receiver and type-assertion deref
// shapes) is not covered.
func validateNilDeclReceiverGuard(pass *analysis.Pass, state *passState) {
	decls := state.nilDeclSet(pass)
	if len(decls) == 0 {
		return
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv == nil {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			if obj == nil {
				continue
			}
			sig, ok := decls[obj]
			if !ok || sig == nil || sig.Recv == nil {
				continue
			}
			if sig.Recv.Nullness != dsl.NullnessNillable {
				continue
			}
			reportNilDeclUnguardedReceiver(pass, fn)
		}
	}
}

// reportNilDeclUnguardedReceiver checks fn for a leading nil guard
// on the receiver and, when none is present, reports the first
// receiver deref site inside the body. The recogniser binds the
// receiver to its types.Object so that closures or nested blocks
// that shadow the receiver name do not borrow the guard or
// contribute spurious deref sites.
func reportNilDeclUnguardedReceiver(pass *analysis.Pass, fn *ast.FuncDecl) {
	recvObj := receiverObject(pass, fn)
	if recvObj == nil {
		return
	}
	if hasLeadingReceiverNilGuard(pass, fn.Body, recvObj) {
		return
	}
	if site := firstReceiverDerefSite(pass, fn.Body, recvObj); site != nil {
		vowReportNilSafetyf(
			pass,
			site.Pos(),
			"vow[nil-safety]: receiver %s is dereferenced without a nil guard; %s declared %s ? at this position",
			recvObj.Name(),
			nilDeclMarker,
			recvObj.Name(),
		)
	}
}

// receiverObject returns the types.Object that the named receiver
// of fn resolves to. Anonymous receivers (`func (T) M()`) carry no
// name a body expression can reference, so the helper returns nil
// and the caller skips the body; the body cannot mention the
// receiver, so no deref site exists.
func receiverObject(pass *analysis.Pass, fn *ast.FuncDecl) types.Object {
	if fn.Recv == nil {
		return nil
	}
	for _, field := range fn.Recv.List {
		for _, name := range field.Names {
			if name.Name == "" {
				continue
			}
			if obj := pass.TypesInfo.Defs[name]; obj != nil {
				return obj
			}
		}
	}
	return nil
}

// hasLeadingReceiverNilGuard reports whether body opens with a
// recognised nil guard on the receiver. Two shapes qualify:
//
//   - `if recv == nil { return ... }` — the short-circuit form
//     shared with the dead-guard recogniser. Control leaves the
//     function on the nil path, so every later statement runs
//     with the receiver proven non-nil.
//   - `if recv != nil { ... }` — the positive form. The Go idiom
//     pins the deref work inside the then-branch, so the body
//     advertises that it has considered nilness before any deref.
//     The recogniser admits the shape regardless of whether the
//     then-branch short-circuits, because surfacing the deref
//     inside the then-branch would contradict the author's
//     declared intent and produce a false positive on the most
//     common Go nil-handling pattern.
//
// Both shapes also count when the receiver check leads a logical
// expression (`recv == nil || …`, `recv != nil && …`); see
// receiverNilCheckOp for why only the leftmost position qualifies.
func hasLeadingReceiverNilGuard(pass *analysis.Pass, body *ast.BlockStmt, recvObj types.Object) bool {
	for _, stmt := range leadingIfStatements(body) {
		op, ok := receiverNilCheckOp(pass, stmt.Cond, recvObj)
		if !ok {
			continue
		}
		if op == "==" && branchShortCircuits(stmt.Body) {
			return true
		}
		if op == "!=" {
			return true
		}
	}
	return false
}

// receiverNilCheckOp reports the comparison operator (`==` or
// `!=`) when cond compares the receiver against the bare `nil`
// identifier. The operand-binding goes through types.Info so the
// comparison rejects unrelated identifiers that happen to share
// the receiver's spelling (a shadow inside a nested scope, an
// imported name).
//
// A guard is often written together with the checks it protects —
// `if recv == nil || recv.dep == nil` and
// `if recv != nil && recv.dep != nil` are both idiomatic. Go
// evaluates the right operand of `||` only when the left one is
// false (and of `&&` only when the left one is true), so the
// leftmost comparison still guards every operand to its right. The
// recogniser therefore descends into the left operand of a logical
// expression and ignores the rest: only a guard in that position
// protects the operands that follow, so accepting anything else
// would admit a deref the receiver check never covered.
func receiverNilCheckOp(pass *analysis.Pass, cond ast.Expr, recvObj types.Object) (string, bool) {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return "", false
	}
	switch bin.Op {
	case token.LOR:
		if op, ok := receiverNilCheckOp(pass, bin.X, recvObj); ok && op == "==" {
			return op, true
		}
		return "", false
	case token.LAND:
		if op, ok := receiverNilCheckOp(pass, bin.X, recvObj); ok && op == "!=" {
			return op, true
		}
		return "", false
	}
	op := bin.Op.String()
	if op != "==" && op != "!=" {
		return "", false
	}
	if isReceiverIdent(pass, bin.X, recvObj) && isBareNilIdent(bin.Y) {
		return op, true
	}
	if isReceiverIdent(pass, bin.Y, recvObj) && isBareNilIdent(bin.X) {
		return op, true
	}
	return "", false
}

// firstReceiverDerefSite returns the first node inside body that
// dereferences the receiver. The recogniser fires on a selector
// expression whose X resolves to the receiver (the `recv.X` shape
// that triggers a panic when recv is nil) and on a star-expression
// pointer load. The match runs through types.Info so a closure
// that shadows the receiver name does not contribute spurious
// sites. Other shapes that risk a panic (an indexed receiver, a
// type assertion) fall through unreported and land in a
// here.
func firstReceiverDerefSite(pass *analysis.Pass, body *ast.BlockStmt, recvObj types.Object) ast.Node {
	var site ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if site != nil {
			return false
		}
		switch node := n.(type) {
		case *ast.SelectorExpr:
			if isReceiverIdent(pass, node.X, recvObj) {
				site = node
				return false
			}
		case *ast.StarExpr:
			if isReceiverIdent(pass, node.X, recvObj) {
				site = node
				return false
			}
		}
		return true
	})
	return site
}

// isReceiverIdent reports whether expr is a bare identifier that
// types.Info resolves to recvObj. The check is the single anchor
// that ties every recogniser in this pass to the same notion of
// "the receiver"; identifier-name comparison alone would let a
// closure parameter or a package-level variable named the same as
// the receiver borrow into the receiver's role.
func isReceiverIdent(pass *analysis.Pass, expr ast.Expr, recvObj types.Object) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	return pass.TypesInfo.ObjectOf(ident) == recvObj
}

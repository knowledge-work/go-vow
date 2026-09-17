package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"iter"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"

	"github.com/knowledge-work/go-vow/internal/seq"
)

// readonlyMarker declares that a struct type's fields are only assigned
// while the instance is still under construction — from the point it is
// created until it first escapes the function that created it.
//
// The guarantee is the one Java's `final` gives a field, not immutability:
// it stops the reference from being replaced, and says nothing about what
// the reference points at. `c.Rules = other` is a violation, `c.Rules[0] =
// r` is not, and the length of `c.Rules` is not fixed by the marker.
const readonlyMarker = "vow:readonly"

// validateReadonlyWrites reports a field assignment on a readonly type
// that runs after the instance has escaped.
//
// Escape follows the set the nullness passes already use: a return result,
// a store to anything but a local slot, a channel send, a map value, and
// any call operand. A value the creating function never lets out is never
// closed, so a constructor that fills its fields and returns is silent.
func validateReadonlyWrites(pass *analysis.Pass) {
	marked := readonlyTypes(pass)
	if len(marked) == 0 {
		return
	}
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok || result == nil {
		return
	}
	for _, fn := range result.SrcFuncs {
		for instr := range instructionsOf(fn) {
			store, ok := instr.(*ssa.Store)
			if !ok {
				continue
			}
			field, ok := store.Addr.(*ssa.FieldAddr)
			if !ok {
				continue
			}
			named, ok := readonlyNamedOf(field.X.Type(), marked)
			if !ok {
				continue
			}
			reportWriteAfterEscape(pass, fn, store, field, named)
		}
	}
}

// instructionsOf walks fn's instructions in block order. The blocks
// themselves carry nothing this check reads, so flattening them away keeps
// the caller's nesting at the two levels it needs: one per function, one
// per instruction.
func instructionsOf(fn *ssa.Function) seq.Chain[ssa.Instruction] {
	return seq.ChainOf(fn.Blocks...).FlatMap(func(block *ssa.BasicBlock) iter.Seq[ssa.Instruction] {
		return slices.Values(block.Instrs)
	})
}

// reportWriteAfterEscape reports store when an escape of the instance it
// writes to reaches it on every route. Asking the question the other way —
// whether the store reaches the escape — would report every store inside a
// branch, because a branch does not dominate the join the escape sits in.
func reportWriteAfterEscape(pass *analysis.Pass, fn *ssa.Function, store *ssa.Store, field *ssa.FieldAddr, named string) {
	alloc, ok := rootAllocOf(field.X)
	if !ok || isParameterSlot(fn, alloc) {
		return
	}
	escape, escapes := firstReadonlyEscape(alloc)
	if !escapes || !readonlyPrecedes(escape, store) {
		return
	}
	vowReportf(pass, store.Pos(),
		"vow[readonly]: %s reassigns a field after the value escaped; a readonly type takes field assignments only while it is under construction",
		named)
}

// readonlyTypes collects the named types whose declaration carries the
// marker.
func readonlyTypes(pass *analysis.Pass) map[*types.Named]bool {
	return declarationsOf(pass.Files).
		FilterMap(asTypeDecl).
		FlatMap(markedSpecsOf).
		FilterMap(func(spec *ast.TypeSpec) (*types.Named, bool) {
			obj, isTypeName := pass.TypesInfo.Defs[spec.Name].(*types.TypeName)
			if !isTypeName {
				return nil, false
			}
			named, isNamed := obj.Type().(*types.Named)
			return named, isNamed
		}).
		ToMap(func(named *types.Named) (*types.Named, bool) {
			return named, true
		})
}

// asTypeDecl narrows a declaration to a type declaration block.
func asTypeDecl(decl ast.Decl) (*ast.GenDecl, bool) {
	gen, ok := decl.(*ast.GenDecl)
	return gen, ok && gen.Tok == token.TYPE
}

// markedSpecsOf yields the specs of gen that carry the marker. The marker
// test runs here, while gen is still in hand, because it falls back to the
// block's own doc comment, which a spec on its own no longer reaches.
func markedSpecsOf(gen *ast.GenDecl) iter.Seq[*ast.TypeSpec] {
	return seq.ChainOf(gen.Specs...).
		FilterMap(func(spec ast.Spec) (*ast.TypeSpec, bool) {
			typeSpec, isTypeSpec := spec.(*ast.TypeSpec)
			return typeSpec, isTypeSpec && carriesReadonlyMarker(gen, typeSpec)
		}).
		ToSeq()
}

// declarationsOf walks the declarations of every file. Which file a
// declaration came from does not reach the marker scan, so the files
// flatten away.
func declarationsOf(files []*ast.File) seq.Chain[ast.Decl] {
	return seq.ChainOf(files...).FlatMap(func(file *ast.File) iter.Seq[ast.Decl] {
		return slices.Values(file.Decls)
	})
}

// carriesReadonlyMarker looks at the spec's own doc first and falls back to
// the block's, so a marker on a `type ( ... )` group reaches every spec in
// it while a marker on one spec stays with that spec.
func carriesReadonlyMarker(gen *ast.GenDecl, spec *ast.TypeSpec) bool {
	if hasReadonlyLine(spec.Doc) {
		return true
	}
	return hasReadonlyLine(gen.Doc)
}

func hasReadonlyLine(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	return slices.ContainsFunc(doc.List, func(comment *ast.Comment) bool {
		return strings.TrimSpace(trimCommentMarker(comment.Text)) == readonlyMarker
	})
}

// readonlyNamedOf reports the marked type the address belongs to, peeling
// the pointer a field address carries.
func readonlyNamedOf(typ types.Type, marked map[*types.Named]bool) (string, bool) {
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	named, ok := typ.(*types.Named)
	if !ok || !marked[named] {
		return "", false
	}
	return named.Obj().Name(), true
}

// rootAllocOf walks an address back to the allocation it belongs to.
func rootAllocOf(v ssa.Value) (*ssa.Alloc, bool) {
	for range 32 {
		switch node := v.(type) {
		case *ssa.Alloc:
			return node, true
		case *ssa.FieldAddr:
			v = node.X
		case *ssa.IndexAddr:
			v = node.X
		case *ssa.UnOp:
			if node.Op != token.MUL {
				return nil, false
			}
			v = node.X
		default:
			return nil, false
		}
	}
	return nil, false
}

// isParameterSlot reports whether alloc is the slot a by-value parameter
// arrives in. Writing a field of that copy cannot reach the caller, so the
// copy opens a construction window of its own at function entry.
func isParameterSlot(fn *ssa.Function, alloc *ssa.Alloc) bool {
	return slices.ContainsFunc(fn.Params, func(param *ssa.Parameter) bool {
		return slices.ContainsFunc(referrersOf(param), func(referrer ssa.Instruction) bool {
			store, ok := referrer.(*ssa.Store)
			return ok && store.Val == param && store.Addr == ssa.Value(alloc)
		})
	})
}

func referrersOf(v ssa.Value) []ssa.Instruction {
	if v.Referrers() == nil {
		return nil
	}
	return *v.Referrers()
}

// firstReadonlyEscape returns the earliest instruction that puts the
// allocation out of the creating function's reach.
func firstReadonlyEscape(alloc *ssa.Alloc) (ssa.Instruction, bool) {
	var earliest ssa.Instruction
	seen := map[ssa.Value]bool{}
	var walk func(v ssa.Value, depth int)
	walk = func(v ssa.Value, depth int) {
		if depth > 8 || seen[v] {
			return
		}
		seen[v] = true
		for _, referrer := range referrersOf(v) {
			candidate := readonlyEscapeThrough(v, referrer, walk, depth)
			if candidate == nil {
				continue
			}
			if earliest == nil || readonlyPrecedes(candidate, earliest) {
				earliest = candidate
			}
		}
	}
	walk(alloc, 0)
	return earliest, earliest != nil
}

// readonlyEscapeThrough answers whether referrer lets value out. A call in
// receiver position is not counted, which is a choice rather than a
// finding: the callee is not read, so a method that stores its receiver
// passes through here unseen. Counting it instead would close the window
// on the first `v.validate()`, and a check that reports every constructor
// is worth less than one that misses the method which stashes.
func readonlyEscapeThrough(value ssa.Value, referrer ssa.Instruction, walk func(ssa.Value, int), depth int) ssa.Instruction {
	switch node := referrer.(type) {
	case *ssa.FieldAddr, *ssa.IndexAddr:
		return nil
	case *ssa.UnOp:
		if node.Op == token.MUL {
			walk(node, depth+1)
		}
		return nil
	case *ssa.MakeInterface:
		walk(node, depth+1)
		return nil
	case *ssa.Store:
		if _, isLocal := node.Addr.(*ssa.Alloc); isLocal || node.Val != value {
			return nil
		}
		return node
	case *ssa.Return, *ssa.Send, *ssa.MakeClosure, *ssa.MapUpdate:
		return referrer
	}
	call, ok := referrer.(ssa.CallInstruction)
	if !ok {
		return nil
	}
	common := call.Common()
	if common == nil {
		return nil
	}
	args := common.Args
	if !common.IsInvoke() && calleeTakesAReceiverSlot(common) {
		args = args[1:]
	}
	if slices.Contains(args, value) {
		return referrer
	}
	return nil
}

// calleeTakesAReceiverSlot reports whether the callee's first argument is
// the receiver. A method expression compiles to a thunk whose own signature
// has no receiver, so the method the thunk stands for is what answers.
//
// That method answers the same for a bound method value, whose receiver is
// a free variable rather than an argument. Such a callee cannot reach the
// assertion below: a function holding free variables is called through the
// closure that supplies them, never by name.
func calleeTakesAReceiverSlot(common *ssa.CallCommon) bool {
	if common.Signature().Recv() != nil {
		return true
	}
	callee, ok := common.Value.(*ssa.Function)
	if !ok || callee.Object() == nil {
		return false
	}
	signature, ok := callee.Object().Type().(*types.Signature)
	return ok && signature.Recv() != nil
}

// readonlyPrecedes reports whether a reaches b on every route: a's block
// dominates b's, or the two share a block and a comes first.
func readonlyPrecedes(a, b ssa.Instruction) bool {
	ablock, bblock := a.Block(), b.Block()
	if ablock == nil || bblock == nil {
		return false
	}
	if ablock == bblock {
		first, found := seq.ChainOf(ablock.Instrs...).Find(func(instr ssa.Instruction) bool {
			return instr == a || instr == b
		})
		return found && first == a
	}
	return ablock.Dominates(bblock)
}

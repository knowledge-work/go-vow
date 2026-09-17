package analysis

import (
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// validateAllocStructEscape reports a struct built through an allocation
// that escapes while a `!`-declared field still has no store reaching it.
// The zero-constant pass covers values the builder folds to a constant;
// this one covers the rest, where a field write exists somewhere and the
// question is whether it runs first.
//
// An allocation handed to a call is left alone: the callee holds the
// pointer and may fill the field, and this pass does not read callee
// bodies. Every other way the allocation leaves — returned, stored,
// sent, loaded and passed on — is an escape, because nothing can fill
// the field after that.
func validateAllocStructEscape(pass *analysis.Pass, state *passState) {
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok || result == nil {
		return
	}
	for _, fn := range result.SrcFuncs {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				alloc, ok := instr.(*ssa.Alloc)
				if !ok {
					continue
				}
				reportUnsetAllocFields(pass, state, alloc)
			}
		}
	}
}

// reportUnsetAllocFields emits one diagnostic per `!`-declared field of
// alloc's struct that no store reaches before the allocation escapes.
func reportUnsetAllocFields(pass *analysis.Pass, state *passState, alloc *ssa.Alloc) {
	if !allocEscapeIsJudged(alloc) {
		return
	}
	escape, ok := firstAllocEscape(alloc)
	if !ok {
		// Judged through the storage this one was copied into, which
		// carries what was left unfilled and reports in its place.
		return
	}
	st, _ := allocatedStruct(alloc)
	members, _ := allocMembers(alloc)
	for _, field := range nonNilFields(pass, state, st, nil, map[*types.Struct]bool{}) {
		if members.allFilled(field.path, escape) {
			continue
		}
		vowReportNilSafetyf(
			pass,
			escapePosition(escape),
			"vow[nil-safety]: field %s left nil where this value escapes; %s declared ! on this field",
			field.name,
			nilDeclMarker,
		)
	}
}

// allocEscapeIsJudged reports whether this pass carries the allocation as
// far as a judgement. Four things stop it: storage that is not a struct
// or an array of them, an allocation a callee or a closure may still fill,
// elements it cannot attribute to a member, and a value it cannot see
// leave — unless that value is copied whole into other storage, which
// answers for it in its place. reportUnsetAllocFields asks this first so
// that the zero pass, which leaves a write into judged storage for this
// one to answer, asks the same question rather than a resemblance of it.
func allocEscapeIsJudged(alloc *ssa.Alloc) bool {
	return allocJudged(alloc, map[*ssa.Alloc]bool{})
}

// allocJudged carries the seen set that bounds the walk through storage
// copied into storage.
func allocJudged(alloc *ssa.Alloc, seen map[*ssa.Alloc]bool) bool {
	if seen[alloc] {
		return false
	}
	seen[alloc] = true
	if _, ok := allocatedStruct(alloc); !ok {
		return false
	}
	if allocHandedToCall(alloc) || allocCapturedForWriting(alloc) {
		return false
	}
	if _, ok := allocMembers(alloc); !ok {
		return false
	}
	if _, ok := firstAllocEscape(alloc); ok {
		return true
	}
	return copiedIntoJudgedStorage(alloc, seen)
}

// copiedIntoJudgedStorage reports whether the allocation's whole value is
// stored into other storage that is judged in its place. The temporary a
// composite literal is built in takes this route: it never escapes on its
// own, and the local it is copied into carries what it left unfilled.
func copiedIntoJudgedStorage(alloc *ssa.Alloc, seen map[*ssa.Alloc]bool) bool {
	if alloc.Referrers() == nil {
		return false
	}
	for _, referrer := range *alloc.Referrers() {
		load, isLoad := referrer.(*ssa.UnOp)
		if !isLoad || load.Op != token.MUL || load.X != ssa.Value(alloc) || load.Referrers() == nil {
			continue
		}
		for _, use := range *load.Referrers() {
			store, isStore := use.(*ssa.Store)
			if !isStore || store.Val != ssa.Value(load) {
				continue
			}
			target, ok := rootAllocation(store.Addr)
			if ok && allocJudged(target, seen) {
				return true
			}
		}
	}
	return false
}

// rootAllocation walks an address back to the allocation it reaches,
// through field addresses and constant-index element addresses. An
// element reached any other way sits inside an array that the
// fields-by-value walk stops at, so nothing would ask about it.
func rootAllocation(addr ssa.Value) (*ssa.Alloc, bool) {
	for {
		switch node := addr.(type) {
		case *ssa.Alloc:
			return node, true
		case *ssa.FieldAddr:
			addr = node.X
		case *ssa.IndexAddr:
			if !constantIndexInto(node) {
				return nil, false
			}
			addr = node.X
		default:
			return nil, false
		}
	}
}

// allocatedStruct returns the struct alloc reserves storage for, reading
// through an array so a slice literal's element type is reached.
func allocatedStruct(alloc *ssa.Alloc) (*types.Struct, bool) {
	stored, ok := allocatedType(alloc)
	if !ok {
		return nil, false
	}
	if arr, isArray := stored.(*types.Array); isArray {
		stored = arr.Elem().Underlying()
	}
	st, ok := stored.(*types.Struct)
	return st, ok
}

// allocatedType returns the underlying type of the storage alloc reserves.
func allocatedType(alloc *ssa.Alloc) (types.Type, bool) {
	ptr, ok := alloc.Type().Underlying().(*types.Pointer)
	if !ok {
		return nil, false
	}
	return ptr.Elem().Underlying(), true
}

// storageMembers holds the addresses through which each struct the
// allocation stores is written.
type storageMembers struct {
	// bases groups the addresses of one member, since a member can be
	// addressed more than once.
	bases [][]ssa.Value
	// whole reports whether bases covers every member of the storage.
	// An array element the builder never addressed has no entry, and
	// that absence is what leaves its fields unwritten.
	whole bool
}

// allocMembers returns the members of alloc's storage: the allocation
// itself for a struct, one entry per element for an array of structs.
// It declines an element reached through a computed index, which leaves
// a fill through it unattributable to any one member.
func allocMembers(alloc *ssa.Alloc) (storageMembers, bool) {
	stored, ok := allocatedType(alloc)
	if !ok {
		return storageMembers{}, false
	}
	arr, isArray := stored.(*types.Array)
	if !isArray {
		return storageMembers{bases: [][]ssa.Value{{alloc}}, whole: true}, true
	}
	if alloc.Referrers() == nil {
		return storageMembers{whole: arr.Len() == 0}, true
	}
	byIndex := map[int64][]ssa.Value{}
	for _, referrer := range *alloc.Referrers() {
		index, isIndex := referrer.(*ssa.IndexAddr)
		if !isIndex {
			continue
		}
		at, isConst := constantIndex(index)
		if !isConst {
			return storageMembers{}, false
		}
		byIndex[at] = append(byIndex[at], index)
	}
	members := storageMembers{whole: int64(len(byIndex)) == arr.Len()}
	for _, bases := range byIndex {
		members.bases = append(members.bases, bases)
	}
	return members, true
}

// constantIndex returns the element index when it is known at build time.
func constantIndex(index *ssa.IndexAddr) (int64, bool) {
	konst, ok := index.Index.(*ssa.Const)
	if !ok || konst.Value == nil {
		return 0, false
	}
	at, ok := constant.Int64Val(konst.Value)
	return at, ok
}

// allFilled reports whether every member has the field at path written
// before escape.
func (m storageMembers) allFilled(path []int, escape ssa.Instruction) bool {
	if !m.whole {
		return false
	}
	for _, bases := range m.bases {
		if !anyBaseFills(bases, path, escape) {
			return false
		}
	}
	return true
}

// anyBaseFills reports whether the field at path is written through one
// of the addresses of a single member.
func anyBaseFills(bases []ssa.Value, path []int, escape ssa.Instruction) bool {
	return slices.ContainsFunc(bases, func(base ssa.Value) bool {
		return fieldStoreReaches(base, path, escape, map[ssa.Value]bool{})
	})
}

// allocHandedToCall reports whether the allocation itself reaches a call,
// which leaves the callee free to fill the field.
func allocHandedToCall(alloc *ssa.Alloc) bool {
	if alloc.Referrers() == nil {
		return false
	}
	for _, referrer := range *alloc.Referrers() {
		call, ok := referrer.(ssa.CallInstruction)
		if !ok {
			continue
		}
		common := call.Common()
		if common == nil {
			continue
		}
		if common.Value == ssa.Value(alloc) {
			return true
		}
		if slices.Contains(common.Args, ssa.Value(alloc)) {
			return true
		}
	}
	return false
}

// allocCapturedForWriting reports whether a function literal captures the
// allocation through a variable it can write, which leaves the closure
// free to fill the field wherever it runs.
func allocCapturedForWriting(alloc *ssa.Alloc) bool {
	if alloc.Referrers() == nil {
		return false
	}
	for _, referrer := range *alloc.Referrers() {
		closure, ok := referrer.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		if capturedForWriting(closure, alloc) {
			return true
		}
	}
	return false
}

// capturedForWriting reports whether the literal closure binds over
// reaches the allocation through anything but a read of the whole value.
func capturedForWriting(closure *ssa.MakeClosure, alloc *ssa.Alloc) bool {
	fn, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return true
	}
	for i, binding := range closure.Bindings {
		if binding != ssa.Value(alloc) || i >= len(fn.FreeVars) {
			continue
		}
		if !onlyLoaded(fn.FreeVars[i]) {
			return true
		}
	}
	return false
}

// onlyLoaded reports whether every use of value reads the whole value,
// which is the one shape that cannot write through it.
func onlyLoaded(value ssa.Value) bool {
	if value.Referrers() == nil {
		return true
	}
	for _, referrer := range *value.Referrers() {
		load, ok := referrer.(*ssa.UnOp)
		if !ok || load.Op != token.MUL {
			return false
		}
	}
	return true
}

// escapePosition returns the position to report the escape at. A closure
// is a synthesized register carrying no position, so the function literal
// that captures the value stands in for it.
func escapePosition(escape ssa.Instruction) token.Pos {
	closure, ok := escape.(*ssa.MakeClosure)
	if !ok {
		return escape.Pos()
	}
	fn, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return escape.Pos()
	}
	return fn.Pos()
}

// firstAllocEscape returns the earliest instruction that lets the
// allocation out of the function's control, taking the whole-struct load
// as the escape when the value is read out rather than the pointer.
func firstAllocEscape(alloc *ssa.Alloc) (ssa.Instruction, bool) {
	if alloc.Referrers() == nil {
		return nil, false
	}
	var earliest ssa.Instruction
	for _, referrer := range *alloc.Referrers() {
		candidate := allocEscapeThrough(alloc, referrer)
		if candidate == nil {
			continue
		}
		if earliest == nil || instructionPrecedes(candidate, earliest) {
			earliest = candidate
		}
	}
	return earliest, earliest != nil
}

// allocEscapeThrough returns the instruction through which referrer lets
// the allocation escape, or nil when it does not. A load of the whole
// struct escapes at whichever instruction consumes the loaded value.
func allocEscapeThrough(alloc *ssa.Alloc, referrer ssa.Instruction) ssa.Instruction {
	switch node := referrer.(type) {
	case *ssa.FieldAddr, *ssa.IndexAddr:
		return nil
	case *ssa.Slice:
		return firstValueEscape(node)
	case *ssa.UnOp:
		if node.Op != token.MUL || node.X != ssa.Value(alloc) {
			return nil
		}
		return firstValueEscape(node)
	case *ssa.Store:
		if node.Val == ssa.Value(alloc) {
			return node
		}
		return nil
	case *ssa.Return, *ssa.Send, *ssa.MakeClosure:
		return referrer
	}
	return nil
}

// firstValueEscape returns the earliest instruction that lets value out
// of the function's control.
func firstValueEscape(value ssa.Value) ssa.Instruction {
	referrers := value.Referrers()
	if referrers == nil {
		return nil
	}
	var earliest ssa.Instruction
	for _, referrer := range *referrers {
		if len(escapingOperands(referrer)) == 0 {
			continue
		}
		for _, operand := range escapingOperands(referrer) {
			if operand != value {
				continue
			}
			if earliest == nil || instructionPrecedes(referrer, earliest) {
				earliest = referrer
			}
		}
	}
	return earliest
}

// fieldStoreReaches reports whether a store that fills the field at path
// reaches escape on every route, which is the case when the store's block
// dominates the escape's and, within one block, the store comes first.
//
// A write short of the path's end counts only for what it carries: the
// store that writes a whole member is asked whether the value it wrote
// holds the field, the same question a store into the storage itself is
// asked. seen bounds the walk through stores that copy one allocation
// into another.
func fieldStoreReaches(base ssa.Value, path []int, escape ssa.Instruction, seen map[ssa.Value]bool) bool {
	if base.Referrers() == nil || seen[base] {
		return false
	}
	seen[base] = true
	defer delete(seen, base)
	if wholeValueStoreFills(base, path, escape, seen) {
		return true
	}
	for _, referrer := range *base.Referrers() {
		addr, ok := referrer.(*ssa.FieldAddr)
		if !ok || addr.Field != path[0] {
			continue
		}
		if len(path) == 1 {
			if storeReaches(addr, escape) {
				return true
			}
			continue
		}
		if fieldStoreReaches(addr, path[1:], escape, seen) {
			return true
		}
	}
	return false
}

// wholeValueStoreFills reports whether a store of a whole struct into base
// itself fills the field at path before escape.
//
// A store addressed by a field escapes the value it stores, so a nested
// struct written in is judged where it was built. A store addressed by the
// allocation is deliberately not an escape, because a later field write can
// still address that storage, so nothing else judges what was written and
// this asks about the stored value directly instead.
func wholeValueStoreFills(base ssa.Value, path []int, escape ssa.Instruction, seen map[ssa.Value]bool) bool {
	for _, referrer := range *base.Referrers() {
		store, ok := referrer.(*ssa.Store)
		if !ok || store.Addr != base || !instructionPrecedes(store, escape) {
			continue
		}
		if storedValueFills(store.Val, path, seen) {
			return true
		}
	}
	return false
}

// storedValueFills reports whether the struct value carries a member at
// path. Two kinds are answered from what this pass can see: a struct
// constant is the zero value and carries none, and a load of storage
// allocated here is asked the same question that storage answers.
//
// Everything else is taken as filled, which is right for the values that
// arrive already built — a parameter, a receiver, a call result — and is
// the standing gap for the rest. A load of a package-level var is the one
// that matters: reaching it needs stores from other functions, which the
// dominance test this pass runs cannot order.
func storedValueFills(value ssa.Value, path []int, seen map[ssa.Value]bool) bool {
	switch node := value.(type) {
	case *ssa.Const:
		return false
	case *ssa.UnOp:
		if node.Op != token.MUL {
			return true
		}
		alloc, prefix, ok := allocatedAddress(node.X)
		if !ok {
			return true
		}
		return fieldStoreReaches(alloc, append(prefix, path...), node, seen)
	}
	return true
}

// allocatedAddress resolves an address to the allocation it reaches and
// the field indices that lead there, so a member loaded out of a local
// struct is asked about under the path its own storage knows it by.
func allocatedAddress(addr ssa.Value) (*ssa.Alloc, []int, bool) {
	var reversed []int
	for {
		switch node := addr.(type) {
		case *ssa.Alloc:
			prefix := make([]int, 0, len(reversed))
			for i := len(reversed) - 1; i >= 0; i-- {
				prefix = append(prefix, reversed[i])
			}
			return node, prefix, true
		case *ssa.FieldAddr:
			reversed = append(reversed, node.Field)
			addr = node.X
		default:
			return nil, nil, false
		}
	}
}

// storeReaches reports whether a store through addr runs before escape.
func storeReaches(addr *ssa.FieldAddr, escape ssa.Instruction) bool {
	if addr.Referrers() == nil {
		return false
	}
	for _, use := range *addr.Referrers() {
		store, ok := use.(*ssa.Store)
		if !ok || store.Addr != ssa.Value(addr) {
			continue
		}
		if instructionPrecedes(store, escape) {
			return true
		}
	}
	return false
}

// instructionPrecedes reports whether a is guaranteed to have run by the
// time b runs: either a's block dominates b's, or both sit in one block
// with a earlier in it.
func instructionPrecedes(a, b ssa.Instruction) bool {
	blockA, blockB := a.Block(), b.Block()
	if blockA == nil || blockB == nil {
		return false
	}
	if blockA != blockB {
		return dominates(blockA, blockB)
	}
	for _, instr := range blockA.Instrs {
		if instr == a {
			return true
		}
		if instr == b {
			return false
		}
	}
	return false
}

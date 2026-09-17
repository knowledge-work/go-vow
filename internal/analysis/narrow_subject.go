package analysis

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// narrowSubject names the thing a guard has to speak about for the
// guard matcher to accept it as a proof. The matcher recognises a
// condition by shape and then asks the subject whether the compared
// operand is the thing under question, so what counts as "the same
// thing" is decided here rather than inside the condition recognisers.
//
// The distinction exists because two readings of the same source
// expression are not the same SSA value. A local or a parameter is one
// value that every mention refers to, while a struct field read twice
// produces two independent loads — the guard's load and the use site's
// load are separate values even though the source spells one field.
type narrowSubject interface {
	// matches reports whether v reads the thing this subject names.
	matches(v ssa.Value) bool
	// proofHoldsFrom reports whether a guard found on guardBlock's
	// terminating branch still speaks about the subject where the
	// subject is used. A subject whose value cannot change between the
	// two points answers true unconditionally.
	proofHoldsFrom(guardBlock *ssa.BasicBlock) bool
}

// valueSubject is the subject of a value that every mention shares:
// a parameter, a local, the result of a call. Identity is the whole
// question, because SSA gives such a value one definition.
type valueSubject struct {
	value ssa.Value
}

// matches reports whether v is the same SSA value.
func (s valueSubject) matches(v ssa.Value) bool {
	return v != nil && v == s.value
}

// proofHoldsFrom always accepts: nothing between the guard and the use
// can make it speak about a different value.
func (s valueSubject) proofHoldsFrom(*ssa.BasicBlock) bool {
	return true
}

// fieldSubject is the subject of one struct field reached through one
// base value — `t.Maybe` for a fixed `t`. Identity cannot answer here,
// so the subject matches by the address a load reads from: the same
// base value and the same field index within the struct.
//
// That bounds what the subject claims: nothing about `u.Maybe` for a
// different base, and nothing about the field after a write, which the
// caller rules out.
type fieldSubject struct {
	base  ssa.Value
	index int
	// useBlock and useIndex locate the read the proof has to reach, so
	// the subject can rule out a write that lands between the guard and
	// that read. useIndex is the position of the using instruction
	// within useBlock.Instrs.
	useBlock *ssa.BasicBlock
	useIndex int
}

// matches reports whether v is a load of this subject's field address.
// The shape is an `*ssa.UnOp` with the dereference operator over an
// `*ssa.FieldAddr`, which is how the builder renders a field read
// through a pointer.
func (s fieldSubject) matches(v ssa.Value) bool {
	load, ok := v.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	return s.matchesAddr(load.X)
}

// matchesAddr reports whether addr is this subject's field address.
func (s fieldSubject) matchesAddr(addr ssa.Value) bool {
	fa, ok := addr.(*ssa.FieldAddr)
	if !ok {
		return false
	}
	return fa.X == s.base && fa.Field == s.index
}

// fieldSubjectForSelector resolves a field-access expression into the
// subject a guard would have to speak about, given the base value the
// enclosing SSA function holds for the receiver identifier. Returns
// ok=false for a shape the subject cannot name: a receiver that is not
// a plain identifier the caller could resolve, or a selector whose
// field is not a field of a struct behind a pointer.
func fieldSubjectForSelector(base ssa.Value, field *types.Var) (fieldSubject, bool) {
	if base == nil || field == nil {
		return fieldSubject{}, false
	}
	st, ok := pointedStruct(base.Type())
	if !ok {
		return fieldSubject{}, false
	}
	index, ok := structFieldIndex(st, field)
	if !ok {
		return fieldSubject{}, false
	}
	return fieldSubject{base: base, index: index}, true
}

// pointedStruct returns the struct a pointer type points at. A base
// value that is not a pointer to a struct carries no FieldAddr-shaped
// read, so the field-subject path does not apply to it.
func pointedStruct(typ types.Type) (*types.Struct, bool) {
	ptr, ok := typ.Underlying().(*types.Pointer)
	if !ok {
		return nil, false
	}
	st, ok := ptr.Elem().Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	return st, true
}

// structFieldIndex returns the position of field within st. The
// comparison runs against the field's origin so a field reached
// through an instantiated generic struct lands on the same index its
// declaration sits at.
func structFieldIndex(st *types.Struct, field *types.Var) (int, bool) {
	target := field.Origin()
	for i := 0; i < st.NumFields(); i++ {
		candidate := st.Field(i)
		if candidate == field || candidate.Origin() == target {
			return i, true
		}
	}
	return 0, false
}

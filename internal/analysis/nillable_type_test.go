package analysis

import (
	"go/types"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// newNamedType wraps underlying in a named type, so the peel-to-
// underlying claim in typeIsNillable's contract is pinned on the
// surface form an author writes rather than on the structural type
// alone.
func newNamedType(name string, underlying types.Type) types.Type {
	return types.NewNamed(types.NewTypeName(0, nil, name, nil), underlying, nil)
}

// TestTypeIsNillableCoversReferenceKinds pins the kind taxonomy every
// nil-safety surface consults: non-nillable Go kinds (int, array,
// struct) report false and nil-bearing reference shapes report true,
// through a named wrapper as much as through the bare kind. The
// type-parameter case pins the conservative answer the predicate
// reaches through the constraint's interface, which is what the SSA
// narrow path relies on at a position whose type argument is not yet
// substituted.
func TestTypeIsNillableCoversReferenceKinds(t *testing.T) {
	typeParam := types.NewTypeParam(types.NewTypeName(0, nil, "T", nil), nil)
	typeParam.SetConstraint(types.NewInterfaceType(nil, nil))
	type nillableCase struct {
		t    types.Type
		want bool
	}

	tabletest.Run(t, map[string]nillableCase{
		"int":                          {types.Typ[types.Int], false},
		"array":                        {types.NewArray(types.Typ[types.Int], 4), false},
		"struct":                       {types.NewStruct(nil, nil), false},
		"pointer":                      {types.NewPointer(types.Typ[types.Int]), true},
		"slice":                        {types.NewSlice(types.Typ[types.Int]), true},
		"map":                          {types.NewMap(types.Typ[types.Int], types.Typ[types.Int]), true},
		"channel":                      {types.NewChan(types.SendRecv, types.Typ[types.Int]), true},
		"interface":                    {types.NewInterfaceType(nil, nil), true},
		"function":                     {types.NewSignatureType(nil, nil, nil, nil, nil, false), true},
		"named pointer":                {newNamedType("NamedPtr", types.NewPointer(types.Typ[types.Int])), true},
		"named struct":                 {newNamedType("NamedStruct", types.NewStruct(nil, nil)), false},
		"type-parameter-unsubstituted": {typeParam, true},
		"nil":                          {nil, false},
	}, func(t *testing.T, c nillableCase) {
		assert.Equal(t, "typeIsNillable", typeIsNillable(c.t), c.want)
	})
}

// newSelfPointerType builds `type SelfPtr *SelfPtr`, which the Go
// spec admits because the pointer breaks the size recursion. The
// layer walk follows pointers, so this is the shape that can hand it
// a chain with no end.
func newSelfPointerType() types.Type {
	named := types.NewNamed(types.NewTypeName(0, nil, "SelfPtr", nil), nil, nil)
	named.SetUnderlying(types.NewPointer(named))
	return named
}

// newMutualPointerTypes builds `type A *B` alongside `type B *A`, the
// two-step form of the same recursion.
func newMutualPointerTypes() (types.Type, types.Type) {
	a := types.NewNamed(types.NewTypeName(0, nil, "A", nil), nil, nil)
	b := types.NewNamed(types.NewTypeName(0, nil, "B", nil), nil, nil)
	a.SetUnderlying(types.NewPointer(b))
	b.SetUnderlying(types.NewPointer(a))
	return a, b
}

// TestNilLayerCountTerminatesOnRecursivePointer pins the walk against
// a named pointer type that reaches itself: the count is unbounded, so
// the answer is unsettled and every caller reading it stays silent.
// The straight chains alongside are the control that the walk still
// counts an ordinary type.
func TestNilLayerCountTerminatesOnRecursivePointer(t *testing.T) {
	mutualA, _ := newMutualPointerTypes()
	type layerCase struct {
		t           types.Type
		wantCount   int
		wantSettled bool
	}

	tabletest.Run(t, map[string]layerCase{
		"self pointer":       {newSelfPointerType(), 1, false},
		"mutual pointer":     {mutualA, 2, false},
		"pointer":            {types.NewPointer(types.Typ[types.Int]), 1, true},
		"pointer to pointer": {types.NewPointer(types.NewPointer(types.Typ[types.Int])), 2, true},
	}, func(t *testing.T, c layerCase) {
		count, settled := nilLayerCount(c.t)
		assert.Equal(t, "nilLayerCount count", count, c.wantCount)
		assert.Equal(t, "nilLayerCount settled", settled, c.wantSettled)
	})
}

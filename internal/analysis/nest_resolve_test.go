package analysis

import (
	"go/types"
	"strconv"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestResolveNestKindClassifiesContainers pins the way the resolver
// reads the surface type to decide whether a nest body should be
// interpreted as slice, map, or generic instantiation. The pointer-
// peel layer guarantees `*Foo[T]` and `**Foo[T]` resolve the same
// way `Foo[T]` does, mirroring the way an author reads a nest body
// through any pointer depth.
func TestResolveNestKindClassifiesContainers(t *testing.T) {
	intT := types.Typ[types.Int]
	slice := types.NewSlice(intT)
	mapT := types.NewMap(intT, intT)
	plainNamed := types.NewNamed(types.NewTypeName(0, nil, "Plain", nil), types.NewStruct(nil, nil), nil)
	interfaceT := types.NewInterfaceType(nil, nil)
	generic := newGenericNamed(t, "Box", 1)
	genericInstance, err := types.Instantiate(nil, generic, []types.Type{intT}, true)
	assert.MustNoError(t, "Instantiate", err)

	type kindCase struct {
		t    types.Type
		want nestKind
	}

	tabletest.Run(t, map[string]kindCase{
		"nil":                          {nil, nestKindUnknown},
		"slice":                        {slice, nestKindSlice},
		"pointer-to-slice":             {types.NewPointer(slice), nestKindSlice},
		"map":                          {mapT, nestKindMap},
		"non-generic-named":            {plainNamed, nestKindUnknown},
		"pointer-to-non-generic-named": {types.NewPointer(plainNamed), nestKindUnknown},
		"interface":                    {interfaceT, nestKindUnknown},
		"generic-instantiation":        {genericInstance, nestKindGeneric},
		"pointer-to-generic":           {types.NewPointer(genericInstance), nestKindGeneric},
		"double-pointer-to-generic":    {types.NewPointer(types.NewPointer(genericInstance)), nestKindGeneric},
		"bare-int":                     {intT, nestKindUnknown},
	}, func(t *testing.T, c kindCase) {
		assert.Equal(t, "resolveNestKind", resolveNestKind(c.t), c.want)
	})
}

// TestNestArityRangesPerKind pins the expected inner-decls range
// each container shape exposes — slices admit zero or one entry,
// maps require exactly one, single-type-parameter generics require
// exactly one, and multi-parameter generics require one per type
// argument.
func TestNestArityRangesPerKind(t *testing.T) {
	intT := types.Typ[types.Int]
	stringT := types.Typ[types.String]
	slice := types.NewSlice(intT)
	mapT := types.NewMap(intT, intT)
	generic1 := newGenericNamed(t, "Box", 1)
	box, err := types.Instantiate(nil, generic1, []types.Type{intT}, true)
	assert.MustNoError(t, "Instantiate Box", err)
	generic2 := newGenericNamed(t, "Pair", 2)
	pair, err := types.Instantiate(nil, generic2, []types.Type{intT, stringT}, true)
	assert.MustNoError(t, "Instantiate Pair", err)

	type arityCase struct {
		t              types.Type
		kind           nestKind
		wantLow, wantH int
	}

	tabletest.Run(t, map[string]arityCase{
		"slice 0..1":     {slice, nestKindSlice, 0, 1},
		"map 1..1":       {mapT, nestKindMap, 1, 1},
		"generic-1 1..1": {box, nestKindGeneric, 1, 1},
		"generic-2 2..2": {pair, nestKindGeneric, 2, 2},
		"unknown empty":  {intT, nestKindUnknown, 0, 0},
	}, func(t *testing.T, c arityCase) {
		low, high := nestArity(c.t, c.kind)
		assert.Equal(t, "nestArity low", low, c.wantLow)
		assert.Equal(t, "nestArity high", high, c.wantH)
	})
}

// TestTypeParamConcreteTypeReadsTypeArgs pins the way the resolver
// reads the i-th concrete type out of a generic instantiation. The
// helper is the bridge the follow-up SSA narrow path will use to
// tell whether an InnerDecls qualifier applies at this instantiation
// site.
func TestTypeParamConcreteTypeReadsTypeArgs(t *testing.T) {
	intT := types.Typ[types.Int]
	stringT := types.Typ[types.String]
	generic := newGenericNamed(t, "Pair", 2)
	instance, err := types.Instantiate(nil, generic, []types.Type{intT, stringT}, true)
	assert.MustNoError(t, "Instantiate", err)
	assert.Equal[types.Type](t, "typeParamConcreteType(0)", typeParamConcreteType(instance, 0), intT)
	assert.Equal[types.Type](t, "typeParamConcreteType(1) through a pointer",
		typeParamConcreteType(types.NewPointer(instance), 1), stringT)
	assert.Nil(t, "typeParamConcreteType with an out-of-range positive index",
		typeParamConcreteType(instance, 5))
	assert.Nil(t, "typeParamConcreteType with a negative index", typeParamConcreteType(instance, -1))
	assert.Nil(t, "typeParamConcreteType on a non-generic container", typeParamConcreteType(intT, 0))
}

// TestTypeParamIndexFindsContainerSlot pins the way the resolver
// locates a field's type-parameter position inside its container.
// The helper is the bridge the SSA narrow path uses to translate a
// `box.V` (field type `T`) access into an `InnerDecls[i]` lookup at
// the enclosing function's `vow:nil`.
func TestTypeParamIndexFindsContainerSlot(t *testing.T) {
	generic := newGenericNamed(t, "Pair", 2)
	tparams := generic.TypeParams()
	assert.MustEqual(t, "TypeParams().Len()", tparams.Len(), 2)
	first := tparams.At(0)
	second := tparams.At(1)

	intT := types.Typ[types.Int]
	stringT := types.Typ[types.String]
	instance, err := types.Instantiate(nil, generic, []types.Type{intT, stringT}, true)
	assert.MustNoError(t, "Instantiate", err)

	type indexCase struct {
		container types.Type
		fieldType types.Type
		want      int
	}

	tabletest.Run(t, map[string]indexCase{
		"first type param":                {instance, first, 0},
		"second type param":               {instance, second, 1},
		"first through pointer container": {types.NewPointer(instance), first, 0},
		"non-type-param field":            {instance, intT, -1},
		"pointer-wrapped type param":      {instance, types.NewPointer(first), -1},
		"nil field type":                  {instance, nil, -1},
		"non-generic container":           {intT, first, -1},
		"origin-only container":           {generic, first, 0},
		"foreign type param":              {instance, newGenericNamed(t, "Foreign", 1).TypeParams().At(0), -1},
	}, func(t *testing.T, c indexCase) {
		assert.Equal(t, "typeParamIndex", typeParamIndex(c.container, c.fieldType), c.want)
	})
}

// newGenericNamed builds a synthetic generic named type carrying
// arity type parameters constrained by the empty interface. The
// helper keeps the resolve / arity tests self-contained without
// depending on a parsed Go source — types.NewNamed accepts the
// non-nil struct underlying directly, so no separate SetUnderlying
// call is needed.
func newGenericNamed(t *testing.T, name string, arity int) *types.Named {
	t.Helper()
	tname := types.NewTypeName(0, nil, name, nil)
	body := types.NewStruct(nil, nil)
	named := types.NewNamed(tname, body, nil)
	params := make([]*types.TypeParam, arity)
	for i := 0; i < arity; i++ {
		paramName := types.NewTypeName(0, nil, "T"+strconv.Itoa(i+1), nil)
		tp := types.NewTypeParam(paramName, nil)
		tp.SetConstraint(types.NewInterfaceType(nil, nil))
		params[i] = tp
	}
	named.SetTypeParams(params)
	return named
}

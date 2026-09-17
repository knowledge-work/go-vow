package analysis

import "go/types"

// nestKind classifies the container shape an analyzer-side resolver
// derives from a Go type. The vow:nil nest grammar is uniform across
// these shapes — `[<inner>] <value>` admits slice, map, and generic
// instantiation alike — so the analyzer uses nestKind to decide how
// to read the InnerDecls list.
type nestKind int

const (
	// nestKindUnknown marks a type the resolver cannot classify into
	// any of the nest-supported shapes. Diagnostics that consult
	// nestKind treat Unknown the same way they treat a missing nest
	// body: nothing to enforce, nothing to narrow.
	nestKindUnknown nestKind = iota
	// nestKindSlice — the underlying type is a Go slice.
	nestKindSlice
	// nestKindMap — the underlying type is a Go map.
	nestKindMap
	// nestKindGeneric — the type is a named generic instantiation
	// carrying type arguments (e.g. `Foo[*Bar]` or `*Foo[*Bar, T]`).
	nestKindGeneric
)

// resolveNestKind returns the container shape of t. Pointer
// indirections are peeled before classification — `*Foo[T]` and the
// rarer `**Foo[T]` both resolve to nestKindGeneric — so the way an
// author writes the annotation through any pointer depth lines up
// with the shape the resolver returns.
func resolveNestKind(t types.Type) nestKind {
	t = peelPointers(t)
	if t == nil {
		return nestKindUnknown
	}
	switch t.Underlying().(type) {
	case *types.Slice:
		return nestKindSlice
	case *types.Map:
		return nestKindMap
	}
	if named, ok := t.(*types.Named); ok && named.TypeArgs().Len() > 0 {
		return nestKindGeneric
	}
	return nestKindUnknown
}

// nestArity returns the inclusive InnerDecls length the nest grammar
// permits for the given container. Slices accept zero or one inner
// decl (the bare `[]` form alongside the qualifier-only `[?]` form);
// maps require exactly one (the key decl); generic instantiations
// require one inner decl per type argument. Containers the resolver
// classifies as Unknown produce a (0, 0) range — the arity walker
// treats those positions as out of scope.
func nestArity(t types.Type, kind nestKind) (low, high int) {
	switch kind {
	case nestKindSlice:
		return 0, 1
	case nestKindMap:
		return 1, 1
	case nestKindGeneric:
		if named, ok := peelPointers(t).(*types.Named); ok {
			n := named.TypeArgs().Len()
			return n, n
		}
	}
	return 0, 0
}

// typeParamConcreteType returns the concrete type bound to the i-th
// type parameter of a generic instantiation, or nil when t is not a
// generic instantiation or when i is out of range. The caller uses
// the result to decide what stands at that type-argument position at
// this instantiation site.
func typeParamConcreteType(t types.Type, i int) types.Type {
	named, ok := peelPointers(t).(*types.Named)
	if !ok {
		return nil
	}
	args := named.TypeArgs()
	if i < 0 || i >= args.Len() {
		return nil
	}
	return args.At(i)
}

// originFieldType returns the field's type as declared on the
// container's origin (un-instantiated) struct, or nil when the
// origin does not expose a field by that name. The caller uses
// the result to recover the type parameter the field references —
// `pass.TypesInfo.ObjectOf(sel.Sel).Type()` substitutes the type
// argument before reaching here, so the origin walk is the reliable
// route back to the type parameter itself.
func originFieldType(container types.Type, fieldName string) types.Type {
	named, ok := peelPointers(container).(*types.Named)
	if !ok {
		return nil
	}
	origin := named.Origin()
	if origin == nil {
		return nil
	}
	st, ok := origin.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Name() == fieldName {
			return f.Type()
		}
	}
	return nil
}

// typeParamIndex returns the position at which fieldType appears in
// the container's type-parameter list, or -1 when fieldType is not
// one of the container's type parameters. Pointer indirections on
// the container are peeled before the search; the field side is
// matched as-is, so a `T` field locates its type parameter while a
// `*T` field answers -1 unless the caller peels it first.
func typeParamIndex(container types.Type, fieldType types.Type) int {
	if fieldType == nil {
		return -1
	}
	tp, ok := fieldType.(*types.TypeParam)
	if !ok {
		return -1
	}
	named, ok := peelPointers(container).(*types.Named)
	if !ok {
		return -1
	}
	params := named.Origin().TypeParams()
	for i := 0; i < params.Len(); i++ {
		if params.At(i) == tp {
			return i
		}
	}
	return -1
}

// peelPointers strips any number of outer *types.Pointer layers, so
// the resolver classifies `*Foo[T]`, `**Foo[T]`, and the bare
// `Foo[T]` identically. The loose-peel default matches the way an
// author reads a nest body through whatever pointer depth the Go
// signature carries.
func peelPointers(t types.Type) types.Type {
	for {
		ptr, ok := t.(*types.Pointer)
		if !ok {
			return t
		}
		t = ptr.Elem()
	}
}

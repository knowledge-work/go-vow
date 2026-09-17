package analysis

import "go/types"

// typeIsNillable reports whether t's zero value is nil under Go's
// kind taxonomy: pointers, interfaces, slices, maps, channels, and
// functions. Named-type wrappers are peeled to their underlying type
// so the surface form (named pointer vs raw pointer, alias vs
// structural) does not alter the answer.
//
// A type parameter answers true, because its underlying type is its
// constraint's interface, which the predicate reads as the nil-bearing
// interface kind. That holds even for a constraint no nil-bearing type
// satisfies (`T int`), so the answer is conservative in one direction
// only: it errs safe where true widens an analysis, and errs wrong
// where true would demand a declaration. A caller of the second kind
// answers the type-parameter case from the constraint instead of
// asking here.
func typeIsNillable(t types.Type) bool {
	if t == nil {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Slice, *types.Map, *types.Chan, *types.Signature:
		return true
	}
	return false
}

// nilLayerCount returns how many layers of t can hold nil and whether
// that number is settled. t itself is a layer when its own kind can
// hold nil, and the walk descends only through pointers, so it stops
// at the first nillable layer that is not one: `int` carries none,
// `*Value` one, and `*error` two — the pointer and the interface it
// addresses.
//
// An unsettled count is a lower bound rather than an answer, which is
// what a type parameter leaves behind: the walk stops there, and only
// the constraint can settle what lies beyond. `~int | ~int64` settles
// at none, while `any`, `comparable`, and a method-set constraint do
// not settle at all. Reading the constraint here is what keeps
// typeIsNillable's type-parameter answer out of the count.
//
// A named type that reaches itself through pointers (`type P *P`)
// never settles: the chain has no end, so no count bounds it from
// above.
func nilLayerCount(t types.Type) (int, bool) {
	count := 0
	seen := map[*types.Named]bool{}
	for t != nil {
		if named, ok := t.(*types.Named); ok {
			if seen[named] {
				return count, false
			}
			seen[named] = true
		}
		if param, ok := t.(*types.TypeParam); ok {
			return count, typeParamExcludesNil(param)
		}
		if !typeIsNillable(t) {
			return count, true
		}
		count++
		ptr, ok := t.Underlying().(*types.Pointer)
		if !ok {
			return count, true
		}
		t = ptr.Elem()
	}
	return count, false
}

// typeParamExcludesNil reports whether param's constraint proves that
// no instantiation of it can be nil.
func typeParamExcludesNil(param *types.TypeParam) bool {
	if param.Constraint() == nil {
		return false
	}
	iface, ok := param.Constraint().Underlying().(*types.Interface)
	return ok && constraintExcludesNil(iface)
}

// constraintExcludesNil reports whether iface's type set is known to
// hold non-nillable types only. Each embedded element is read on its
// own because the type set is the intersection of the elements, so one
// element that admits no nillable type settles the whole constraint.
// False is the answer for everything else, including a constraint that
// admits nillable types only: an interface with no embedded element —
// `any`, `comparable`, a method-set constraint — answers false the same
// way `interface{ *int }` does.
func constraintExcludesNil(iface *types.Interface) bool {
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		if embeddedExcludesNil(iface.EmbeddedType(i)) {
			return true
		}
	}
	return false
}

// embeddedExcludesNil reports whether one embedded constraint element
// holds non-nillable types only. An element is a union of terms
// (`~int | ~int64`), a single type (`interface{ int }`), or a further
// interface whose own elements decide it.
func embeddedExcludesNil(t types.Type) bool {
	if union, ok := t.(*types.Union); ok {
		return unionExcludesNil(union)
	}
	if iface, ok := t.Underlying().(*types.Interface); ok {
		return constraintExcludesNil(iface)
	}
	return !typeIsNillable(t)
}

// unionExcludesNil reports whether union names non-nillable types
// only. A tilde term (`~int`) is read through the type it names,
// because every type with that underlying type shares its kind. A
// union carrying no term answers false: an empty set settles nothing.
func unionExcludesNil(union *types.Union) bool {
	for i := 0; i < union.Len(); i++ {
		if typeIsNillable(union.Term(i).Type()) {
			return false
		}
	}
	return union.Len() > 0
}

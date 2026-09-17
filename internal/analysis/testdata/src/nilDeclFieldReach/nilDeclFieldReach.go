// Package nilDeclFieldReach pins which positions a `vow:nil` field
// marker is read at. The decl collection reads the fields of a declared
// struct type; a marker anywhere else exports no fact and no check
// consults it, so the marker is reported rather than dropped — an inert
// contract that reads as enforced is worse than no contract.
package nilDeclFieldReach

// Foo is the pointee the markers below declare about.
type Foo struct{ N int }

// Tracked is the baseline: a field of a declared struct type, which the
// collection reads and exports a fact for.
type Tracked struct {
	// vow:nil !
	Ref *Foo // want Ref:"fieldNil\\(!\\)"

	// vow:nil ?
	Maybe *Foo // want Maybe:"fieldNil\\(\\?\\)"

	// Plain carries no marker at all, so nothing is reported about it
	// at any position.
	Plain *Foo
}

// AnonymousNested holds an anonymous struct type. Its fields belong to
// a type the collection never names, so a marker there is inert.
type AnonymousNested struct {
	Inner struct {
		// vow:nil !
		Ref *Foo // want `vow\[nil-safety\]: vow:nil on a field of an anonymous struct type is not tracked; give the struct type a name to declare on its fields`
	}

	// Unmarked fields of the same anonymous struct are silent — the
	// report is about a marker, not about the position.
	Other struct {
		Ref *Foo
	}
}

// EmbeddedMarked carries a marker on an embedded field, which has no
// name for a decl to be keyed against.
type EmbeddedMarked struct {
	// vow:nil !
	*Foo // want `vow\[nil-safety\]: vow:nil on an embedded field is not tracked; declare the nullness on the embedded type's own fields`
}

// EmbeddedUnmarked embeds without a marker, which is the ordinary case
// and stays silent.
type EmbeddedUnmarked struct {
	*Foo
}

// looseAnonymous pins the marker inside an anonymous struct used
// directly as a variable's type, which no type declaration names.
var looseAnonymous struct {
	// vow:nil !
	Ref *Foo // want `vow\[nil-safety\]: vow:nil on a field of an anonymous struct type is not tracked; give the struct type a name to declare on its fields`
}

// anonymousInsideFunc pins the same position inside a function body,
// where the collection likewise never reaches.
func anonymousInsideFunc() int {
	local := struct {
		// vow:nil !
		Ref *Foo // want `vow\[nil-safety\]: vow:nil on a field of an anonymous struct type is not tracked; give the struct type a name to declare on its fields`
	}{}
	if local.Ref == nil {
		return 0
	}
	return local.Ref.N
}

// MalformedAtUnreachedPosition carries a marker whose payload does not
// parse, at a position the collection never reads. Only the reach
// diagnostic surfaces: the parse error would name a contract that was
// never going to be consulted.
type MalformedAtUnreachedPosition struct {
	Inner struct {
		// vow:nil (xyz)
		Bad *Foo // want `vow\[nil-safety\]: vow:nil on a field of an anonymous struct type is not tracked; give the struct type a name to declare on its fields`
	}
}

// MalformedAtTrackedPosition keeps the parse error where the collection
// does read the field, so the two diagnostics stay distinguishable.
//
// This case pins how reachability is decided. Deciding it from the set
// of registered decls instead of from the shape of the declaration
// would call this field unreached — a malformed payload registers no
// decl — and stack a spurious "not tracked" on top of the parse error.
// Asking which struct types the collection walks keeps the two answers
// separate: this position is read, and what was written there does not
// parse.
type MalformedAtTrackedPosition struct {
	// vow:nil (xyz)
	Bad *Foo // want `vow\[nil-safety\]: vow:nil: vow:nil .*: field: expected .*got "\(xyz\)"`
}

// The three positions below are anonymous struct types reached through
// a type constructor rather than through a declaration. They are pinned
// because the walk visits every struct type node in the file: narrowing
// it to type and var declarations — the shape the collection itself
// uses — would leave these in the inert state this check exists to
// surface.

// mapValueAnonymous holds the marker in a map's value type.
var mapValueAnonymous map[string]struct {
	// vow:nil !
	Ref *Foo // want `vow\[nil-safety\]: vow:nil on a field of an anonymous struct type is not tracked; give the struct type a name to declare on its fields`
}

// sliceElementAnonymous holds the marker in a slice's element type.
var sliceElementAnonymous []struct {
	// vow:nil !
	Ref *Foo // want `vow\[nil-safety\]: vow:nil on a field of an anonymous struct type is not tracked; give the struct type a name to declare on its fields`
}

// paramTypeAnonymous holds the marker in a parameter's type, which no
// declaration names at all.
func paramTypeAnonymous(a struct {
	// vow:nil !
	Ref *Foo // want `vow\[nil-safety\]: vow:nil on a field of an anonymous struct type is not tracked; give the struct type a name to declare on its fields`
}) {
}

// Package condTupleXPkgType pins the tuple-element match for a
// term that names an imported type.
//
// vow:import result "preset/std/result"
package condTupleXPkgType

import (
	"errors"

	"condTupleXPkgPayload"
)

// newFoo is the shape std/result exists to describe, written
// against a type the file had to import.
//
// vow:cond @result.OkErr[*payload.Foo, error]
func newFoo(bad bool) (*payload.Foo, error) {
	if bad {
		return nil, errors.New("bad")
	}
	return &payload.Foo{V: 1}, nil
}

// newFooViaCall routes the payload through a constructor, so the
// return expression names no type of its own.
//
// vow:cond @result.OkErr[*payload.Foo, error]
func newFooViaCall(bad bool) (*payload.Foo, error) {
	if bad {
		return nil, errors.New("bad")
	}
	return payload.New(), nil
}

// passThroughFoo hands back a parameter, so the element is an
// identifier with no defining expression to resolve to and the
// match runs on the name axes rather than the type-only path the
// two above take.
//
// vow:cond @result.OkErr[*payload.Foo, error]
func passThroughFoo(bad bool, foo *payload.Foo) (*payload.Foo, error) {
	if bad {
		return nil, errors.New("bad")
	}
	return foo, nil
}

// newFooBothNil violates the sum, so the widened match is shown
// not to have softened it.
//
// vow:cond @result.OkErr[*payload.Foo, error]
func newFooBothNil(bad bool) (*payload.Foo, error) {
	if bad {
		return nil, nil // want `vow\[sentinel-error\]: return values do not match any tuple of the declared sum`
	}
	return &payload.Foo{V: 1}, nil
}

// Bar stands in for a payload the rule does not name.
type Bar struct{ v int }

// newFooWrongType returns a Bar where the rule names the imported
// Foo, so the axis discriminates across the boundary too.
//
// vow:cond @result.OkErr[*payload.Foo, error]
func newFooWrongType(bad bool) (any, error) {
	if bad {
		return nil, errors.New("bad")
	}
	return &Bar{v: 1}, nil // want `vow\[sentinel-error\]: return values do not match any tuple of the declared sum`
}

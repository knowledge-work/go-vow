// vow:import result "preset/std/result"
package condTupleTypedElement

import "errors"

// Foo is the payload the constructors return.
type Foo struct{ v int }

// wrapf stands in for the error helpers a constructor reaches for
// instead of errors.New.
func wrapf(msg string) error { return errors.New(msg) }

// newFoo is the shape std/result exists to describe: a composite
// literal address paired with a nil error, or a nil payload paired
// with a freshly built one.
//
// vow:cond @result.OkErr[*Foo, error]
func newFoo(bad bool) (*Foo, error) {
	if bad {
		return nil, errors.New("bad")
	}
	return &Foo{v: 1}, nil
}

// newFooHelperErr routes the failure path through a helper, so the
// error position is a call whose type is all the term can read.
//
// vow:cond @result.OkErr[*Foo, error]
func newFooHelperErr(bad bool) (*Foo, error) {
	if bad {
		return nil, wrapf("bad")
	}
	return &Foo{v: 1}, nil
}

// wrapErr is the project-helper shape: it hands back a concrete
// error type rather than the interface the position declares.
type wrapErr struct{ msg string }

func (e *wrapErr) Error() string { return e.msg }

func newWrapErr(msg string) *wrapErr { return &wrapErr{msg: msg} }

// newFooConcreteErr fills the error position with a concrete type.
// The sum talks about the position, whose declared type is error.
//
// vow:cond @result.OkErr[*Foo, error]
func newFooConcreteErr(bad bool) (*Foo, error) {
	if bad {
		return nil, newWrapErr("bad")
	}
	return &Foo{v: 1}, nil
}

// newFooBothNil violates the sum: neither tuple admits a nil
// payload beside a nil error.
//
// vow:cond @result.OkErr[*Foo, error]
func newFooBothNil(bad bool) (*Foo, error) {
	if bad {
		return nil, nil // want `vow\[sentinel-error\]: return values do not match any tuple of the declared sum`
	}
	return &Foo{v: 1}, nil
}

// Bar is a second payload type so the fixture can show that the
// type axis discriminates rather than admitting any call.
type Bar struct{ v int }

func newBar() *Bar { return &Bar{} }

// newFooWrongType returns a Bar where the rule names Foo.
//
// vow:cond @result.OkErr[*Foo, error]
func newFooWrongType(bad bool) (any, error) {
	if bad {
		return nil, errors.New("bad")
	}
	return newBar(), nil // want `vow\[sentinel-error\]: return values do not match any tuple of the declared sum`
}

// newFooTypedNil hands back a typed nil where the rule requires a
// non-zero payload. Matching on the type axis alone would admit it,
// so the nil recogniser has to run before the element passes.
//
// vow:cond @result.OkErr[*Foo, error]
func newFooTypedNil() (*Foo, error) {
	return (*Foo)(nil), nil // want `vow\[sentinel-error\]: return values do not match any tuple of the declared sum`
}

// newFooViaLocal binds the payload to a local before returning it.
// The return expression is then an identifier, which reaches the
// name axes first and only lands on the type axes through the
// package-relative rendering the annotation actually spells.
//
// vow:cond @result.OkErr[*Foo, error]
func newFooViaLocal(f *Foo) (*Foo, error) {
	return f, nil
}

// newPairGrouped uses a grouped result list so the slot walk has to
// expand one field into two positions.
//
// vow:cond * -> (nonzero *Foo, nonzero *Foo)
func newPairGrouped() (first, second *Foo) {
	return &Foo{v: 1}, &Foo{v: 2}
}

// newPairGroupedNil violates the second position of that same
// grouped list.
//
// vow:cond * -> (nonzero *Foo, nonzero *Foo)
func newPairGroupedNil() (first, second *Foo) {
	return &Foo{v: 1}, nil // want `vow\[sentinel-error\]: return values do not match any tuple of the declared sum`
}

// newFooWildcard leaves the payload position unconstrained. A
// wildcard states "no constraint here", so the call it admits is
// the same set an identifier position already admitted.
//
// vow:cond * -> (*, nil)
func newFooWildcard() (*Foo, error) {
	return newBar2(), nil
}

func newBar2() *Foo { return &Foo{} }

// newFooPlaceholder uses the label-aware fall-through in the
// payload position.
//
// vow:cond * -> (_, nil)
func newFooPlaceholder() (*Foo, error) {
	return &Foo{v: 1}, nil
}

// singlePositionCall keeps a non-tuple sum, which reads the same
// shape but routes through validateSumPosition.
//
// vow:cond * -> ErrKnown | nil
func singlePositionCall() error {
	return errors.New("bad")
}

// ErrKnown gives the single-position sum a named member.
var ErrKnown = errors.New("known")

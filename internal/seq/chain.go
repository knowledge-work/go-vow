package seq

import (
	"fmt"
	"iter"
	"slices"
)

// Chain is a chainable view over an iter.Seq. NewChain and ChainOf start
// a chain; ToSeq returns the underlying iter.Seq for APIs that take one.
type Chain[T any] iter.Seq[T]

// Sequence constrains a type parameter to iter.Seq[T] and Chain[T], so
// FlatMap and FlatMapOrErr take a function returning either of them
// without a conversion.
//
// The two are listed rather than admitted by underlying type: a
// FailableChain carries outcomes that are a value or an error, and under
// ~func(func(T) bool) it would arrive here as a chain of those outcomes
// with the errors turned into elements.
type Sequence[T any] interface {
	iter.Seq[T] | Chain[T]
}

// NewChain wraps an iter.Seq to start a method chain. Converting with
// Chain[T](input) does the same, so this exists to infer T instead of
// spelling it out.
// vow:nil (!) !
func NewChain[T any](input iter.Seq[T]) Chain[T] {
	return Chain[T](input)
}

// ChainOf returns a Chain over the given values.
// vow:nil (?) !
func ChainOf[T any](values ...T) Chain[T] {
	return Chain[T](slices.Values(values))
}

// Filter keeps the elements for which f reports true.
// vow:nil (!) !
func (c Chain[T]) Filter(f func(T) bool) Chain[T] {
	return func(yield func(T) bool) {
		for v := range c {
			if f(v) && !yield(v) {
				return
			}
		}
	}
}

// FilterOrErr keeps the elements for which f reports true, and stops at
// the first error f reports.
// vow:nil (!) !
func (c Chain[T]) FilterOrErr(f func(T) (bool, error)) FailableChain[T] {
	return c.failable().FilterOrErr(f)
}

// Map applies f to every element.
// vow:nil (!) !
func (c Chain[T]) Map[U any](f func(T) U) Chain[U] {
	return func(yield func(U) bool) {
		for v := range c {
			if !yield(f(v)) {
				return
			}
		}
	}
}

// MapOrErr applies f to every element, and stops at the first error f
// reports.
// vow:nil (!) !
func (c Chain[T]) MapOrErr[U any](f func(T) (U, error)) FailableChain[U] {
	return c.failable().MapOrErr(f)
}

// FilterMap yields f(v) only for the elements where f returns true as its
// second return value.
// vow:nil (!) !
func (c Chain[T]) FilterMap[U any](f func(T) (U, bool)) Chain[U] {
	return func(yield func(U) bool) {
		for v := range c {
			u, keep := f(v)
			if keep && !yield(u) {
				return
			}
		}
	}
}

// FilterMapOrErr yields f(v) only for the elements where f returns true as
// its second return value, and stops at the first error f reports.
// vow:nil (!) !
func (c Chain[T]) FilterMapOrErr[U any](f func(T) (U, bool, error)) FailableChain[U] {
	return c.failable().FilterMapOrErr(f)
}

// FlatMap flattens the sequences f returns.
// vow:nil (!) !
func (c Chain[T]) FlatMap[U any, S Sequence[U]](f func(T) S) Chain[U] {
	return func(yield func(U) bool) {
		for v := range c {
			for u := range f(v) {
				if !yield(u) {
					return
				}
			}
		}
	}
}

// FlatMapOrErr flattens the sequences f returns, and stops at the first
// error f reports.
// vow:nil (!) !
func (c Chain[T]) FlatMapOrErr[U any, S Sequence[U]](f func(T) (S, error)) FailableChain[U] {
	return c.failable().FlatMapOrErr(f)
}

// Take returns a chain over the first n elements. A zero n takes none, and
// a negative n is no limit: the chain then ends only when its source does,
// so an endless source makes it endless too.
// vow:nil () !
func (c Chain[T]) Take(n int) Chain[T] {
	return func(yield func(T) bool) {
		if n == 0 {
			return
		}
		rest := n
		for v := range c {
			if !yield(v) {
				return
			}
			rest--
			if rest == 0 {
				return
			}
		}
	}
}

// TakeWhile returns a chain over the elements before the first one for
// which f reports false. Where Filter drops that element and carries on,
// TakeWhile ends the chain there.
// vow:nil (!) !
func (c Chain[T]) TakeWhile(f func(T) bool) Chain[T] {
	return func(yield func(T) bool) {
		for v := range c {
			if !f(v) || !yield(v) {
				return
			}
		}
	}
}

// TakeWhileOrErr returns a chain over the elements before the first one
// for which f reports false, and stops at the first error f reports.
// vow:nil (!) !
func (c Chain[T]) TakeWhileOrErr(f func(T) (bool, error)) FailableChain[T] {
	return c.failable().TakeWhileOrErr(f)
}

// Skip returns a chain that discards the first n elements. A negative n
// discards none.
// vow:nil () !
func (c Chain[T]) Skip(n int) Chain[T] {
	return func(yield func(T) bool) {
		rest := n
		for v := range c {
			if rest > 0 {
				rest--
				continue
			}
			if !yield(v) {
				return
			}
		}
	}
}

// UniqFunc keeps the first element per key f returns.
// vow:nil (!) !
func (c Chain[T]) UniqFunc[K comparable](f func(T) K) Chain[T] {
	return func(yield func(T) bool) {
		seen := make(map[K]struct{})
		for v := range c {
			k := f(v)
			if _, duplicated := seen[k]; duplicated {
				continue
			}
			seen[k] = struct{}{}
			if !yield(v) {
				return
			}
		}
	}
}

// UniqFuncOrErr keeps the first element per key f returns, and stops at
// the first error f reports.
// vow:nil (!) !
func (c Chain[T]) UniqFuncOrErr[K comparable](f func(T) (K, error)) FailableChain[T] {
	return c.failable().UniqFuncOrErr(f)
}

// Chunked returns a Chain over consecutive runs of up to n elements, the
// last of which may be shorter. It panics if n is less than one, as
// slices.Chunk does.
//
// Each run is a fresh slice. Reusing one buffer would rewrite a run the
// consumer still holds; slices.Chunk can alias its input because that
// input outlives the iteration, and an iterator has no such backing array.
//
// It is a function rather than a method because the compiler rejects
// Chain[T] -> Chain[[]T] as an instantiation cycle.
// vow:nil (!,) !
func Chunked[T any](c Chain[T], n int) Chain[[]T] {
	if n < 1 {
		panic(fmt.Sprintf("seq: Chunked: n must be at least one, got %d", n))
	}
	return func(yield func([]T) bool) {
		chunk := make([]T, 0, n)
		for v := range c {
			chunk = append(chunk, v)
			if len(chunk) < n {
				continue
			}
			if !yield(chunk) {
				return
			}
			chunk = make([]T, 0, n)
		}
		if len(chunk) > 0 {
			yield(chunk)
		}
	}
}

// Indexed pairs an element with its position in the chain WithIndex was
// applied to.
type Indexed[T any] struct {
	Index int
	Value T
}

// WithIndex pairs every element with its position, counting from zero.
// The positions number the elements that reach WithIndex, so a Filter
// before it renumbers them and a Filter after it leaves gaps.
//
// It is a function rather than a method because Indexed[T] is built from
// T, which makes Chain[T] -> Chain[Indexed[T]] an instantiation cycle the
// compiler rejects, as on Chunked.
// vow:nil (!) !
func WithIndex[T any](c Chain[T]) Chain[Indexed[T]] {
	return func(yield func(Indexed[T]) bool) {
		next := 0
		for v := range c {
			if !yield(Indexed[T]{Index: next, Value: v}) {
				return
			}
			next++
		}
	}
}

// ToSeq returns the underlying iter.Seq.
// vow:nil () ?
func (c Chain[T]) ToSeq() iter.Seq[T] {
	return iter.Seq[T](c)
}

// ToSlice returns the elements as a slice. An empty chain gives a nil
// slice rather than an empty one.
// vow:nil () ?
func (c Chain[T]) ToSlice() []T {
	// Delegating to slices.Collect tips this past the inliner's budget, and an
	// uninlined body heap-allocates the chain's closures. ToSeq barely fits.
	return slices.AppendSeq([]T(nil), iter.Seq[T](c))
}

// ToMap collects the elements into a map by the key and value f returns
// for each of them. An empty chain gives an empty map rather than a nil
// one, and where f gives the same key for several elements the map keeps
// the value from the last of them.
// vow:nil (!) !
func (c Chain[T]) ToMap[K comparable, V any](f func(T) (K, V)) map[K]V {
	collected := map[K]V{}
	for v := range c {
		key, value := f(v)
		collected[key] = value
	}
	return collected
}

// ToMapOrErr collects the elements into a map by the key and value f
// returns for each of them, and stops at the first error f reports. It
// returns a nil map together with the error.
// vow:nil (!) ?,
func (c Chain[T]) ToMapOrErr[K comparable, V any](f func(T) (K, V, error)) (map[K]V, error) {
	return c.failable().ToMapOrErr(f)
}

// ToSet collects the keys f returns for the elements into a set. An empty
// chain gives an empty set rather than a nil one.
// vow:nil (!) !
func (c Chain[T]) ToSet[K comparable](f func(T) K) map[K]struct{} {
	collected := map[K]struct{}{}
	for v := range c {
		collected[f(v)] = struct{}{}
	}
	return collected
}

// Count returns the number of elements.
// vow:nil ()
func (c Chain[T]) Count() int {
	// Delegating to Reduce tips this past the inliner's budget, as on ToSlice.
	n := 0
	for range c {
		n++
	}
	return n
}

// Any reports whether f returns true for at least one element, stopping at
// the first one that satisfies it. It reports false for an empty chain.
// vow:nil (!)
func (c Chain[T]) Any(f func(T) bool) bool {
	// Delegating to Find tips this past the inliner's budget, as on ToSlice.
	for v := range c {
		if f(v) {
			return true
		}
	}
	return false
}

// AnyOrErr reports whether f returns true for at least one element,
// stopping at the first one that satisfies it and at the first error f
// reports, whichever comes first. It reports false together with the error.
// vow:nil (!) ,
func (c Chain[T]) AnyOrErr(f func(T) (bool, error)) (bool, error) {
	_, found, err := c.FindOrErr(f)
	return found, err
}

// All reports whether f returns true for every element, stopping at the
// first one that fails it. It reports true for an empty chain.
// vow:nil (!)
func (c Chain[T]) All(f func(T) bool) bool {
	// Delegating to Find tips this past the inliner's budget, as on ToSlice.
	for v := range c {
		if !f(v) {
			return false
		}
	}
	return true
}

// AllOrErr reports whether f returns true for every element, stopping at
// the first one that fails it and at the first error f reports, whichever
// comes first. It reports false together with the error.
// vow:nil (!) ,
func (c Chain[T]) AllOrErr(f func(T) (bool, error)) (bool, error) {
	_, failed, err := c.FindOrErr(func(v T) (bool, error) {
		holds, err := f(v)
		return !holds, err
	})
	if err != nil {
		return false, err
	}
	return !failed, nil
}

// Find returns the first element for which f reports true. The second
// return value is false when no element does.
// vow:nil (!) ,
func (c Chain[T]) Find(f func(T) bool) (T, bool) {
	for v := range c {
		if f(v) {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// FindOrErr returns the first element for which f reports true. The second
// return value is false when no element does, and when the iteration stops
// at an error f reports.
// vow:nil (!) ,,
func (c Chain[T]) FindOrErr(f func(T) (bool, error)) (T, bool, error) {
	return c.failable().FindOrErr(f)
}

// Reduce accumulates every element into the accumulator, starting from
// initial.
// vow:nil (,!)
func (c Chain[T]) Reduce[U any](initial U, f func(U, T) U) U {
	acc := initial
	for v := range c {
		acc = f(acc, v)
	}
	return acc
}

// ReduceOrErr accumulates every element into the accumulator, starting
// from initial, and stops at the first error f reports. It returns the
// zero accumulator together with the error.
// vow:nil (,!) ,
func (c Chain[T]) ReduceOrErr[U any](initial U, f func(U, T) (U, error)) (U, error) {
	return c.failable().ReduceOrErr(initial, f)
}

// Each calls f for every element.
// vow:nil (!)
func (c Chain[T]) Each(f func(T)) {
	for v := range c {
		f(v)
	}
}

// EachOrErr calls f for every element, and stops at the first error f
// reports.
// vow:nil (!)
func (c Chain[T]) EachOrErr(f func(T) error) error {
	return c.failable().EachOrErr(f)
}

// failable lifts the chain into a FailableChain in which every element is
// a successful outcome.
// vow:nil () !
func (c Chain[T]) failable() FailableChain[T] {
	return func(yield func(either[T]) bool) {
		for v := range c {
			if !yield(either[T]{value: v}) {
				return
			}
		}
	}
}

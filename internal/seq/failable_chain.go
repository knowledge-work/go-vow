// Package seq turns a loop over an iter.Seq into a chain of method calls.
//
// Chain carries a sequence that cannot fail, FailableChain one whose
// iteration can. The OrErr methods take a function that reports an error;
// calling one on a Chain lifts it into a FailableChain, so a chain that
// starts out infallible becomes failable at the step that can fail.
//
// Either kind can be iterated as many times as its source can. Every
// method keeps its per-iteration state inside the sequence it returns, so
// a replayable source stays replayable and a one-shot one does not.
package seq

import "iter"

// FailableChain is a chainable view over a sequence whose iteration can
// fail. NewFailableChain starts one; ToSeq2 and ToSlice end it.
//
// Iteration stops at the first error, so a value and an error never
// arrive together and at most one error is reported. A method taking a
// function that cannot fail does not apply it to a failed element.
type FailableChain[T any] iter.Seq[either[T]]

// either carries one step's outcome: the value it produced, or the error
// that ends the chain. It is unexported so that no caller can build an
// outcome holding both, or holding neither.
type either[T any] struct {
	value T
	err   error
}

// NewFailableChain wraps an iter.Seq2 of elements and errors to start a
// method chain. It ends the iteration at the first non-nil error, so a
// source that keeps going after one is cut short.
// vow:nil (!) !
func NewFailableChain[T any](input iter.Seq2[T, error]) FailableChain[T] {
	return func(yield func(either[T]) bool) {
		for v, err := range input {
			if err != nil {
				yield(either[T]{err: err})
				return
			}
			if !yield(either[T]{value: v}) {
				return
			}
		}
	}
}

// Filter keeps the elements for which f reports true.
// vow:nil (!) !
func (fc FailableChain[T]) Filter(f func(T) bool) FailableChain[T] {
	return fc.FilterOrErr(func(v T) (bool, error) { return f(v), nil })
}

// FilterOrErr keeps the elements for which f reports true, and stops at
// the first error f reports.
// vow:nil (!) !
func (fc FailableChain[T]) FilterOrErr(f func(T) (bool, error)) FailableChain[T] {
	return fc.FilterMapOrErr(func(v T) (T, bool, error) {
		keep, err := f(v)
		return v, keep, err
	})
}

// Map applies f to every element.
// vow:nil (!) !
func (fc FailableChain[T]) Map[U any](f func(T) U) FailableChain[U] {
	return fc.MapOrErr(func(v T) (U, error) { return f(v), nil })
}

// MapOrErr applies f to every element, and stops at the first error f
// reports.
// vow:nil (!) !
func (fc FailableChain[T]) MapOrErr[U any](f func(T) (U, error)) FailableChain[U] {
	return fc.FilterMapOrErr(func(v T) (U, bool, error) {
		u, err := f(v)
		return u, true, err
	})
}

// FilterMap yields f(v) only for the elements where f returns true as its
// second return value.
// vow:nil (!) !
func (fc FailableChain[T]) FilterMap[U any](f func(T) (U, bool)) FailableChain[U] {
	return fc.FilterMapOrErr(func(v T) (U, bool, error) {
		u, keep := f(v)
		return u, keep, nil
	})
}

// FilterMapOrErr yields f(v) only for the elements where f returns true as
// its second return value, and stops at the first error f reports. The
// other element-wise methods are expressed in terms of this one, except
// FlatMapOrErr, Take, TakeWhileOrErr, Skip and UniqFuncOrErr: each drives
// its own loop, so the properties FailableChain's own doc comment states
// have to be rechecked at each of them rather than only here.
// vow:nil (!) !
func (fc FailableChain[T]) FilterMapOrErr[U any](f func(T) (U, bool, error)) FailableChain[U] {
	return func(yield func(either[U]) bool) {
		for e := range fc {
			if e.err != nil {
				yield(either[U]{err: e.err})
				return
			}
			u, keep, err := f(e.value)
			if err != nil {
				yield(either[U]{err: err})
				return
			}
			if keep && !yield(either[U]{value: u}) {
				return
			}
		}
	}
}

// FlatMap flattens the sequences f returns.
// vow:nil (!) !
func (fc FailableChain[T]) FlatMap[U any, S Sequence[U]](f func(T) S) FailableChain[U] {
	return fc.FlatMapOrErr(func(v T) (S, error) { return f(v), nil })
}

// FlatMapOrErr flattens the sequences f returns, and stops at the first
// error f reports.
// vow:nil (!) !
func (fc FailableChain[T]) FlatMapOrErr[U any, S Sequence[U]](f func(T) (S, error)) FailableChain[U] {
	return func(yield func(either[U]) bool) {
		for e := range fc {
			if e.err != nil {
				yield(either[U]{err: e.err})
				return
			}
			inner, err := f(e.value)
			if err != nil {
				yield(either[U]{err: err})
				return
			}
			for u := range inner {
				if !yield(either[U]{value: u}) {
					return
				}
			}
		}
	}
}

// Take returns a chain over the first n elements. A zero n takes none, and
// a negative n is no limit: the chain then ends only when its source does,
// so an endless source makes it endless too. An error arriving before the
// n-th element is reported and ends the iteration.
// vow:nil () !
func (fc FailableChain[T]) Take(n int) FailableChain[T] {
	return func(yield func(either[T]) bool) {
		if n == 0 {
			return
		}
		rest := n
		for e := range fc {
			if !yield(e) {
				return
			}
			if e.err != nil {
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
func (fc FailableChain[T]) TakeWhile(f func(T) bool) FailableChain[T] {
	return fc.TakeWhileOrErr(func(v T) (bool, error) { return f(v), nil })
}

// TakeWhileOrErr returns a chain over the elements before the first one
// for which f reports false, and stops at the first error f reports. An
// error the chain carries before the element f rejects is reported and
// ends the iteration; one after it is never reached. Where f reports
// false and an error at once, the error is reported.
// vow:nil (!) !
func (fc FailableChain[T]) TakeWhileOrErr(f func(T) (bool, error)) FailableChain[T] {
	return func(yield func(either[T]) bool) {
		for e := range fc {
			if e.err != nil {
				yield(e)
				return
			}
			keep, err := f(e.value)
			if err != nil {
				yield(either[T]{err: err})
				return
			}
			if !keep || !yield(e) {
				return
			}
		}
	}
}

// Skip returns a chain that discards the first n elements. A negative n
// discards none. An error reached while discarding is reported instead of
// being discarded.
// vow:nil () !
func (fc FailableChain[T]) Skip(n int) FailableChain[T] {
	return func(yield func(either[T]) bool) {
		rest := n
		for e := range fc {
			if e.err != nil {
				yield(e)
				return
			}
			if rest > 0 {
				rest--
				continue
			}
			if !yield(e) {
				return
			}
		}
	}
}

// UniqFunc keeps the first element per key f returns.
// vow:nil (!) !
func (fc FailableChain[T]) UniqFunc[K comparable](f func(T) K) FailableChain[T] {
	return fc.UniqFuncOrErr(func(v T) (K, error) { return f(v), nil })
}

// UniqFuncOrErr keeps the first element per key f returns, and stops at
// the first error f reports.
// vow:nil (!) !
func (fc FailableChain[T]) UniqFuncOrErr[K comparable](f func(T) (K, error)) FailableChain[T] {
	return func(yield func(either[T]) bool) {
		seen := make(map[K]struct{})
		for e := range fc {
			if e.err != nil {
				yield(e)
				return
			}
			k, err := f(e.value)
			if err != nil {
				yield(either[T]{err: err})
				return
			}
			if _, duplicated := seen[k]; duplicated {
				continue
			}
			seen[k] = struct{}{}
			if !yield(e) {
				return
			}
		}
	}
}

// ToSeq2 returns the elements paired with a nil error, followed by the
// zero element paired with the error that ended the iteration if there was
// one. No pair ever carries both.
// vow:nil () !
func (fc FailableChain[T]) ToSeq2() iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for e := range fc {
			// Ending on the error here keeps the at-most-one-error property
			// true at this boundary without resting on every upstream method
			// stopping on its own.
			if !yield(e.value, e.err) || e.err != nil {
				return
			}
		}
	}
}

// ToSlice returns the elements as a slice, or a nil slice together with
// the error. A chain that succeeds with no elements gives a nil slice
// rather than an empty one, so the slice alone does not say whether the
// chain failed.
// vow:nil () ?,
func (fc FailableChain[T]) ToSlice() ([]T, error) {
	var collected []T
	for e := range fc {
		if e.err != nil {
			return nil, e.err
		}
		collected = append(collected, e.value)
	}
	return collected, nil
}

// ToMap collects the elements into a map by the key and value f returns
// for each of them, or gives a nil map together with the error. A chain
// that succeeds with no elements gives an empty map rather than a nil one,
// and where f gives the same key for several elements the map keeps the
// value from the last of them.
// vow:nil (!) ?,
func (fc FailableChain[T]) ToMap[K comparable, V any](f func(T) (K, V)) (map[K]V, error) {
	collected := map[K]V{}
	for e := range fc {
		if e.err != nil {
			return nil, e.err
		}
		key, value := f(e.value)
		collected[key] = value
	}
	return collected, nil
}

// ToMapOrErr collects the elements into a map by the key and value f
// returns for each of them, and stops at the first error f reports or the
// chain carries. It returns a nil map together with the error.
// vow:nil (!) ?,
func (fc FailableChain[T]) ToMapOrErr[K comparable, V any](f func(T) (K, V, error)) (map[K]V, error) {
	collected := map[K]V{}
	for e := range fc {
		if e.err != nil {
			return nil, e.err
		}
		key, value, err := f(e.value)
		if err != nil {
			return nil, err
		}
		collected[key] = value
	}
	return collected, nil
}

// ToSet collects the keys f returns for the elements into a set, or gives
// a nil set together with the error. A chain that succeeds with no
// elements gives an empty set rather than a nil one.
// vow:nil (!) ?,
func (fc FailableChain[T]) ToSet[K comparable](f func(T) K) (map[K]struct{}, error) {
	collected := map[K]struct{}{}
	for e := range fc {
		if e.err != nil {
			return nil, e.err
		}
		collected[f(e.value)] = struct{}{}
	}
	return collected, nil
}

// Count returns the number of elements, or zero together with the error
// the chain carries if it reaches one first.
// vow:nil () ,
func (fc FailableChain[T]) Count() (int, error) {
	return fc.Reduce(0, func(n int, _ T) int { return n + 1 })
}

// Any reports whether f returns true for at least one element, stopping at
// the first one that satisfies it. It reports false for an empty chain. The
// walk ends where the answer is settled, so a chain carrying an error
// reports that error only when no earlier element satisfies f.
// vow:nil (!) ,
func (fc FailableChain[T]) Any(f func(T) bool) (bool, error) {
	return fc.AnyOrErr(func(v T) (bool, error) { return f(v), nil })
}

// AnyOrErr reports whether f returns true for at least one element,
// stopping at the first one that satisfies it and at the first error
// reported by the chain or by f, whichever comes first. It reports false
// together with the error.
// vow:nil (!) ,
func (fc FailableChain[T]) AnyOrErr(f func(T) (bool, error)) (bool, error) {
	_, found, err := fc.FindOrErr(f)
	return found, err
}

// All reports whether f returns true for every element, stopping at the
// first one that fails it. It reports true for an empty chain. The walk
// ends where the answer is settled, so a chain carrying an error reports
// that error only when no earlier element fails f.
// vow:nil (!) ,
func (fc FailableChain[T]) All(f func(T) bool) (bool, error) {
	return fc.AllOrErr(func(v T) (bool, error) { return f(v), nil })
}

// AllOrErr reports whether f returns true for every element, stopping at
// the first one that fails it and at the first error reported by the chain
// or by f, whichever comes first. It reports false together with the error.
// vow:nil (!) ,
func (fc FailableChain[T]) AllOrErr(f func(T) (bool, error)) (bool, error) {
	_, failed, err := fc.FindOrErr(func(v T) (bool, error) {
		holds, err := f(v)
		return !holds, err
	})
	if err != nil {
		return false, err
	}
	return !failed, nil
}

// Find returns the first element for which f reports true. The second
// return value is false when no element does, and when the chain carries
// an error.
// vow:nil (!) ,,
func (fc FailableChain[T]) Find(f func(T) bool) (T, bool, error) {
	return fc.FindOrErr(func(v T) (bool, error) { return f(v), nil })
}

// FindOrErr returns the first element for which f reports true. The second
// return value is false when no element does, and when the iteration stops
// at an error reported by the chain or by f.
// vow:nil (!) ,,
func (fc FailableChain[T]) FindOrErr(f func(T) (bool, error)) (T, bool, error) {
	var zero T
	for e := range fc {
		if e.err != nil {
			return zero, false, e.err
		}
		found, err := f(e.value)
		if err != nil {
			return zero, false, err
		}
		if found {
			return e.value, true, nil
		}
	}
	return zero, false, nil
}

// Reduce accumulates every element into the accumulator, starting from
// initial. It returns the zero accumulator together with the error the
// chain carries if it reaches one.
// vow:nil (,!) ,
func (fc FailableChain[T]) Reduce[U any](initial U, f func(U, T) U) (U, error) {
	return fc.ReduceOrErr(initial, func(acc U, v T) (U, error) { return f(acc, v), nil })
}

// ReduceOrErr accumulates every element into the accumulator, starting
// from initial, and stops at the first error reported by the chain or by
// f. It returns the zero accumulator together with the error.
// vow:nil (,!) ,
func (fc FailableChain[T]) ReduceOrErr[U any](initial U, f func(U, T) (U, error)) (U, error) {
	acc := initial
	for e := range fc {
		if e.err != nil {
			var zero U
			return zero, e.err
		}
		next, err := f(acc, e.value)
		if err != nil {
			var zero U
			return zero, err
		}
		acc = next
	}
	return acc, nil
}

// Each calls f for every element, and returns the error the chain carries
// if it reaches one.
// vow:nil (!)
func (fc FailableChain[T]) Each(f func(T)) error {
	return fc.EachOrErr(func(v T) error {
		f(v)
		return nil
	})
}

// EachOrErr calls f for every element, and stops at the first error
// reported by the chain or by f.
// vow:nil (!)
func (fc FailableChain[T]) EachOrErr(f func(T) error) error {
	for e := range fc {
		if e.err != nil {
			return e.err
		}
		if err := f(e.value); err != nil {
			return err
		}
	}
	return nil
}

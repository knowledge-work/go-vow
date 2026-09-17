package seq

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

var (
	errBoom  = errors.New("boom")
	errLater = errors.New("later")
)

// pulledFailableChain returns a FailableChain that yields the given
// outcomes verbatim, together with a pointer to the number it has yielded
// so far. The outcomes bypass the exported constructors so that a chain
// carrying more than one error, which NewFailableChain never produces, can
// still be fed to a downstream method.
func pulledFailableChain(outcomes ...either[int]) (FailableChain[int], *int) {
	pulled := 0
	return func(yield func(either[int]) bool) {
		for _, e := range outcomes {
			pulled++
			if !yield(e) {
				return
			}
		}
	}, &pulled
}

// value and fault build the two outcome shapes the source can hold.
func value(v int) either[int]     { return either[int]{value: v} }
func fault(err error) either[int] { return either[int]{err: err} }
func values(vs ...int) []either[int] {
	outcomes := make([]either[int], 0, len(vs))
	for _, v := range vs {
		outcomes = append(outcomes, value(v))
	}
	return outcomes
}

// collect drains a chain into its value and error halves, keeping the
// outcomes in the order they arrived. It reads the chain directly rather
// than through ToSeq2, whose own stop-at-the-error guard would otherwise
// stand in for the behaviour of the method under test.
func collect(fc FailableChain[int]) (vs []int, errs []error) {
	for e := range fc {
		vs = append(vs, e.value)
		errs = append(errs, e.err)
	}
	return vs, errs
}

// TestFailableChain_stopsAtFirstError pins the invariant across the
// element-wise methods: the outcome after the error is never pulled from
// the source, and the error is the last thing the chain reports.
func TestFailableChain_stopsAtFirstError(t *testing.T) {
	type stopsCase struct {
		op func(FailableChain[int]) FailableChain[int]
	}
	tabletest.Run(t, map[string]stopsCase{
		"Filter": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.Filter(func(int) bool { return true })
		}},
		"FilterOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.FilterOrErr(func(int) (bool, error) { return true, nil })
		}},
		"Map": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.Map(func(v int) int { return v })
		}},
		"MapOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.MapOrErr(func(v int) (int, error) { return v, nil })
		}},
		"FilterMap": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.FilterMap(func(v int) (int, bool) { return v, true })
		}},
		"FilterMapOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.FilterMapOrErr(func(v int) (int, bool, error) { return v, true, nil })
		}},
		"FlatMap": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.FlatMap(func(v int) iter.Seq[int] { return slices.Values([]int{v}) })
		}},
		"FlatMapOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.FlatMapOrErr(func(v int) (iter.Seq[int], error) {
				return slices.Values([]int{v}), nil
			})
		}},
		"UniqFunc": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.UniqFunc(func(v int) int { return v })
		}},
		"UniqFuncOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.UniqFuncOrErr(func(v int) (int, error) { return v, nil })
		}},
		"Take past the error": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.Take(3)
		}},
		"TakeWhile": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.TakeWhile(func(int) bool { return true })
		}},
		"TakeWhileOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.TakeWhileOrErr(func(int) (bool, error) { return true, nil })
		}},
		"Skip nothing": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.Skip(0)
		}},
	}, func(t *testing.T, c stopsCase) {
		src, pulled := pulledFailableChain(value(1), fault(errBoom), value(3))
		vs, errs := collect(c.op(src))
		assert.Equal(t, "outcomes pulled from the source; the third is past the error", *pulled, 2)
		assert.DeepEqual(t, "values; the failed element carries the zero value", vs, []int{1, 0})
		assert.MustLen(t, "errors", errs, 2)
		assert.NoError(t, "errors[0]", errs[0])
		assert.ErrorIs(t, "errors[1]", errs[1], errBoom)
	})
}

// TestFailableChain_reportsAtMostOneError feeds a source holding two
// errors. The chain reports the first and stops, so the second never
// reaches the consumer.
func TestFailableChain_reportsAtMostOneError(t *testing.T) {
	src, pulled := pulledFailableChain(fault(errBoom), fault(errLater))
	vs, errs := collect(src.Map(func(v int) int { return v }))
	assert.Equal(t, "outcomes pulled from the source", *pulled, 1)
	assert.DeepEqual(t, "values", vs, []int{0})
	assert.MustLen(t, "errors", errs, 1)
	assert.ErrorIs(t, "errors[0]", errs[0], errBoom)
}

// TestFailableChain_skipsFailedElement pins the invariant that a method
// taking a function that cannot fail never applies it to a failed element.
// Each case records the elements its function saw.
func TestFailableChain_skipsFailedElement(t *testing.T) {
	type skipsCase struct {
		run func(fc FailableChain[int], seen *[]int)
	}
	tabletest.Run(t, map[string]skipsCase{
		"Filter": {func(fc FailableChain[int], seen *[]int) {
			collect(fc.Filter(func(v int) bool { *seen = append(*seen, v); return true }))
		}},
		"Map": {func(fc FailableChain[int], seen *[]int) {
			collect(fc.Map(func(v int) int { *seen = append(*seen, v); return v }))
		}},
		"FilterMap": {func(fc FailableChain[int], seen *[]int) {
			collect(fc.FilterMap(func(v int) (int, bool) { *seen = append(*seen, v); return v, true }))
		}},
		"FlatMap": {func(fc FailableChain[int], seen *[]int) {
			collect(fc.FlatMap(func(v int) iter.Seq[int] {
				*seen = append(*seen, v)
				return slices.Values([]int{v})
			}))
		}},
		"TakeWhile": {func(fc FailableChain[int], seen *[]int) {
			collect(fc.TakeWhile(func(v int) bool { *seen = append(*seen, v); return true }))
		}},
		"UniqFunc": {func(fc FailableChain[int], seen *[]int) {
			collect(fc.UniqFunc(func(v int) int { *seen = append(*seen, v); return v }))
		}},
		"Each": {func(fc FailableChain[int], seen *[]int) {
			_ = fc.Each(func(v int) { *seen = append(*seen, v) })
		}},
		"Reduce": {func(fc FailableChain[int], seen *[]int) {
			_, _ = fc.Reduce(0, func(acc, v int) int { *seen = append(*seen, v); return acc })
		}},
		"Find": {func(fc FailableChain[int], seen *[]int) {
			_, _, _ = fc.Find(func(v int) bool { *seen = append(*seen, v); return false })
		}},
	}, func(t *testing.T, c skipsCase) {
		src, _ := pulledFailableChain(value(1), fault(errBoom), value(3))
		var seen []int
		c.run(src, &seen)
		assert.DeepEqual(t, "the elements the function was applied to", seen, []int{1})
	})
}

// TestFailableChain_flatMapTakesAnySequence is the failable counterpart of
// the Chain case: FlatMap and FlatMapOrErr take a function returning a Chain
// too. Most of what it pins is that this compiles.
func TestFailableChain_flatMapTakesAnySequence(t *testing.T) {
	src := func() FailableChain[int] {
		return ChainOf(1, 2).FilterOrErr(func(int) (bool, error) { return true, nil })
	}
	want := []int{1, -1, 2, -2}
	t.Run("FlatMap over a Chain", func(t *testing.T) {
		got, err := src().FlatMap(func(v int) Chain[int] { return ChainOf(v, -v) }).ToSlice()
		assert.NoError(t, "FlatMap()", err)
		assert.DeepEqual(t, "FlatMap()", got, want)
	})
	t.Run("FlatMap over an iter.Seq", func(t *testing.T) {
		got, err := src().FlatMap(func(v int) iter.Seq[int] { return slices.Values([]int{v, -v}) }).ToSlice()
		assert.NoError(t, "FlatMap()", err)
		assert.DeepEqual(t, "FlatMap()", got, want)
	})
	t.Run("FlatMapOrErr over a Chain", func(t *testing.T) {
		got, err := src().FlatMapOrErr(func(v int) (Chain[int], error) { return ChainOf(v, -v), nil }).ToSlice()
		assert.NoError(t, "FlatMapOrErr()", err)
		assert.DeepEqual(t, "FlatMapOrErr()", got, want)
	})
	t.Run("FlatMapOrErr over an iter.Seq", func(t *testing.T) {
		got, err := src().FlatMapOrErr(func(v int) (iter.Seq[int], error) {
			return slices.Values([]int{v, -v}), nil
		}).ToSlice()
		assert.NoError(t, "FlatMapOrErr()", err)
		assert.DeepEqual(t, "FlatMapOrErr()", got, want)
	})
}

// TestFailableChain_earlyReturnStopsTheSource pins the path where the
// consumer stops before the source is exhausted: yield returns false and
// each method must return instead of pulling the next outcome.
func TestFailableChain_earlyReturnStopsTheSource(t *testing.T) {
	type earlyReturnCase struct {
		op         func(FailableChain[int]) FailableChain[int]
		wantPulled int
	}
	tabletest.Run(t, map[string]earlyReturnCase{
		"Filter keeping every element": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.Filter(func(int) bool { return true })
		}, 1},
		"Filter dropping the first two": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.Filter(func(v int) bool { return v > 2 })
		}, 3},
		"Map": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.Map(func(v int) int { return v })
		}, 1},
		"FilterMap": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.FilterMap(func(v int) (int, bool) { return v, true })
		}, 1},
		"FilterMapOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.FilterMapOrErr(func(v int) (int, bool, error) { return v, true, nil })
		}, 1},
		"FlatMap": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.FlatMap(func(v int) iter.Seq[int] { return slices.Values([]int{v, v}) })
		}, 1},
		"FlatMapOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.FlatMapOrErr(func(v int) (iter.Seq[int], error) {
				return slices.Values([]int{v, v}), nil
			})
		}, 1},
		"Take":                   {func(fc FailableChain[int]) FailableChain[int] { return fc.Take(4) }, 1},
		"Take with a negative n": {func(fc FailableChain[int]) FailableChain[int] { return fc.Take(-1) }, 1},
		"TakeWhile": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.TakeWhile(func(int) bool { return true })
		}, 1},
		"TakeWhileOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.TakeWhileOrErr(func(int) (bool, error) { return true, nil })
		}, 1},
		"Skip":                   {func(fc FailableChain[int]) FailableChain[int] { return fc.Skip(2) }, 3},
		"Skip with a negative n": {func(fc FailableChain[int]) FailableChain[int] { return fc.Skip(-1) }, 1},
		"UniqFunc": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.UniqFunc(func(v int) int { return v })
		}, 1},
		"UniqFuncOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.UniqFuncOrErr(func(v int) (int, error) { return v, nil })
		}, 1},
	}, func(t *testing.T, c earlyReturnCase) {
		src, pulled := pulledFailableChain(values(1, 2, 3, 4, 5)...)
		for range c.op(src) {
			break
		}
		assert.Equal(t, "outcomes pulled from the source", *pulled, c.wantPulled)
	})
}

// TestFailableChain_terminalsStopAtTheirAnswer pins how far each terminal
// reads: the ones that settle on an element stop there, and the ones that
// build a collection read to the end.
func TestFailableChain_terminalsStopAtTheirAnswer(t *testing.T) {
	type terminalCase struct {
		run        func(FailableChain[int])
		wantPulled int
	}
	tabletest.Run(t, map[string]terminalCase{
		"Find matching the second element": {func(fc FailableChain[int]) {
			_, _, _ = fc.Find(func(v int) bool { return v == 2 })
		}, 2},
		"FindOrErr failing on the second element": {func(fc FailableChain[int]) {
			_, _, _ = fc.FindOrErr(func(v int) (bool, error) { return false, map[bool]error{true: errBoom, false: nil}[v == 2] })
		}, 2},
		"EachOrErr failing on the third element": {func(fc FailableChain[int]) {
			_ = fc.EachOrErr(func(v int) error {
				if v == 3 {
					return errBoom
				}
				return nil
			})
		}, 3},
		"ToSlice draining everything": {func(fc FailableChain[int]) {
			_, _ = fc.ToSlice()
		}, 5},
		"ToMap draining everything": {func(fc FailableChain[int]) {
			_, _ = fc.ToMap(func(v int) (int, int) { return v, v })
		}, 5},
		"ToSet draining everything": {func(fc FailableChain[int]) {
			_, _ = fc.ToSet(func(v int) int { return v })
		}, 5},
	}, func(t *testing.T, c terminalCase) {
		src, pulled := pulledFailableChain(values(1, 2, 3, 4, 5)...)
		c.run(src)
		assert.Equal(t, "outcomes pulled from the source", *pulled, c.wantPulled)
	})
}

// TestNewFailableChain_cutsTheSourceShort pins that the constructor ends
// the iteration at the first non-nil error even when the iter.Seq2 behind
// it keeps going.
func TestNewFailableChain_cutsTheSourceShort(t *testing.T) {
	yielded := 0
	source := func(yield func(int, error) bool) {
		for _, pair := range []struct {
			v   int
			err error
		}{{1, nil}, {0, errBoom}, {3, nil}} {
			yielded++
			if !yield(pair.v, pair.err) {
				return
			}
		}
	}
	got, err := NewFailableChain(iter.Seq2[int, error](source)).ToSlice()
	assert.ErrorIs(t, "ToSlice()", err, errBoom)
	assert.Nil(t, "slice alongside the error", got)
	assert.Equal(t, "pairs the source yielded", yielded, 2)
}

// TestNewFailableChain_earlyReturnStopsTheSource pins the other way the
// constructor's loop can end: the consumer stops before the source is
// exhausted, so yield returns false and no further pair is drawn.
func TestNewFailableChain_earlyReturnStopsTheSource(t *testing.T) {
	yielded := 0
	source := func(yield func(int, error) bool) {
		for _, v := range []int{1, 2, 3} {
			yielded++
			if !yield(v, nil) {
				return
			}
		}
	}
	for range NewFailableChain(iter.Seq2[int, error](source)) {
		break
	}
	assert.Equal(t, "pairs the source yielded", yielded, 1)
}

// TestFailableChain_Take pins the element count, what a negative n takes,
// and the error that lands before the n-th element.
func TestFailableChain_Take(t *testing.T) {
	type takeCase struct {
		outcomes  []either[int]
		n         int
		wantValue []int
		wantErr   error
	}
	tabletest.Run(t, map[string]takeCase{
		"fewer than n":                        {values(1, 2), 5, []int{1, 2}, nil},
		"exactly n":                           {values(1, 2, 3), 3, []int{1, 2, 3}, nil},
		"more than n":                         {values(1, 2, 3), 2, []int{1, 2}, nil},
		"none":                                {values(1, 2, 3), 0, nil, nil},
		"negative n takes everything":         {values(1, 2, 3), -1, []int{1, 2, 3}, nil},
		"negative n still stops at the error": {[]either[int]{value(1), fault(errBoom)}, -1, nil, errBoom},
		"error before the n-th":               {[]either[int]{value(1), fault(errBoom), value(3)}, 3, nil, errBoom},
		"error after the n-th":                {[]either[int]{value(1), value(2), fault(errBoom)}, 2, []int{1, 2}, nil},
	}, func(t *testing.T, c takeCase) {
		src, _ := pulledFailableChain(c.outcomes...)
		got, err := src.Take(c.n).ToSlice()
		assert.MustErrorIs(t, "Take().ToSlice()", err, c.wantErr)
		assert.DeepEqual(t, "Take().ToSlice()", got, c.wantValue)
	})
}

// TestFailableChain_TakeWhile pins where the chain ends when f reports
// false, and which errors it reaches before ending there.
func TestFailableChain_TakeWhile(t *testing.T) {
	type takeWhileCase struct {
		outcomes  []either[int]
		wantValue []int
		wantErr   error
	}
	tabletest.Run(t, map[string]takeWhileCase{
		"every element holds":              {values(1, 2), []int{1, 2}, nil},
		"stops rather than skipping":       {values(1, 2, 3, 1), []int{1, 2}, nil},
		"the first element fails":          {values(3, 1), nil, nil},
		"error before the predicate fails": {[]either[int]{value(1), fault(errBoom), value(2)}, nil, errBoom},
		"error after the predicate fails":  {[]either[int]{value(1), value(3), fault(errBoom)}, []int{1}, nil},
	}, func(t *testing.T, c takeWhileCase) {
		src, _ := pulledFailableChain(c.outcomes...)
		got, err := src.TakeWhile(func(v int) bool { return v < 3 }).ToSlice()
		assert.MustErrorIs(t, "TakeWhile().ToSlice()", err, c.wantErr)
		assert.DeepEqual(t, "TakeWhile().ToSlice()", got, c.wantValue)
	})
}

// TestFailableChain_TakeWhileOrErr pins the answer TakeWhile cannot reach:
// the error f reports at the very element it rejects.
func TestFailableChain_TakeWhileOrErr(t *testing.T) {
	src, _ := pulledFailableChain(values(1, 2, 3)...)
	got, err := src.TakeWhileOrErr(func(v int) (bool, error) { return v != 2, failOn(v, 2) }).ToSlice()
	assert.MustErrorIs(t, "TakeWhileOrErr().ToSlice()", err, errBoom)
	assert.Nil(t, "slice alongside the error", got)
}

// TestFailableChain_Skip pins the discarded count, what a negative n
// discards, and the error that lands while elements are still being
// discarded.
func TestFailableChain_Skip(t *testing.T) {
	type skipCase struct {
		outcomes  []either[int]
		n         int
		wantValue []int
		wantErr   error
	}
	tabletest.Run(t, map[string]skipCase{
		"fewer than n":                       {values(1, 2), 5, nil, nil},
		"exactly n":                          {values(1, 2, 3), 3, nil, nil},
		"more than n":                        {values(1, 2, 3), 1, []int{2, 3}, nil},
		"none":                               {values(1, 2), 0, []int{1, 2}, nil},
		"negative n discards nothing":        {values(1, 2), -1, []int{1, 2}, nil},
		"negative n still reports the error": {[]either[int]{value(1), fault(errBoom)}, -1, nil, errBoom},
		"error while discarding":             {[]either[int]{value(1), fault(errBoom), value(3)}, 2, nil, errBoom},
		"error after discarding":             {[]either[int]{value(1), value(2), fault(errBoom)}, 1, nil, errBoom},
	}, func(t *testing.T, c skipCase) {
		src, _ := pulledFailableChain(c.outcomes...)
		got, err := src.Skip(c.n).ToSlice()
		assert.MustErrorIs(t, "Skip().ToSlice()", err, c.wantErr)
		assert.DeepEqual(t, "Skip().ToSlice()", got, c.wantValue)
	})
}

// TestFailableChain_UniqFuncOrErr pins that only the first element per key
// survives, and that an error the key function reports ends the chain.
func TestFailableChain_UniqFuncOrErr(t *testing.T) {
	src, _ := pulledFailableChain(values(1, 2, 1, 3, 2)...)
	got, err := src.UniqFunc(func(v int) int { return v }).ToSlice()
	assert.MustNoError(t, "UniqFunc().ToSlice()", err)
	assert.DeepEqual(t, "UniqFunc().ToSlice()", got, []int{1, 2, 3})

	failing, pulled := pulledFailableChain(values(1, 2, 3)...)
	_, err = failing.UniqFuncOrErr(func(v int) (int, error) {
		if v == 2 {
			return 0, errBoom
		}
		return v, nil
	}).ToSlice()
	assert.ErrorIs(t, "UniqFuncOrErr().ToSlice()", err, errBoom)
	assert.Equal(t, "outcomes pulled from the source", *pulled, 2)
}

// TestFailableChain_terminals pins the answers the terminals give on a
// chain that succeeds and on one that carries an error.
func TestFailableChain_terminals(t *testing.T) {
	t.Run("Count", func(t *testing.T) {
		src, _ := pulledFailableChain(values(1, 2, 3)...)
		got, err := src.Count()
		assert.NoError(t, "Count()", err)
		assert.Equal(t, "Count()", got, 3)
	})
	t.Run("Count with an error", func(t *testing.T) {
		src, _ := pulledFailableChain(value(1), fault(errBoom))
		got, err := src.Count()
		assert.ErrorIs(t, "Count()", err, errBoom)
		assert.Equal(t, "Count() alongside the error", got, 0)
	})
	t.Run("ToSlice of nothing is nil rather than empty", func(t *testing.T) {
		// The Take and Skip tables also fail when this returns an empty
		// slice, but only because the want they were written for is nil.
		src, _ := pulledFailableChain()
		got, err := src.ToSlice()
		assert.NoError(t, "ToSlice()", err)
		assert.Nil(t, "ToSlice()", got)
	})
	t.Run("ToMap", func(t *testing.T) {
		src, _ := pulledFailableChain(values(1, 2, 3)...)
		got, err := src.ToMap(func(v int) (int, int) { return v, v * 10 })
		assert.NoError(t, "ToMap()", err)
		assert.DeepEqual(t, "ToMap()", got, map[int]int{1: 10, 2: 20, 3: 30})
	})
	t.Run("ToMap of nothing is empty rather than nil", func(t *testing.T) {
		src, _ := pulledFailableChain()
		got, err := src.ToMap(func(v int) (int, int) { return v, v })
		assert.NoError(t, "ToMap()", err)
		assert.DeepEqual(t, "ToMap()", got, map[int]int{})
	})
	t.Run("ToMapOrErr stops at the first error f reports", func(t *testing.T) {
		src, pulled := pulledFailableChain(values(1, 2, 3, 4, 5)...)
		got, err := src.ToMapOrErr(func(v int) (int, int, error) { return v, v, failOn(v, 3) })
		assert.ErrorIs(t, "ToMapOrErr()", err, errBoom)
		assert.Nil(t, "ToMapOrErr() map alongside the error", got)
		assert.Equal(t, "outcomes pulled from the source", *pulled, 3)
	})
	t.Run("ToMapOrErr reports the error the chain carries", func(t *testing.T) {
		src, _ := pulledFailableChain(value(1), fault(errBoom))
		got, err := src.ToMapOrErr(func(v int) (int, int, error) { return v, v, nil })
		assert.ErrorIs(t, "ToMapOrErr()", err, errBoom)
		assert.Nil(t, "ToMapOrErr() map alongside the error", got)
	})
	t.Run("ToMap with an error", func(t *testing.T) {
		src, _ := pulledFailableChain(value(1), fault(errBoom))
		got, err := src.ToMap(func(v int) (int, int) { return v, v })
		assert.ErrorIs(t, "ToMap()", err, errBoom)
		assert.Nil(t, "ToMap() map alongside the error", got)
	})
	t.Run("ToMap keeps the value from the last element on a duplicate key", func(t *testing.T) {
		src, _ := pulledFailableChain(values(1, 2, 3, 4, 5)...)
		got, err := src.ToMap(func(v int) (int, int) { return v % 2, v })
		assert.NoError(t, "ToMap()", err)
		assert.DeepEqual(t, "ToMap()", got, map[int]int{0: 4, 1: 5})
	})
	t.Run("ToSet", func(t *testing.T) {
		src, _ := pulledFailableChain(values(1, 2, 3)...)
		got, err := src.ToSet(func(v int) int { return v * 10 })
		assert.NoError(t, "ToSet()", err)
		assert.DeepEqual(t, "ToSet()", got, map[int]struct{}{10: {}, 20: {}, 30: {}})
	})
	t.Run("ToSet of nothing is empty rather than nil", func(t *testing.T) {
		src, _ := pulledFailableChain()
		got, err := src.ToSet(func(v int) int { return v })
		assert.NoError(t, "ToSet()", err)
		assert.DeepEqual(t, "ToSet()", got, map[int]struct{}{})
	})
	t.Run("ToSet with an error", func(t *testing.T) {
		src, _ := pulledFailableChain(value(1), fault(errBoom))
		got, err := src.ToSet(func(v int) int { return v })
		assert.ErrorIs(t, "ToSet()", err, errBoom)
		assert.Nil(t, "ToSet() set alongside the error", got)
	})
	t.Run("ToSet folds the elements giving the same key into one member", func(t *testing.T) {
		src, _ := pulledFailableChain(values(1, 2, 3, 4, 5)...)
		got, err := src.ToSet(func(v int) int { return v % 2 })
		assert.NoError(t, "ToSet()", err)
		assert.DeepEqual(t, "ToSet()", got, map[int]struct{}{0: {}, 1: {}})
	})
	t.Run("Find with no match", func(t *testing.T) {
		src, _ := pulledFailableChain(values(1, 2)...)
		got, found, err := src.Find(func(int) bool { return false })
		assert.NoError(t, "Find()", err)
		assert.Equal(t, "Find() element", got, 0)
		assert.Equal(t, "Find() found", found, false)
	})
	t.Run("Find with a match", func(t *testing.T) {
		src, _ := pulledFailableChain(values(1, 2)...)
		got, found, err := src.Find(func(v int) bool { return v == 2 })
		assert.NoError(t, "Find()", err)
		assert.Equal(t, "Find() element", got, 2)
		assert.Equal(t, "Find() found", found, true)
	})
	t.Run("Find with an error", func(t *testing.T) {
		src, _ := pulledFailableChain(value(1), fault(errBoom))
		got, found, err := src.Find(func(int) bool { return false })
		assert.ErrorIs(t, "Find()", err, errBoom)
		assert.Equal(t, "Find() element", got, 0)
		assert.Equal(t, "Find() found", found, false)
	})
	t.Run("Reduce folds onto the initial accumulator", func(t *testing.T) {
		src, _ := pulledFailableChain(values(1, 2, 3)...)
		got, err := src.Reduce(100, func(acc, v int) int { return acc + v })
		assert.NoError(t, "Reduce()", err)
		assert.Equal(t, "Reduce()", got, 106)
	})
	t.Run("ReduceOrErr returns the zero accumulator with the error", func(t *testing.T) {
		src, _ := pulledFailableChain(values(1, 2, 3)...)
		got, err := src.ReduceOrErr(100, func(acc, v int) (int, error) {
			if v == 3 {
				return 0, errBoom
			}
			return acc + v, nil
		})
		assert.ErrorIs(t, "ReduceOrErr()", err, errBoom)
		assert.Equal(t, "ReduceOrErr() accumulator alongside the error", got, 0)
	})
	t.Run("Each reports the chain's error", func(t *testing.T) {
		src, _ := pulledFailableChain(value(1), fault(errBoom))
		assert.ErrorIs(t, "Each()", src.Each(func(int) {}), errBoom)
	})
	t.Run("ToSeq2 stops when the consumer does", func(t *testing.T) {
		src, pulled := pulledFailableChain(values(1, 2, 3)...)
		for range src.ToSeq2() {
			break
		}
		assert.Equal(t, "outcomes pulled from the source", *pulled, 1)
	})
}

// TestFailableChain_ToSeq2StopsAtTheFirstError pins the at-most-one-error
// property at the package boundary rather than at the methods upstream of
// it. The source hands over two errors, which no exported constructor
// produces, so only ToSeq2 itself can cut the second one off.
func TestFailableChain_ToSeq2StopsAtTheFirstError(t *testing.T) {
	src, pulled := pulledFailableChain(fault(errBoom), fault(errLater))
	var errs []error
	for _, err := range src.ToSeq2() {
		errs = append(errs, err)
	}
	assert.MustLen(t, "errors", errs, 1)
	assert.ErrorIs(t, "errors[0]", errs[0], errBoom)
	assert.Equal(t, "outcomes pulled from the source", *pulled, 1)
}

// TestFlatMapOrErr_stopsExpandingWhenTheConsumerDoes pins the inner loop:
// the consumer can stop part-way through one element's expansion, and f
// must not be called for the element after it.
func TestFlatMapOrErr_stopsExpandingWhenTheConsumerDoes(t *testing.T) {
	src, _ := pulledFailableChain(values(1, 2, 3)...)
	applied := 0
	expanded := src.FlatMapOrErr(func(v int) (iter.Seq[int], error) {
		applied++
		return slices.Values([]int{v, v * 10, v * 100}), nil
	})
	for range expanded {
		break
	}
	assert.Equal(t, "times f was applied; the consumer stopped inside the first expansion", applied, 1)
}

// TestFailableChain_replaysWithItsSource is the failable counterpart of the
// Chain replay case: the methods that carry state across elements hold it
// inside the sequence they return, so a replayable source replays.
func TestFailableChain_replaysWithItsSource(t *testing.T) {
	type replayCase struct {
		op   func(FailableChain[int]) FailableChain[int]
		want []int
	}
	tabletest.Run(t, map[string]replayCase{
		"Take": {func(fc FailableChain[int]) FailableChain[int] { return fc.Take(2) }, []int{1, 2}},
		"Skip": {func(fc FailableChain[int]) FailableChain[int] { return fc.Skip(3) }, []int{4, 5}},
		"UniqFunc": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.UniqFunc(func(v int) int { return v % 3 })
		}, []int{1, 2, 3}},
		"UniqFuncOrErr": {func(fc FailableChain[int]) FailableChain[int] {
			return fc.UniqFuncOrErr(func(v int) (int, error) { return v % 3, nil })
		}, []int{1, 2, 3}},
	}, func(t *testing.T, c replayCase) {
		chain := c.op(NewFailableChain(func(yield func(int, error) bool) {
			for _, v := range []int{1, 2, 3, 4, 5} {
				if !yield(v, nil) {
					return
				}
			}
		}))
		for round := range 2 {
			got, err := chain.ToSlice()
			assert.MustNoError(t, fmt.Sprintf("round %d", round+1), err)
			assert.DeepEqual(t, fmt.Sprintf("round %d", round+1), got, c.want)
		}
	})
}

// TestFailableChain_anyAll pins the two predicates on the failable side.
// The error the chain carries only surfaces when the predicate has not
// already settled the answer, so the last two cases are a pair: the same
// chain reports the error through one predicate and not the other.
func TestFailableChain_anyAll(t *testing.T) {
	carriesError := []either[int]{value(2), fault(errBoom)}
	type anyAllCase struct {
		outcomes   []either[int]
		f          func(int) bool
		wantAny    bool
		wantAnyErr error
		wantAll    bool
		wantAllErr error
	}
	tabletest.Run(t, map[string]anyAllCase{
		"empty":                 {nil, func(int) bool { return true }, false, nil, true, nil},
		"a match":               {values(1, 2, 3), func(v int) bool { return v == 2 }, true, nil, false, nil},
		"no match":              {values(1, 1), func(v int) bool { return v == 2 }, false, nil, false, nil},
		"every element matches": {values(2, 2), func(v int) bool { return v == 2 }, true, nil, true, nil},
		"error reached, because nothing matches": {carriesError,
			func(v int) bool { return v == 9 }, false, errBoom, false, nil},
		"error reached, because everything matches": {carriesError,
			func(v int) bool { return v == 2 }, true, nil, false, errBoom},
	}, func(t *testing.T, c anyAllCase) {
		src, _ := pulledFailableChain(c.outcomes...)
		gotAny, err := src.Any(c.f)
		assert.ErrorIs(t, "Any()", err, c.wantAnyErr)
		assert.Equal(t, "Any()", gotAny, c.wantAny)
		src, _ = pulledFailableChain(c.outcomes...)
		gotAll, err := src.All(c.f)
		assert.ErrorIs(t, "All()", err, c.wantAllErr)
		assert.Equal(t, "All()", gotAll, c.wantAll)
	})
}

// TestFailableChain_anyAllStopEarly pins that neither predicate keeps
// pulling once its answer is settled.
func TestFailableChain_anyAllStopEarly(t *testing.T) {
	t.Run("Any", func(t *testing.T) {
		src, pulled := pulledFailableChain(values(1, 2, 3, 4, 5)...)
		got, err := src.Any(func(v int) bool { return v == 2 })
		assert.MustNoError(t, "Any()", err)
		assert.MustEqual(t, "Any()", got, true)
		assert.Equal(t, "outcomes pulled from the source", *pulled, 2)
	})
	t.Run("AllOrErr stops at the predicate's error", func(t *testing.T) {
		src, pulled := pulledFailableChain(values(1, 2, 3, 4, 5)...)
		got, err := src.AllOrErr(func(v int) (bool, error) {
			if v == 3 {
				return true, errBoom
			}
			return true, nil
		})
		assert.ErrorIs(t, "AllOrErr()", err, errBoom)
		assert.Equal(t, "AllOrErr() answer alongside the error", got, false)
		assert.Equal(t, "outcomes pulled from the source", *pulled, 3)
	})
}

// TestFailableChain_terminalsAllocateLikeALoop measures the collecting
// terminals the way TestChain_toMapAllocatesLikeALoop measures ToMap. The
// source is lifted from a Chain by the unexported failable, and that is a
// condition of the comparison rather than a shorthand. NewFailableChain and
// the four OrErr methods on Chain each add a layer that leaves ToSlice three
// allocations above its loop, so the ToSlice row fails through any of them;
// the pulled helper the other tests use counts what it has yielded, and the
// count lands on the chained side alone.
func TestFailableChain_terminalsAllocateLikeALoop(t *testing.T) {
	type allocCase struct {
		chained func()
		looped  func()
	}
	src := []int{1, 2, 3, 4, 5, 6, 7, 8}
	pair := func(v int) (int, int) { return v, v * 10 }
	triple := func(v int) (int, int, error) { return v, v * 10, nil }
	key := func(v int) int { return v * 10 }
	tabletest.Run(t, map[string]allocCase{
		"ToSet": {
			chained: func() { _, _ = ChainOf(src...).failable().ToSet(key) },
			looped: func() {
				collected := map[int]struct{}{}
				for _, v := range src {
					collected[key(v)] = struct{}{}
				}
				_ = collected
			},
		},
		"ToSlice": {
			chained: func() { _, _ = ChainOf(src...).failable().ToSlice() },
			looped: func() {
				var collected []int
				for _, v := range src {
					collected = append(collected, v)
				}
				_ = collected
			},
		},
		"ToMap": {
			chained: func() { _, _ = ChainOf(src...).failable().ToMap(pair) },
			looped: func() {
				collected := map[int]int{}
				for _, v := range src {
					k, value := pair(v)
					collected[k] = value
				}
				_ = collected
			},
		},
		"ToMapOrErr": {
			chained: func() { _, _ = ChainOf(src...).failable().ToMapOrErr(triple) },
			looped: func() {
				collected := map[int]int{}
				for _, v := range src {
					k, value, _ := triple(v)
					collected[k] = value
				}
				_ = collected
			},
		},
	}, func(t *testing.T, testCase allocCase) {
		assert.Equal(t, "the allocations the terminal made, against a plain loop over the same input",
			testing.AllocsPerRun(100, testCase.chained), testing.AllocsPerRun(100, testCase.looped))
	})
}

// TestFailableChain_terminalsDoNotAllocate pins the inlining these four
// depend on, the way TestChain_terminalsDoNotAllocate does for the chain
// that cannot fail. The collecting terminals are held against a plain loop
// instead, since what they allocate is the collection they return.
func TestFailableChain_terminalsDoNotAllocate(t *testing.T) {
	src := []int{1, 2, 3, 4, 5, 6, 7, 8}
	tabletest.Run(t, map[string]func(){
		"ToSeq2": func() {
			for range ChainOf(src...).failable().ToSeq2() {
			}
		},
		"FindOrErr": func() {
			_, _, _ = ChainOf(src...).failable().FindOrErr(func(v int) (bool, error) { return v == 8, nil })
		},
		"ReduceOrErr": func() {
			_, _ = ChainOf(src...).failable().ReduceOrErr(0, func(acc, v int) (int, error) { return acc + v, nil })
		},
		"EachOrErr": func() {
			_ = ChainOf(src...).failable().EachOrErr(func(int) error { return nil })
		},
	}, func(t *testing.T, call func()) {
		assert.Equal(t, "allocations the terminal made", testing.AllocsPerRun(100, call), float64(0))
	})
}

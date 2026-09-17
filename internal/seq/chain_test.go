package seq

import (
	"fmt"
	"iter"
	"slices"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// pulledChain returns a Chain over the given values, together with a
// pointer to the number it has yielded so far.
func pulledChain(vs ...int) (Chain[int], *int) {
	pulled := 0
	return func(yield func(int) bool) {
		for _, v := range vs {
			pulled++
			if !yield(v) {
				return
			}
		}
	}, &pulled
}

// ids is a named slice type, so the spread case below pins both spellings
// ChainOf has to accept from a caller holding a slice.
type ids []int

// TestChain_constructors pins that the entry points produce the same chain,
// and that ToSeq hands back something an iter.Seq consumer accepts. The
// spread case is the form a caller reaches for when the elements are
// already in a slice; what it pins is that the call compiles, since the
// variadic packing happens before ChainOf sees anything.
func TestChain_constructors(t *testing.T) {
	want := []int{1, 2, 3}
	type constructorCase struct {
		chain Chain[int]
	}
	tabletest.Run(t, map[string]constructorCase{
		"NewChain":       {NewChain(slices.Values(want))},
		"ChainOf":        {ChainOf(1, 2, 3)},
		"ChainOf spread": {ChainOf(ids{1, 2, 3}...)},
		"conversion":     {Chain[int](slices.Values(want))},
	}, func(t *testing.T, c constructorCase) {
		assert.DeepEqual(t, "elements", slices.Collect(c.chain.ToSeq()), want)
	})
}

// TestChain_elementWise pins the answers of the methods that keep the
// chain infallible.
func TestChain_elementWise(t *testing.T) {
	type elementWiseCase struct {
		op   func(Chain[int]) Chain[int]
		want []int
	}
	tabletest.Run(t, map[string]elementWiseCase{
		"Filter": {func(c Chain[int]) Chain[int] {
			return c.Filter(func(v int) bool { return v%2 == 1 })
		}, []int{1, 3, 5}},
		"Map": {func(c Chain[int]) Chain[int] {
			return c.Map(func(v int) int { return v * 10 })
		}, []int{10, 20, 30, 40, 50}},
		"FilterMap": {func(c Chain[int]) Chain[int] {
			return c.FilterMap(func(v int) (int, bool) { return v * 10, v%2 == 1 })
		}, []int{10, 30, 50}},
		"FlatMap": {func(c Chain[int]) Chain[int] {
			return c.FlatMap(func(v int) iter.Seq[int] { return slices.Values([]int{v, -v}) })
		}, []int{1, -1, 2, -2, 3, -3, 4, -4, 5, -5}},
		"Take":                   {func(c Chain[int]) Chain[int] { return c.Take(2) }, []int{1, 2}},
		"Take beyond the end":    {func(c Chain[int]) Chain[int] { return c.Take(99) }, []int{1, 2, 3, 4, 5}},
		"Take none":              {func(c Chain[int]) Chain[int] { return c.Take(0) }, nil},
		"Take with a negative n": {func(c Chain[int]) Chain[int] { return c.Take(-1) }, []int{1, 2, 3, 4, 5}},
		"TakeWhile": {func(c Chain[int]) Chain[int] {
			return c.TakeWhile(func(v int) bool { return v < 3 })
		}, []int{1, 2}},
		"TakeWhile stops rather than skipping": {func(c Chain[int]) Chain[int] {
			return c.TakeWhile(func(v int) bool { return v != 3 })
		}, []int{1, 2}},
		"TakeWhile none": {func(c Chain[int]) Chain[int] {
			return c.TakeWhile(func(int) bool { return false })
		}, nil},
		"TakeWhile all": {func(c Chain[int]) Chain[int] {
			return c.TakeWhile(func(int) bool { return true })
		}, []int{1, 2, 3, 4, 5}},
		"Skip":                   {func(c Chain[int]) Chain[int] { return c.Skip(3) }, []int{4, 5}},
		"Skip beyond the end":    {func(c Chain[int]) Chain[int] { return c.Skip(99) }, nil},
		"Skip none":              {func(c Chain[int]) Chain[int] { return c.Skip(0) }, []int{1, 2, 3, 4, 5}},
		"Skip with a negative n": {func(c Chain[int]) Chain[int] { return c.Skip(-1) }, []int{1, 2, 3, 4, 5}},
		"UniqFunc": {func(c Chain[int]) Chain[int] {
			return c.UniqFunc(func(v int) int { return v % 2 })
		}, []int{1, 2}},
	}, func(t *testing.T, c elementWiseCase) {
		src, _ := pulledChain(1, 2, 3, 4, 5)
		assert.DeepEqual(t, "elements", c.op(src).ToSlice(), c.want)
	})
}

// TestChain_flatMapTakesAnySequence pins that FlatMap accepts a function
// returning a Chain as readily as one returning an iter.Seq, and that neither
// spelling needs the type arguments written out. Most of what it pins is that
// this compiles; the comparisons only confirm each spelling yields the same
// elements.
func TestChain_flatMapTakesAnySequence(t *testing.T) {
	pairAsChain := func(v int) Chain[int] { return ChainOf(v, -v) }
	pairAsSeq := func(v int) iter.Seq[int] { return slices.Values([]int{v, -v}) }
	want := []int{1, -1, 2, -2}
	t.Run("FlatMap over a Chain", func(t *testing.T) {
		assert.DeepEqual(t, "FlatMap()", ChainOf(1, 2).FlatMap(pairAsChain).ToSlice(), want)
	})
	t.Run("FlatMap over an iter.Seq", func(t *testing.T) {
		assert.DeepEqual(t, "FlatMap()", ChainOf(1, 2).FlatMap(pairAsSeq).ToSlice(), want)
	})
	t.Run("FlatMap to a different element type", func(t *testing.T) {
		got := ChainOf(1, 2).FlatMap(func(int) Chain[string] { return ChainOf("x") }).ToSlice()
		assert.DeepEqual(t, "FlatMap()", got, []string{"x", "x"})
	})
	t.Run("FlatMapOrErr over a Chain", func(t *testing.T) {
		got, err := ChainOf(1, 2).FlatMapOrErr(func(v int) (Chain[int], error) { return pairAsChain(v), nil }).ToSlice()
		assert.NoError(t, "FlatMapOrErr()", err)
		assert.DeepEqual(t, "FlatMapOrErr()", got, want)
	})
}

// TestChain_earlyReturnStopsTheSource pins the path where the consumer
// stops before the source is exhausted: yield returns false and each
// method must return instead of pulling the next element.
func TestChain_earlyReturnStopsTheSource(t *testing.T) {
	type earlyReturnCase struct {
		op         func(Chain[int]) Chain[int]
		wantPulled int
	}
	tabletest.Run(t, map[string]earlyReturnCase{
		"Filter keeping every element": {func(c Chain[int]) Chain[int] {
			return c.Filter(func(int) bool { return true })
		}, 1},
		"Filter dropping the first two": {func(c Chain[int]) Chain[int] {
			return c.Filter(func(v int) bool { return v > 2 })
		}, 3},
		"Map": {func(c Chain[int]) Chain[int] {
			return c.Map(func(v int) int { return v })
		}, 1},
		"FilterMap": {func(c Chain[int]) Chain[int] {
			return c.FilterMap(func(v int) (int, bool) { return v, true })
		}, 1},
		"FlatMap": {func(c Chain[int]) Chain[int] {
			return c.FlatMap(func(v int) iter.Seq[int] { return slices.Values([]int{v, v}) })
		}, 1},
		"Take":                   {func(c Chain[int]) Chain[int] { return c.Take(4) }, 1},
		"Take with a negative n": {func(c Chain[int]) Chain[int] { return c.Take(-1) }, 1},
		"TakeWhile": {func(c Chain[int]) Chain[int] {
			return c.TakeWhile(func(int) bool { return true })
		}, 1},
		"Skip":                   {func(c Chain[int]) Chain[int] { return c.Skip(2) }, 3},
		"Skip with a negative n": {func(c Chain[int]) Chain[int] { return c.Skip(-1) }, 1},
		"UniqFunc": {func(c Chain[int]) Chain[int] {
			return c.UniqFunc(func(v int) int { return v })
		}, 1},
		"WithIndex": {func(c Chain[int]) Chain[int] {
			return NewChain(func(yield func(int) bool) {
				for indexed := range WithIndex(c).ToSeq() {
					if !yield(indexed.Value) {
						return
					}
				}
			})
		}, 1},
		"failable via FilterOrErr": {func(c Chain[int]) Chain[int] {
			return NewChain(func(yield func(int) bool) {
				for v, err := range c.FilterOrErr(func(int) (bool, error) { return true, nil }).ToSeq2() {
					if err != nil || !yield(v) {
						return
					}
				}
			})
		}, 1},
	}, func(t *testing.T, c earlyReturnCase) {
		src, pulled := pulledChain(1, 2, 3, 4, 5)
		for range c.op(src).ToSeq() {
			break
		}
		assert.Equal(t, "elements pulled from the source", *pulled, c.wantPulled)
	})
}

// TestChain_orErrLiftsIntoAFailableChain pins that every OrErr method on
// Chain reports the error its function raises and stops there, which is
// the behaviour the lift into FailableChain is there to provide.
func TestChain_orErrLiftsIntoAFailableChain(t *testing.T) {
	type orErrCase struct {
		op func(Chain[int]) FailableChain[int]
	}
	tabletest.Run(t, map[string]orErrCase{
		"FilterOrErr": {func(c Chain[int]) FailableChain[int] {
			return c.FilterOrErr(func(v int) (bool, error) { return true, failOn(v, 3) })
		}},
		"MapOrErr": {func(c Chain[int]) FailableChain[int] {
			return c.MapOrErr(func(v int) (int, error) { return v, failOn(v, 3) })
		}},
		"FilterMapOrErr": {func(c Chain[int]) FailableChain[int] {
			return c.FilterMapOrErr(func(v int) (int, bool, error) { return v, true, failOn(v, 3) })
		}},
		"FlatMapOrErr": {func(c Chain[int]) FailableChain[int] {
			return c.FlatMapOrErr(func(v int) (iter.Seq[int], error) {
				return slices.Values([]int{v}), failOn(v, 3)
			})
		}},
		"TakeWhileOrErr": {func(c Chain[int]) FailableChain[int] {
			return c.TakeWhileOrErr(func(v int) (bool, error) { return true, failOn(v, 3) })
		}},
		"UniqFuncOrErr": {func(c Chain[int]) FailableChain[int] {
			return c.UniqFuncOrErr(func(v int) (int, error) { return v, failOn(v, 3) })
		}},
	}, func(t *testing.T, c orErrCase) {
		src, pulled := pulledChain(1, 2, 3, 4, 5)
		got, err := c.op(src).ToSlice()
		assert.MustErrorIs(t, "ToSlice()", err, errBoom)
		assert.Nil(t, "slice alongside the error", got)
		assert.Equal(t, "elements pulled from the source", *pulled, 3)
	})
}

// TestChain_takeWhileOrErrStopsRatherThanFiltering pins where the lift
// lands: TakeWhileOrErr, not FilterOrErr, so the chain ends at the element
// f rejects instead of carrying on past it.
func TestChain_takeWhileOrErrStopsRatherThanFiltering(t *testing.T) {
	src, _ := pulledChain(1, 2, 3)
	got, err := src.TakeWhileOrErr(func(v int) (bool, error) { return v != 2, nil }).ToSlice()
	assert.MustNoError(t, "TakeWhileOrErr().ToSlice()", err)
	assert.DeepEqual(t, "TakeWhileOrErr().ToSlice()", got, []int{1})
}

// failOn reports errBoom when v equals bad, so that the OrErr cases can
// share one failing-function shape.
func failOn(v, bad int) error {
	if v == bad {
		return errBoom
	}
	return nil
}

// TestChain_terminals pins the answers of the methods that end a chain,
// including the two that stop before the source is exhausted.
func TestChain_terminals(t *testing.T) {
	t.Run("ToSlice", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		assert.DeepEqual(t, "ToSlice()", src.ToSlice(), []int{1, 2, 3})
	})
	t.Run("ToSlice of nothing", func(t *testing.T) {
		src, _ := pulledChain()
		assert.Nil(t, "ToSlice()", src.ToSlice())
	})
	t.Run("ToMap", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		assert.DeepEqual(t, "ToMap()", src.ToMap(func(v int) (int, int) { return v, v * 10 }),
			map[int]int{1: 10, 2: 20, 3: 30})
	})
	t.Run("ToMap of nothing is empty rather than nil", func(t *testing.T) {
		src, _ := pulledChain()
		assert.DeepEqual(t, "ToMap()", src.ToMap(func(v int) (int, int) { return v, v }), map[int]int{})
	})
	t.Run("ToMap keeps the value from the last element on a duplicate key", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3, 4, 5)
		assert.DeepEqual(t, "ToMap()", src.ToMap(func(v int) (int, int) { return v % 2, v }),
			map[int]int{0: 4, 1: 5})
	})
	t.Run("ToMapOrErr stops at the first error", func(t *testing.T) {
		src, pulled := pulledChain(1, 2, 3, 4, 5)
		got, err := src.ToMapOrErr(func(v int) (int, int, error) { return v, v, failOn(v, 3) })
		assert.ErrorIs(t, "ToMapOrErr()", err, errBoom)
		assert.Nil(t, "ToMapOrErr() map alongside the error", got)
		assert.Equal(t, "elements pulled from the source", *pulled, 3)
	})
	t.Run("ToSet", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		assert.DeepEqual(t, "ToSet()", src.ToSet(func(v int) int { return v * 10 }),
			map[int]struct{}{10: {}, 20: {}, 30: {}})
	})
	t.Run("ToSet of nothing is empty rather than nil", func(t *testing.T) {
		src, _ := pulledChain()
		assert.DeepEqual(t, "ToSet()", src.ToSet(func(v int) int { return v }), map[int]struct{}{})
	})
	t.Run("ToSet folds the elements giving the same key into one member", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3, 4, 5)
		assert.DeepEqual(t, "ToSet()", src.ToSet(func(v int) int { return v % 2 }),
			map[int]struct{}{0: {}, 1: {}})
	})
	t.Run("Count", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		assert.Equal(t, "Count()", src.Count(), 3)
	})
	t.Run("Reduce folds onto the initial accumulator", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		assert.Equal(t, "Reduce()",
			src.Reduce("seen:", func(acc string, v int) string { return acc + string(rune('0'+v)) }), "seen:123")
	})
	t.Run("Each", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		var seen []int
		src.Each(func(v int) { seen = append(seen, v) })
		assert.DeepEqual(t, "the elements Each saw", seen, []int{1, 2, 3})
	})
	t.Run("EachOrErr stops at the first error", func(t *testing.T) {
		src, pulled := pulledChain(1, 2, 3, 4, 5)
		assert.ErrorIs(t, "EachOrErr()", src.EachOrErr(func(v int) error { return failOn(v, 3) }), errBoom)
		assert.Equal(t, "elements pulled from the source", *pulled, 3)
	})
	t.Run("Find stops at the match", func(t *testing.T) {
		src, pulled := pulledChain(1, 2, 3, 4, 5)
		got, found := src.Find(func(v int) bool { return v == 2 })
		assert.Equal(t, "Find() element", got, 2)
		assert.Equal(t, "Find() found", found, true)
		assert.Equal(t, "elements pulled from the source", *pulled, 2)
	})
	t.Run("Find with no match", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		got, found := src.Find(func(int) bool { return false })
		assert.Equal(t, "Find() element", got, 0)
		assert.Equal(t, "Find() found", found, false)
	})
	t.Run("FindOrErr reports the error its function raises", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		got, found, err := src.FindOrErr(func(v int) (bool, error) { return false, failOn(v, 2) })
		assert.ErrorIs(t, "FindOrErr()", err, errBoom)
		assert.Equal(t, "FindOrErr() element", got, 0)
		assert.Equal(t, "FindOrErr() found", found, false)
	})
	t.Run("ReduceOrErr returns the zero accumulator with the error", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		got, err := src.ReduceOrErr(100, func(acc, v int) (int, error) { return acc + v, failOn(v, 2) })
		assert.ErrorIs(t, "ReduceOrErr()", err, errBoom)
		assert.Equal(t, "ReduceOrErr() accumulator alongside the error", got, 0)
	})
}

// TestWithIndex pins the positions WithIndex hands out, and what a Filter
// on either side of it does to them: one before closes the gaps its dropped
// elements would have left, one after keeps them.
func TestWithIndex(t *testing.T) {
	odd := func(v int) bool { return v%2 == 1 }
	type withIndexCase struct {
		chain Chain[Indexed[int]]
		want  []Indexed[int]
	}
	tabletest.Run(t, map[string]withIndexCase{
		"pairs every element":             {WithIndex(ChainOf(10, 20, 30)), []Indexed[int]{{0, 10}, {1, 20}, {2, 30}}},
		"pairs nothing in an empty chain": {WithIndex(ChainOf[int]()), nil},
		"a Filter before it renumbers": {WithIndex(ChainOf(1, 2, 3, 4, 5).Filter(odd)),
			[]Indexed[int]{{0, 1}, {1, 3}, {2, 5}}},
		"a Filter after it leaves gaps": {WithIndex(ChainOf(1, 2, 3, 4, 5)).Filter(func(indexed Indexed[int]) bool {
			return odd(indexed.Value)
		}), []Indexed[int]{{0, 1}, {2, 3}, {4, 5}}},
	}, func(t *testing.T, c withIndexCase) {
		assert.DeepEqual(t, "elements", c.chain.ToSlice(), c.want)
	})
}

// TestChunked pins the run lengths, the shorter final run, and the panic
// on a run length below one.
func TestChunked(t *testing.T) {
	type chunkedCase struct {
		in   []int
		n    int
		want [][]int
	}
	tabletest.Run(t, map[string]chunkedCase{
		"exact multiple":             {[]int{1, 2, 3, 4}, 2, [][]int{{1, 2}, {3, 4}}},
		"shorter final run":          {[]int{1, 2, 3, 4, 5}, 2, [][]int{{1, 2}, {3, 4}, {5}}},
		"run longer than the source": {[]int{1, 2}, 5, [][]int{{1, 2}}},
		"runs of one":                {[]int{1, 2}, 1, [][]int{{1}, {2}}},
		"empty source":               {nil, 2, nil},
	}, func(t *testing.T, c chunkedCase) {
		src, _ := pulledChain(c.in...)
		assert.DeepEqual(t, "runs", Chunked(src, c.n).ToSlice(), c.want)
	})
	t.Run("stops when the consumer does", func(t *testing.T) {
		src, pulled := pulledChain(1, 2, 3, 4, 5)
		for range Chunked(src, 2).ToSeq() {
			break
		}
		assert.Equal(t, "elements pulled from the source", *pulled, 2)
	})
	t.Run("run length below one panics", func(t *testing.T) {
		defer func() {
			assert.NotNil(t, "the value Chunked(_, 0) panicked with", recover())
		}()
		src, _ := pulledChain()
		Chunked(src, 0)
	})
}

// TestChain_replaysWithItsSource pins the per-iteration state the Chain
// docstring promises: the methods that carry state across elements hold it
// inside the sequence they return, so a replayable source replays.
//
// Each case applies the operation under test once, outside the chain it hands
// back, so that the second round replays that operation rather than a freshly
// built one.
func TestChain_replaysWithItsSource(t *testing.T) {
	cases := []struct {
		name string
		op   func(Chain[int]) Chain[int]
		want []int
	}{
		{"UniqFunc", func(c Chain[int]) Chain[int] {
			return c.UniqFunc(func(v int) int { return v % 3 })
		}, []int{1, 2, 3}},
		{"Take", func(c Chain[int]) Chain[int] { return c.Take(2) }, []int{1, 2}},
		{"Skip", func(c Chain[int]) Chain[int] { return c.Skip(3) }, []int{4, 5}},
		{"WithIndex numbered", func(c Chain[int]) Chain[int] {
			indexed := WithIndex(c)
			return NewChain(func(yield func(int) bool) {
				for pair := range indexed.ToSeq() {
					if !yield(pair.Index) {
						return
					}
				}
			})
		}, []int{0, 1, 2, 3, 4}},
		{"Chunked flattened", func(c Chain[int]) Chain[int] {
			runs := Chunked(c, 2)
			return NewChain(func(yield func(int) bool) {
				for run := range runs.ToSeq() {
					for _, v := range run {
						if !yield(v) {
							return
						}
					}
				}
			})
		}, []int{1, 2, 3, 4, 5}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chain := c.op(ChainOf(1, 2, 3, 4, 5))
			for round := range 2 {
				assert.DeepEqual(t, fmt.Sprintf("round %d", round+1), chain.ToSlice(), c.want)
			}
		})
	}
}

// TestChunked_doesNotRewriteAHeldRun pins why each run is freshly allocated:
// a consumer that keeps a run must still see it unchanged once the next one
// has been produced.
func TestChunked_doesNotRewriteAHeldRun(t *testing.T) {
	var held [][]int
	for run := range Chunked(ChainOf(1, 2, 3, 4), 2).ToSeq() {
		held = append(held, run)
	}
	assert.MustLen(t, "runs", held, 2)
	assert.DeepEqual(t, "the first run, after the second arrived", held[0], []int{1, 2})
}

// TestChain_anyAll pins the two predicates against an empty chain, a match
// at the first and last element, and no match at all. The empty cases are
// the ones a caller is most likely to guess wrong.
func TestChain_anyAll(t *testing.T) {
	type anyAllCase struct {
		in      []int
		f       func(int) bool
		wantAny bool
		wantAll bool
	}
	tabletest.Run(t, map[string]anyAllCase{
		"empty":                 {nil, func(int) bool { return true }, false, true},
		"first element matches": {[]int{2, 1, 1}, func(v int) bool { return v == 2 }, true, false},
		"last element matches":  {[]int{1, 1, 2}, func(v int) bool { return v == 2 }, true, false},
		"none matches":          {[]int{1, 1, 1}, func(v int) bool { return v == 2 }, false, false},
		"every element matches": {[]int{2, 2}, func(v int) bool { return v == 2 }, true, true},
	}, func(t *testing.T, c anyAllCase) {
		assert.Equal(t, "Any()", ChainOf(c.in...).Any(c.f), c.wantAny)
		assert.Equal(t, "All()", ChainOf(c.in...).All(c.f), c.wantAll)
	})
}

// TestChain_terminalsDoNotAllocate pins the inlining these three depend on.
// Their bodies are loops rather than delegations so the compiler can inline
// them; if a later toolchain stops, the chain's closures move to the heap and
// this fails instead of the cost coming back unnoticed. ToSlice and ToMap are
// left out because their allocations are the collection they return, which a
// plain for loop over the same input pays too.
func TestChain_terminalsDoNotAllocate(t *testing.T) {
	src := []int{1, 2, 3, 4, 5, 6, 7, 8}
	type terminalAllocCase struct {
		call func()
	}
	tabletest.Run(t, map[string]terminalAllocCase{
		"Count": {func() { _ = ChainOf(src...).Count() }},
		"Any":   {func() { _ = ChainOf(src...).Any(func(v int) bool { return v == 8 }) }},
		"All":   {func() { _ = ChainOf(src...).All(func(v int) bool { return v > 0 }) }},
	}, func(t *testing.T, c terminalAllocCase) {
		assert.Equal(t, "AllocsPerRun()", testing.AllocsPerRun(100, c.call), float64(0))
	})
}

// TestChain_toMapAllocatesLikeALoop measures ToMap against a plain loop
// rather than against a count. Whatever the toolchain charges for building
// the map it charges to both sides, so a difference can only come from the
// chain itself: the closures an uninlined body puts on the heap.
func TestChain_toMapAllocatesLikeALoop(t *testing.T) {
	src := []int{1, 2, 3, 4, 5, 6, 7, 8}
	pair := func(v int) (int, int) { return v, v * 10 }
	chained := testing.AllocsPerRun(100, func() {
		_ = ChainOf(src...).ToMap(pair)
	})
	looped := testing.AllocsPerRun(100, func() {
		collected := map[int]int{}
		for _, v := range src {
			key, value := pair(v)
			collected[key] = value
		}
		_ = collected
	})
	assert.Equal(t, "the allocations ToMap made, against a plain loop over the same input", chained, looped)
}

// TestChain_toSetAllocatesLikeALoop measures ToSet the way
// TestChain_toMapAllocatesLikeALoop measures ToMap. Expressing ToSet as a
// ToMap over struct{} values is the obvious simplification, and the
// delegation moves the chain's closures to the heap.
func TestChain_toSetAllocatesLikeALoop(t *testing.T) {
	src := []int{1, 2, 3, 4, 5, 6, 7, 8}
	key := func(v int) int { return v * 10 }
	chained := testing.AllocsPerRun(100, func() {
		_ = ChainOf(src...).ToSet(key)
	})
	looped := testing.AllocsPerRun(100, func() {
		collected := map[int]struct{}{}
		for _, v := range src {
			collected[key(v)] = struct{}{}
		}
		_ = collected
	})
	assert.Equal(t, "the allocations ToSet made, against a plain loop over the same input", chained, looped)
}

// TestChain_flatMapOverAChainAllocatesLikeALoop pins that taking the inner
// sequence through a type parameter costs no allocation. It measures against
// the nested loop it stands for, as TestChain_toMapAllocatesLikeALoop does.
func TestChain_flatMapOverAChainAllocatesLikeALoop(t *testing.T) {
	src := []int{1, 2, 3, 4, 5, 6, 7, 8}
	inner := func(v int) Chain[int] { return ChainOf(v) }
	chained := testing.AllocsPerRun(100, func() {
		_ = ChainOf(src...).FlatMap(inner).Count()
	})
	looped := testing.AllocsPerRun(100, func() {
		total := 0
		for _, v := range src {
			for range inner(v) {
				total++
			}
		}
		_ = total
	})
	assert.Equal(t, "the allocations FlatMap made, against the nested loop over the same input", chained, looped)
}

// TestChain_anyAllStopEarly pins that neither predicate keeps pulling once
// its answer is settled: Any at the element that satisfies it, All at the
// element that fails it.
func TestChain_anyAllStopEarly(t *testing.T) {
	t.Run("Any", func(t *testing.T) {
		src, pulled := pulledChain(1, 2, 3, 4, 5)
		assert.MustEqual(t, "Any()", src.Any(func(v int) bool { return v == 2 }), true)
		assert.Equal(t, "elements pulled from the source", *pulled, 2)
	})
	t.Run("All", func(t *testing.T) {
		src, pulled := pulledChain(1, 2, 3, 4, 5)
		assert.MustEqual(t, "All()", src.All(func(v int) bool { return v < 3 }), false)
		assert.Equal(t, "elements pulled from the source", *pulled, 3)
	})
}

// TestChain_anyAllOrErr pins that an error from the predicate stops the
// walk and comes back with false, including for All, where the plain answer
// would otherwise have been true.
func TestChain_anyAllOrErr(t *testing.T) {
	t.Run("AnyOrErr reports the error", func(t *testing.T) {
		src, pulled := pulledChain(1, 2, 3)
		got, err := src.AnyOrErr(func(v int) (bool, error) { return false, failOn(v, 2) })
		assert.ErrorIs(t, "AnyOrErr()", err, errBoom)
		assert.Equal(t, "AnyOrErr() answer alongside the error", got, false)
		assert.Equal(t, "elements pulled from the source", *pulled, 2)
	})
	t.Run("AllOrErr reports false with the error", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		got, err := src.AllOrErr(func(v int) (bool, error) { return true, failOn(v, 2) })
		assert.ErrorIs(t, "AllOrErr()", err, errBoom)
		assert.Equal(t, "AllOrErr() answer alongside the error", got, false)
	})
	t.Run("AllOrErr with no error", func(t *testing.T) {
		src, _ := pulledChain(1, 2, 3)
		got, err := src.AllOrErr(func(v int) (bool, error) { return v > 0, nil })
		assert.NoError(t, "AllOrErr()", err)
		assert.Equal(t, "AllOrErr()", got, true)
	})
}

// TestChain_orErrSettlesBeforeTheError pins the interaction the docstrings
// name: the predicate's error is only reported when no earlier element has
// already settled the answer.
func TestChain_orErrSettlesBeforeTheError(t *testing.T) {
	t.Run("AnyOrErr settles before f fails", func(t *testing.T) {
		src, pulled := pulledChain(1, 2, 3)
		got, err := src.AnyOrErr(func(v int) (bool, error) { return v == 1, failOn(v, 2) })
		assert.NoError(t, "AnyOrErr()", err)
		assert.Equal(t, "AnyOrErr()", got, true)
		assert.Equal(t, "elements pulled from the source", *pulled, 1)
	})
	t.Run("AllOrErr settles before f fails", func(t *testing.T) {
		src, pulled := pulledChain(1, 2, 3)
		got, err := src.AllOrErr(func(v int) (bool, error) { return v != 1, failOn(v, 2) })
		assert.NoError(t, "AllOrErr()", err)
		assert.Equal(t, "AllOrErr()", got, false)
		assert.Equal(t, "elements pulled from the source", *pulled, 1)
	})
}

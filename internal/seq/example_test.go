package seq_test

import (
	"fmt"
	"strconv"

	"github.com/knowledge-work/go-vow/internal/seq"
)

// Example shows a chain that starts out infallible, becomes failable at
// the step that parses, and returns to a function that cannot fail. Only
// the parsing step has to deal in errors.
func Example() {
	total, err := seq.ChainOf("3", "14", "15").
		MapOrErr(strconv.Atoi).
		Filter(func(n int) bool { return n%2 == 1 }).
		Reduce(0, func(sum, n int) int { return sum + n })
	fmt.Println(total, err)
	// Output: 18 <nil>
}

// Example_error shows the same chain meeting an element it cannot parse.
// The accumulator comes back zero rather than partially summed, and the
// element after the failure is never parsed.
func Example_error() {
	total, err := seq.ChainOf("3", "twelve", "15").
		MapOrErr(strconv.Atoi).
		Filter(func(n int) bool { return n%2 == 1 }).
		Reduce(0, func(sum, n int) int { return sum + n })
	fmt.Println(total, err)
	// Output: 0 strconv.Atoi: parsing "twelve": invalid syntax
}

// ExampleNewFailableChain starts from a source that already reports an
// error per element, the shape a reader over rows or files hands back.
func ExampleNewFailableChain() {
	rows := func(yield func(string, error) bool) {
		for _, row := range []string{"3", "14", "15"} {
			if !yield(row, nil) {
				return
			}
		}
	}
	parsed, err := seq.NewFailableChain(rows).
		MapOrErr(strconv.Atoi).
		ToSlice()
	fmt.Println(parsed, err)
	// Output: [3 14 15] <nil>
}

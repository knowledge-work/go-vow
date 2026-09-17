// Package tabletest runs a table of cases as subtests, so a test can
// state each case as a row and its check once, without the case loop
// and the subtest call standing between the reader and the check.
//
// Cases are keyed by their subtest name:
//
//	tabletest.Run(t, map[string]expandCase{
//		"substitutes every param": {body: "(T!, nil)", args: []string{"Result"}, want: "(Result!, nil)"},
//		"rejects wrong arity":     {body: "T", args: []string{"a", "b"}, wantErr: "expects 1"},
//	}, func(t *testing.T, c expandCase) {
//		got, err := expand(c)
//		...
//	})
//
// The map is what supplies the names: a table keyed by name needs no
// name field and no accessor for one, and the compiler rejects a
// repeated case name as a duplicate key. Sorting them also holds a
// case's position in the output when another case is inserted, which
// its index in a slice would not.
//
// The case type has to be named, because the function receiving it
// would otherwise have to spell the anonymous struct out a second time.
// Declaring it inside the test function is enough; it does not have to
// reach the package.
package tabletest

import (
	"maps"
	"slices"
	"testing"
)

// Run runs each case in cases as a subtest named by its key, in the
// order the names sort, so a failing run reads the same way twice.
//
// An empty table stops the test. A table whose cases have all been
// removed or filtered away otherwise reports success while checking
// nothing, and that is the one failure this package can rule out on
// its callers' behalf.
// vow:nil (!,!,!)
func Run[C any](t *testing.T, cases map[string]C, run func(t *testing.T, testCase C)) {
	t.Helper()
	if len(cases) == 0 {
		t.Fatal("tabletest.Run: no cases; an empty table would report success without checking anything")
	}
	for _, name := range slices.Sorted(maps.Keys(cases)) {
		t.Run(name, func(t *testing.T) {
			run(t, cases[name])
		})
	}
}

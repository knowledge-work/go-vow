package cli

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestResolveScopePackages_DropsSyntheticTestPackages pins the
// structural guarantee that no synthetic test package reaches the
// returned slices even when the adopter supplies no suffix
// configuration at all. The harness targets caller_resolver.go as
// the changed file: under `Tests: true`, `packages.Load`
// materializes the owning package's `.test` binary and bracketed
// variants alongside the base package, and the regression this
// test guards against let those leak into the driver's positional
// args (which the loader rejects). Changed must collapse to
// exactly the base package, and Callers must remain populated so a
// future refactor that drops the cross-package caller shape
// surfaces as a test failure rather than silent coverage loss.
func TestResolveScopePackages_DropsSyntheticTestPackages(t *testing.T) {
	// The resolver loads `./...` relative to the process working
	// directory and production invokes it from the module root, so
	// the fixture hops up from internal/cli to match; a package-dir
	// working directory would leave the module-level callers
	// outside Phase 1's view.
	t.Chdir("../..")
	// runtime.Caller(0) reports this test file's compile-time
	// embedded path rather than a hardcoded literal, so a directory
	// move keeps the fixture accurate. The changed file must be a
	// sibling whose declarations the cmd/vow package actually
	// references (this test file's own declarations are referenced
	// by nobody, so it would resolve to zero callers).
	_, testFile, _, ok := runtime.Caller(0)
	assert.MustEqual(t, "runtime.Caller(0) resolved this test file's path", ok, true)
	changedFile := filepath.Join(filepath.Dir(testFile), "caller_resolver.go")
	scope, err := ResolveScopePackages([]string{changedFile}, nil)
	assert.MustNoError(t, "ResolveScopePackages", err)
	assert.MustNotNil(t, "scope; a real changed file must resolve to a non-nil scope", scope)
	assert.DeepEqual(t, "Changed", scope.Changed, []string{"github.com/knowledge-work/go-vow/internal/cli"})

	// The synthetic check is a backstop: the structural filter drops
	// synthetic packages upstream, so this catches a leak arriving by
	// some other route.
	wantCaller := "github.com/knowledge-work/go-vow/cmd/vow"
	for _, p := range scope.Callers {
		assert.Equal(t, "synthetic .test binary entry "+p, strings.HasSuffix(p, ".test"), false)
		assert.NotContains(t, "synthetic bracketed test variant entry", p, " [")
	}
	assert.Equal(t, "Callers includes the real cross-package caller "+wantCaller,
		slices.Contains(scope.Callers, wantCaller), true)
}

func TestResolveScopePackages_EmptyInput(t *testing.T) {
	type emptyInputCase struct {
		changed []string
	}

	tabletest.Run(t, map[string]emptyInputCase{
		"nil slice returns nil":         {changed: nil},
		"zero-length slice returns nil": {changed: []string{}},
	}, func(t *testing.T, c emptyInputCase) {
		got, err := ResolveScopePackages(c.changed, nil)
		assert.MustNoError(t, "ResolveScopePackages", err)
		assert.Nil(t, "ResolveScopePackages", got)
	})
}

// TestSortedKeys pins the helper that turns the resolver's
// internal de-duplication map into the ScopePackages slice form.
// The contract is: keys returned in lexical order, no duplicates,
// nil set yields an empty (non-nil) slice.
func TestSortedKeys(t *testing.T) {
	type sortedKeysCase struct {
		in   map[string]struct{}
		want []string
	}

	tabletest.Run(t, map[string]sortedKeysCase{
		"nil set yields empty slice": {in: nil, want: []string{}},
		"single entry":               {in: map[string]struct{}{"a": {}}, want: []string{"a"}},
		"multiple entries sort lexically": {
			in:   map[string]struct{}{"c": {}, "a": {}, "b": {}},
			want: []string{"a", "b", "c"},
		},
	}, func(t *testing.T, c sortedKeysCase) {
		assert.DeepEqual(t, "sortedKeys", sortedKeys(c.in), c.want)
	})
}

// TestReverseImportTraversal pins the upper-bound caller-candidate
// set the resolver hands to Phase 2. The contract is: a package
// whose import list mentions any changed package becomes a
// candidate; packages already in the changed set are excluded; an
// importer that mentions multiple changed packages still appears
// at most once.
func TestReverseImportTraversal(t *testing.T) {
	mkPkg := func(path string, imports ...string) *packages.Package {
		impMap := make(map[string]*packages.Package, len(imports))
		for _, imp := range imports {
			impMap[imp] = nil
		}
		return &packages.Package{PkgPath: path, Imports: impMap}
	}
	pkgs := []*packages.Package{
		mkPkg("example.com/changed/a"),
		mkPkg("example.com/changed/b"),
		mkPkg("example.com/caller/one", "example.com/changed/a"),
		mkPkg("example.com/caller/two", "example.com/changed/a", "example.com/changed/b"),
		mkPkg("example.com/unrelated"),
		mkPkg("example.com/transitive", "example.com/caller/one"),
		// The nil-Imports and self-import shapes the cases below pin.
		{PkgPath: "example.com/nilimports", Imports: nil},
		{
			PkgPath: "example.com/self",
			Imports: map[string]*packages.Package{
				"example.com/self": nil,
			},
		},
	}

	type traversalCase struct {
		changed map[string]struct{}
		want    map[string]struct{}
	}

	tabletest.Run(t, map[string]traversalCase{
		"single changed package finds its direct importers": {
			changed: map[string]struct{}{"example.com/changed/a": {}},
			want: map[string]struct{}{
				"example.com/caller/one": {},
				"example.com/caller/two": {},
			},
		},
		"multi-import caller appears once even with several changed packages": {
			changed: map[string]struct{}{
				"example.com/changed/a": {},
				"example.com/changed/b": {},
			},
			want: map[string]struct{}{
				"example.com/caller/one": {},
				"example.com/caller/two": {},
			},
		},
		"transitive importers are not included": {
			changed: map[string]struct{}{"example.com/changed/a": {}},
			want: map[string]struct{}{
				"example.com/caller/one": {},
				"example.com/caller/two": {},
			},
		},
		"no callers when nothing imports the changed set": {
			changed: map[string]struct{}{"example.com/unrelated": {}},
			want:    map[string]struct{}{},
		},
		"changed packages themselves never appear as candidates": {
			changed: map[string]struct{}{"example.com/caller/one": {}},
			want:    map[string]struct{}{"example.com/transitive": {}},
		},
		"package with nil Imports map is iterated as no-op": {
			changed: map[string]struct{}{"example.com/changed/a": {}},
			want: map[string]struct{}{
				"example.com/caller/one": {},
				"example.com/caller/two": {},
			},
		},
		"self-import on a changed package is excluded from candidates": {
			changed: map[string]struct{}{"example.com/self": {}},
			want:    map[string]struct{}{},
		},
	}, func(t *testing.T, c traversalCase) {
		assert.DeepEqual(t, "reverseImportTraversal", reverseImportTraversal(pkgs, c.changed), c.want)
	})
}

// TestFilterBySuffix pins the helper behind the adopter-intent
// knob that removes scope entries whose import path ends with an
// excluded suffix (e.g. generated mock packages). Synthetic test
// packages are removed structurally before this filter applies,
// so the helper stays a plain generic suffix match.
func TestFilterBySuffix(t *testing.T) {
	type suffixCase struct {
		paths   []string
		exclude []string
		want    []string
	}

	tabletest.Run(t, map[string]suffixCase{
		"empty exclusion list returns input unchanged": {
			paths:   []string{"a", "b"},
			exclude: nil,
			want:    []string{"a", "b"},
		},
		"test suffix dropped": {
			paths:   []string{"example.com/foo", "example.com/foo_test", "example.com/bar"},
			exclude: []string{"_test"},
			want:    []string{"example.com/foo", "example.com/bar"},
		},
		"multiple suffixes match": {
			paths:   []string{"example.com/foo_test", "example.com/bar_mock", "example.com/baz"},
			exclude: []string{"_test", "_mock"},
			want:    []string{"example.com/baz"},
		},
		"empty suffix entry ignored": {
			paths:   []string{"example.com/foo", "example.com/foo_test"},
			exclude: []string{"", "_test"},
			want:    []string{"example.com/foo"},
		},
		"no matches keeps every path": {
			paths:   []string{"example.com/foo", "example.com/bar"},
			exclude: []string{"_test"},
			want:    []string{"example.com/foo", "example.com/bar"},
		},
		"empty input slice yields empty result": {
			paths:   []string{},
			exclude: []string{"_test"},
			want:    []string{},
		},
		"suffix in middle of path does not match": {
			paths:   []string{"example.com/_test_helpers/foo"},
			exclude: []string{"_test"},
			want:    []string{"example.com/_test_helpers/foo"},
		},
		"dot-test suffix (test binary path) drops when configured": {
			paths:   []string{"example.com/pkg", "example.com/pkg.test", "example.com/pkg_test"},
			exclude: []string{".test"},
			want:    []string{"example.com/pkg", "example.com/pkg_test"},
		},
		"underscore-test and dot-test both drop when both configured": {
			paths:   []string{"example.com/pkg", "example.com/pkg.test", "example.com/pkg_test"},
			exclude: []string{"_test", ".test"},
			want:    []string{"example.com/pkg"},
		},
	}, func(t *testing.T, c suffixCase) {
		assert.DeepEqual(t, "filterBySuffix", filterBySuffix(c.paths, c.exclude), c.want)
	})
}

// TestLoadablePackagePaths pins the structural discriminator that
// separates real, pattern-loadable packages from the synthetic
// shapes `Tests: true` materializes. The decision never inspects
// path spelling: a bracketed variant falls out through ID !=
// PkgPath, the `.test` binary through its empty compiled-file
// list, and a real package whose import path legally ends in
// `_test` stays loadable.
func TestLoadablePackagePaths(t *testing.T) {
	pkgs := []*packages.Package{
		{
			ID:              "example.com/pkg",
			PkgPath:         "example.com/pkg",
			CompiledGoFiles: []string{"/x/pkg/a.go"},
		},
		{
			ID:              "example.com/pkg [example.com/pkg.test]",
			PkgPath:         "example.com/pkg",
			CompiledGoFiles: []string{"/x/pkg/a.go", "/x/pkg/a_test.go"},
		},
		{
			ID:              "example.com/pkg_test [example.com/pkg.test]",
			PkgPath:         "example.com/pkg_test",
			CompiledGoFiles: []string{"/x/pkg/b_test.go"},
		},
		{
			ID:      "example.com/pkg.test",
			PkgPath: "example.com/pkg.test",
		},
		{
			ID:              "example.com/e2e_test",
			PkgPath:         "example.com/e2e_test",
			CompiledGoFiles: []string{"/x/e2e_test/e2e.go"},
		},
	}
	assert.DeepEqual(t, "loadablePackagePaths", loadablePackagePaths(pkgs), map[string]struct{}{
		"example.com/pkg":      {},
		"example.com/e2e_test": {},
	})
}

// TestFilterLoadable pins the allowlist projection the resolver
// applies to every outgoing path slice: membership filtering with
// input order preserved and a non-nil empty result for empty
// input.
func TestFilterLoadable(t *testing.T) {
	loadable := map[string]struct{}{
		"example.com/a": {},
		"example.com/c": {},
	}
	type loadableCase struct {
		paths []string
		want  []string
	}

	tabletest.Run(t, map[string]loadableCase{
		"keeps only allowlisted paths in input order": {
			paths: []string{"example.com/c", "example.com/b", "example.com/a"},
			want:  []string{"example.com/c", "example.com/a"},
		},
		"empty input yields empty non-nil slice": {
			paths: []string{},
			want:  []string{},
		},
		"no members yields empty slice": {
			paths: []string{"example.com/b"},
			want:  []string{},
		},
	}, func(t *testing.T, c loadableCase) {
		assert.DeepEqual(t, "filterLoadable", filterLoadable(c.paths, loadable), c.want)
	})
}

func TestChangedPackages(t *testing.T) {
	pkgs := []*packages.Package{
		{
			PkgPath:         "example.com/foo",
			CompiledGoFiles: []string{"/x/foo/a.go", "/x/foo/b.go"},
		},
		{
			PkgPath:         "example.com/bar",
			CompiledGoFiles: []string{"/x/bar/c.go"},
		},
	}
	type changedCase struct {
		changed   []string
		want      map[string]struct{}
		wantFiles []string
	}

	tabletest.Run(t, map[string]changedCase{
		"single file maps to its package": {
			changed:   []string{"/x/foo/a.go"},
			want:      map[string]struct{}{"example.com/foo": {}},
			wantFiles: []string{"/x/foo/a.go"},
		},
		"multiple files in one package collapse to a single entry": {
			changed:   []string{"/x/foo/a.go", "/x/foo/b.go"},
			want:      map[string]struct{}{"example.com/foo": {}},
			wantFiles: []string{"/x/foo/a.go", "/x/foo/b.go"},
		},
		"files in different packages produce a set of entries": {
			changed:   []string{"/x/foo/a.go", "/x/bar/c.go"},
			want:      map[string]struct{}{"example.com/foo": {}, "example.com/bar": {}},
			wantFiles: []string{"/x/foo/a.go", "/x/bar/c.go"},
		},
		"unknown files are skipped in both results": {
			changed:   []string{"/x/foo/a.go", "/x/zzz/never.go", "/x/go.mod"},
			want:      map[string]struct{}{"example.com/foo": {}},
			wantFiles: []string{"/x/foo/a.go"},
		},
		"no matching files yield empty results": {
			changed:   []string{"/x/zzz/never.go"},
			want:      map[string]struct{}{},
			wantFiles: []string{},
		},
	}, func(t *testing.T, c changedCase) {
		got, gotFiles := changedPackages(pkgs, c.changed)
		assert.DeepEqual(t, "changedPackages packages", got, c.want)
		assert.DeepEqual(t, "changedPackages files", gotFiles, c.wantFiles)
	})
}

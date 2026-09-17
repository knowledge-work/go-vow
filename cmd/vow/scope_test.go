package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	vowanalysis "github.com/knowledge-work/go-vow/internal/analysis"
	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/config"
	"github.com/knowledge-work/go-vow/internal/driver"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestMatchesScopePrefix(t *testing.T) {
	type scopeCase struct {
		path     string
		prefixes []string
		want     bool
	}
	tabletest.Run(t, map[string]scopeCase{
		"exact match":      {"example.com/foo", []string{"example.com/foo"}, true},
		"subpackage match": {"example.com/foo/bar", []string{"example.com/foo"}, true},
		"sibling prefix does not match across the path boundary": {"example.com/foobar", []string{"example.com/foo"}, false},
		"trailing slash on the prefix is normalised away":        {"example.com/foo/bar", []string{"example.com/foo/"}, true},
		"empty prefix matches nothing":                           {"example.com/foo", []string{""}, false},
		"second prefix can match":                                {"other.example/lib", []string{"example.com/foo", "other.example"}, true},
	}, func(t *testing.T, c scopeCase) {
		assert.Equal(t, fmt.Sprintf("matchesScopePrefix(%q, %v)", c.path, c.prefixes),
			matchesScopePrefix(c.path, c.prefixes), c.want)
	})
}

// scopeFixtureDir returns the absolute path of the scope-mode
// fixture module.
func scopeFixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "scope"))
	assert.MustNoError(t, "resolve testdata", err)
	return dir
}

// runOn loads patterns from dir — through the first-party scope
// when prefixes is non-empty, through the whole-closure default
// otherwise — runs the real analyzer over the roots, and returns
// the printed diagnostics.
func runOn(t *testing.T, dir string, patterns, prefixes []string) string {
	t.Helper()
	var roots, loaded []*packages.Package
	var err error
	if len(prefixes) > 0 {
		roots, loaded, err = loadScopedPackages(dir, false, patterns, prefixes)
	} else {
		roots, err = loadTargetPackages(dir, false, patterns)
		loaded = roots
	}
	assert.MustNoError(t, "load", err)
	assert.MustEqual(t, "packages.PrintErrors(loaded)", packages.PrintErrors(loaded), 0)
	result, err := driver.Run([]*analysis.Analyzer{vowanalysis.NewDefault(&config.Config{})}, roots)
	assert.MustNoError(t, "driver.Run", err)
	var buf bytes.Buffer
	assert.MustNoError(t, "driver.PrintDiagnostics", driver.PrintDiagnostics(&buf, result))
	return buf.String()
}

// The scoped load must promote the marker-bearing dependency to a
// pattern (with syntax), keep the original target as the only
// root, and leave the out-of-scope stdlib dependency unparsed.
//
// The comparisons below are order-sensitive. scopeExtraPatterns sorts
// its result for a deterministic load, and packages.Load returns in
// the order of the patterns it was given — which its documentation
// does not promise. An x/tools upgrade that reorders the result fails
// here first.
func TestLoadScopedPackagesPartition(t *testing.T) {
	dir := scopeFixtureDir(t)
	roots, loaded, err := loadScopedPackages(dir, false, []string{"./root"}, []string{"vowtest.example"})
	assert.MustNoError(t, "loadScopedPackages", err)
	assert.MustDeepEqual(t, "loadScopedPackages roots", pkgPaths(roots), []string{"vowtest.example/scope/root"})
	assert.MustDeepEqual(t, "loadScopedPackages loaded", pkgPaths(loaded),
		[]string{"vowtest.example/scope/dep", "vowtest.example/scope/root"})
	for _, pkg := range loaded {
		assert.Equal(t, fmt.Sprintf("len(%s.Syntax) > 0", pkg.PkgPath), len(pkg.Syntax) > 0, true)
	}
	// The stdlib dependency stays a stub: reachable in the import
	// graph, never parsed.
	var sawStringsStub bool
	packages.Visit(loaded, nil, func(pkg *packages.Package) {
		if pkg.PkgPath == "strings" {
			sawStringsStub = true
			assert.Equal(t, "len(strings.Syntax); an out-of-scope dependency must stay unparsed", len(pkg.Syntax), 0)
		}
	})
	assert.Equal(t, "strings reachable in the loaded graph", sawStringsStub, true)
}

// The scoped run must print exactly the diagnostics the
// whole-closure run prints: the promoted dependency's fact still
// drives the cross-package check, and the dependency's own
// diagnostics stay unprinted because it is not a root.
func TestScopedRunMatchesWholeClosureRun(t *testing.T) {
	dir := scopeFixtureDir(t)
	full := runOn(t, dir, []string{"./root"}, nil)
	scoped := runOn(t, dir, []string{"./root"}, []string{"vowtest.example"})
	if scoped != full {
		t.Errorf("scoped diagnostics differ from whole-closure run\nscoped:\n%s\nfull:\n%s", scoped, full)
	}
	if !strings.Contains(scoped, "nil") {
		t.Errorf("expected the cross-package nil diagnostic to fire; got:\n%s", scoped)
	}
}

func pkgPaths(pkgs []*packages.Package) []string {
	paths := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		paths = append(paths, pkg.PkgPath)
	}
	return paths
}

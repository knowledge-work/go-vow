package main

import (
	"path/filepath"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestSentinelFactReachesTheImportingPackage checks that the default
// driver and --vet-mode report the same leak of a sentinel declared in
// a package outside the run's patterns. The default driver hands the
// fact over in memory and --vet-mode through the build cache, so each
// has its own way to lose it.
func TestSentinelFactReachesTheImportingPackage(t *testing.T) {
	binary := buildVowBinary(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "sentinelxpkg"))
	assert.MustNoError(t, "resolve fixture", err)
	const config = "nil_decl:\n  require_declarations: false\n"

	type runCase struct {
		args []string // flags placed before the pattern
		leak string   // a leak the run must report
	}
	tabletest.Run(t, map[string]runCase{
		"package files": {
			leak: "use.go:9:9: vow[sentinel-error]: sentinel error decl.ErrGone leaked",
		},
		"external test package": {
			args: []string{"-test"},
			leak: "use_test.go:12:9: vow[sentinel-error]: sentinel error decl.ErrGone leaked",
		},
	}, func(t *testing.T, c runCase) {
		driver := runVowBinary(t, binary, dir, append(append([]string{"--config-yaml", config}, c.args...), "./use")...)
		vet := runVowBinary(t, binary, dir, append(append([]string{"--vet-mode", "--config-yaml", config}, c.args...), "./use")...)
		assert.MustContains(t, "default driver output", driver, c.leak)
		assert.Equal(t, "--vet-mode output", vet, driver)
	})
}

// TestSentinelFactReachesTheImportingPackageUnderAScope checks that a
// first-party scope, which loads the declaring package from source only
// because its path matches a prefix, still hands its fact to the
// importing package.
func TestSentinelFactReachesTheImportingPackageUnderAScope(t *testing.T) {
	binary := buildVowBinary(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "sentinelxpkg"))
	assert.MustNoError(t, "resolve fixture", err)
	const config = "nil_decl:\n  require_declarations: false\n"
	const scoped = config + "analysis_scope:\n  first_party_prefixes: [sentinelxpkg]\n"

	whole := runVowBinary(t, binary, dir, "--config-yaml", config, "./use")
	scope := runVowBinary(t, binary, dir, "--config-yaml", scoped, "./use")
	assert.MustContains(t, "whole-closure output", whole, "sentinel error decl.ErrGone leaked")
	assert.Equal(t, "first-party scope output", scope, whole)
}

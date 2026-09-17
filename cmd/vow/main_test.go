package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	vowanalysis "github.com/knowledge-work/go-vow/internal/analysis"
	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/cli"
	"github.com/knowledge-work/go-vow/internal/config"
	"github.com/knowledge-work/go-vow/internal/driver"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestParseDriverArgs(t *testing.T) {
	type argsCase struct {
		args          []string
		wantFlags     driverFlags
		wantRemaining []string
		wantErr       string
	}
	tabletest.Run(t, map[string]argsCase{
		"default package args fall through unchanged": {
			args:          []string{"./...", "github.com/foo/bar"},
			wantRemaining: []string{"./...", "github.com/foo/bar"},
		},
		"changed-files with variadic paths": {
			args:      []string{"--changed-files", "a.go", "b.go", "c.go"},
			wantFlags: driverFlags{Changed: cli.ChangedFileSet{Files: []string{"a.go", "b.go", "c.go"}}},
		},
		"changed-files with companion with-callers": {
			args:      []string{"--changed-files", "a.go", "b.go", "--with-callers"},
			wantFlags: driverFlags{Changed: cli.ChangedFileSet{Files: []string{"a.go", "b.go"}, WithCallers: true}},
		},
		"empty changed-files expansion with with-callers": {
			args:      []string{"--changed-files", "--with-callers"},
			wantFlags: driverFlags{Changed: cli.ChangedFileSet{WithCallers: true}},
		},
		"with-callers without changed-files is rejected": {
			args:    []string{"--with-callers", "./..."},
			wantErr: "--with-callers requires --changed-files",
		},
		"changed-files followed by package args via separator": {
			args:          []string{"--changed-files", "a.go", "b.go", "--", "./..."},
			wantFlags:     driverFlags{Changed: cli.ChangedFileSet{Files: []string{"a.go", "b.go"}}},
			wantRemaining: []string{"--", "./..."},
		},
		"package arg without separator is captured as file": {
			args:      []string{"--changed-files", "a.go", "./..."},
			wantFlags: driverFlags{Changed: cli.ChangedFileSet{Files: []string{"a.go", "./..."}}},
		},
		"repeated changed-files accumulates": {
			args:      []string{"--changed-files", "a.go", "--changed-files", "b.go"},
			wantFlags: driverFlags{Changed: cli.ChangedFileSet{Files: []string{"a.go", "b.go"}}},
		},
		"config-file standalone": {
			args:          []string{"--config-file", "vow.yaml", "./..."},
			wantFlags:     driverFlags{ConfigFile: "vow.yaml"},
			wantRemaining: []string{"./..."},
		},
		"config-file combined with changed-files": {
			args:      []string{"--changed-files", "a.go", "--config-file", "vow.yaml", "--with-callers"},
			wantFlags: driverFlags{Changed: cli.ChangedFileSet{Files: []string{"a.go"}, WithCallers: true}, ConfigFile: "vow.yaml"},
		},
		"config-file missing value": {
			args:    []string{"--config-file"},
			wantErr: "--config-file requires a path argument",
		},
		"config-yaml inline content": {
			args:          []string{"--config-yaml", "closable:\n  built_in_closables: []\n", "./..."},
			wantFlags:     driverFlags{ConfigYAML: "closable:\n  built_in_closables: []\n"},
			wantRemaining: []string{"./..."},
		},
		"config-yaml missing value": {
			args:    []string{"--config-yaml"},
			wantErr: "--config-yaml requires a yaml content argument",
		},
		"config-file and config-yaml are mutually exclusive": {
			args:    []string{"--config-file", "vow.yaml", "--config-yaml", "closable: {}"},
			wantErr: "--config-file and --config-yaml are mutually exclusive",
		},
		"test flag opts test files in via single dash": {
			args:          []string{"-test", "./..."},
			wantFlags:     driverFlags{IncludeTests: true},
			wantRemaining: []string{"./..."},
		},
		"test flag opts test files in via double dash": {
			args:          []string{"--test", "./..."},
			wantFlags:     driverFlags{IncludeTests: true},
			wantRemaining: []string{"./..."},
		},
		"test flag defaults to false when absent": {
			args:          []string{"./..."},
			wantRemaining: []string{"./..."},
		},
		"test flag composes with changed-files and positional pattern": {
			args:          []string{"--changed-files", "a.go", "-test", "./..."},
			wantFlags:     driverFlags{Changed: cli.ChangedFileSet{Files: []string{"a.go"}}, IncludeTests: true},
			wantRemaining: []string{"./..."},
		},
	}, func(t *testing.T, c argsCase) {
		flags, remaining, err := parseDriverArgs(c.args)
		if c.wantErr != "" {
			assert.MustError(t, "parseDriverArgs", err)
			assert.Equal(t, "parseDriverArgs error", err.Error(), c.wantErr)
			return
		}
		assert.MustNoError(t, "parseDriverArgs", err)
		assert.DeepEqual(t, "flags", flags, c.wantFlags)
		assert.DeepEqual(t, "remaining", remaining, c.wantRemaining)
	})
}

// TestLoadTargetPackages_TestsFlag drives the full analyzer against a
// module whose sentinel-leak diagnostic sits in a _test.go file. The
// default run must load the production file only and surface no
// diagnostic; the -test-equivalent run must admit the test file and
// surface the sentinel diagnostic.
func TestLoadTargetPackages_TestsFlag(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("testdata", "testflag"))
	assert.MustNoError(t, "resolve testdata", err)
	t.Run("test file excluded by default", func(t *testing.T) {
		assert.NotContains(t, "diagnostics without -test", runAnalyzer(t, dir, false), "ErrTestOnly")
	})
	t.Run("test file included with -test", func(t *testing.T) {
		assert.Contains(t, "diagnostics with -test", runAnalyzer(t, dir, true), "ErrTestOnly")
	})
}

func runAnalyzer(t *testing.T, dir string, includeTests bool) string {
	t.Helper()
	pkgs, err := loadTargetPackages(dir, includeTests, []string{"./..."})
	assert.MustNoError(t, "loadTargetPackages", err)
	// Fail on package-level load errors so a broken testdata module
	// cannot pass silently by returning zero diagnostics.
	assert.MustEqual(t, "packages.PrintErrors(pkgs)", packages.PrintErrors(pkgs), 0)
	// Pin the analyzer to an explicit empty config so a stray vow.yaml
	// somewhere above the testdata module cannot leak into this run.
	result, err := driver.Run([]*analysis.Analyzer{vowanalysis.NewDefault(&config.Config{})}, pkgs)
	assert.MustNoError(t, "driver.Run", err)
	var buf bytes.Buffer
	assert.MustNoError(t, "driver.PrintDiagnostics", driver.PrintDiagnostics(&buf, result))
	return buf.String()
}

func TestScopePositionals(t *testing.T) {
	type scopePosCase struct {
		scope *cli.ScopePackages
		want  []string
	}
	tabletest.Run(t, map[string]scopePosCase{
		"nil scope resolves to nothing":         {scope: nil},
		"both halves empty resolves to nothing": {scope: &cli.ScopePackages{}},
		"changed only":                          {scope: &cli.ScopePackages{Changed: []string{"a"}}, want: []string{"a"}},
		"changed before callers":                {scope: &cli.ScopePackages{Changed: []string{"a"}, Callers: []string{"b", "c"}}, want: []string{"a", "b", "c"}},
	}, func(t *testing.T, c scopePosCase) {
		assert.DeepEqual(t, "scopePositionals()", scopePositionals(c.scope), c.want)
	})
}

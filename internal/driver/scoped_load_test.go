package driver

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/knowledge-work/go-vow/internal/assert"
)

// loadScopedTestdataPkgs mirrors the CLI's first-party scope load:
// syntax and type information land only on the packages the
// patterns name, while their dependencies contribute types from
// export data and carry no syntax. The tests below use it to pin
// how Run behaves when the import graph contains syntax-less
// packages.
func loadScopedTestdataPkgs(t *testing.T, patterns ...string) []*packages.Package {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "xpkg"))
	assert.MustNoError(t, "filepath.Abs(testdata/xpkg)", err)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedModule,
		Dir: root,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	assert.MustNoError(t, "packages.Load", err)
	assert.MustEqual(t, "packages.PrintErrors", packages.PrintErrors(pkgs), 0)
	return pkgs
}

// A dependency loaded without syntax gets no job: the analyzer
// cannot run there, and with no source in scope no fact can
// originate there. The run completes without error; the fact-borne
// diagnostic simply cannot fire because its exporter was never
// scheduled.
func TestRun_ScopedLoadSkipsSyntaxlessImport(t *testing.T) {
	pkgs := loadScopedTestdataPkgs(t, "./caller")
	result, err := Run([]*analysis.Analyzer{newMarkAnalyzer()}, pkgs)
	assert.MustNoError(t, "Run", err)
	calleeJobs := 0
	for _, j := range result.Jobs {
		assert.NoError(t, fmt.Sprintf("job %s@%s", j.Analyzer.Name, j.Package.PkgPath), j.Err)
		if j.Package.PkgPath == "vowdrivertest.example/xpkg/callee" {
			calleeJobs++
		}
	}
	assert.Equal(t, "jobs scheduled for the syntax-less dependency", calleeJobs, 0)
	var buf bytes.Buffer
	assert.MustNoError(t, "PrintDiagnostics", PrintDiagnostics(&buf, result))
	assert.NotContains(t, "printed diagnostics", buf.String(), "marked call to Marked")
}

// A dependency the scoped load DID materialise with syntax — the
// CLI promotes in-scope dependencies to load patterns — keeps its
// job and its facts, and because only the original targets are
// handed to Run as roots, its own diagnostics stay unprinted. This
// is the invariant the first-party scope mode rests on.
func TestRun_ScopedLoadKeepsFactsForLoadedExtras(t *testing.T) {
	pkgs := loadScopedTestdataPkgs(t, "./caller", "./callee")
	callerRoot := findPkg(t, pkgs, "vowdrivertest.example/xpkg/caller")
	result, err := Run([]*analysis.Analyzer{newMarkAnalyzer()}, []*packages.Package{callerRoot})
	assert.MustNoError(t, "Run", err)
	var calleeJob *Job
	for _, j := range result.Jobs {
		if j.Package.PkgPath == "vowdrivertest.example/xpkg/callee" && j.Analyzer.Name == "markanalyzer" {
			calleeJob = j
		}
	}
	assert.MustNotNil(t, "job for the syntax-bearing callee dependency", calleeJob)
	assert.Equal(t, "callee dependency job Root", calleeJob.Root, false)
	var buf bytes.Buffer
	assert.MustNoError(t, "PrintDiagnostics", PrintDiagnostics(&buf, result))
	assert.Contains(t, "printed diagnostics", buf.String(), "marked call to Marked")
	assert.Contains(t, "printed diagnostics", buf.String(), "package vowdrivertest.example/xpkg/callee")
}

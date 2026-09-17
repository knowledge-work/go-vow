package driver

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/knowledge-work/go-vow/internal/assert"
)

// markFact tags an object as "of interest" for the fact-propagation
// test. It has no payload; its presence in the fact table is what
// the caller-side pass reads.
type markFact struct{}

func (*markFact) AFact() {}

// pkgTag is a package-scoped fact carrying the exporting package's
// import path. It lets the caller-side pass verify that
// ImportPackageFact returns something inheritance produced.
type pkgTag struct {
	Path string
}

func (*pkgTag) AFact() {}

// newMarkAnalyzer returns an analyzer that (a) exports markFact
// against every top-level function whose name is "Marked" or
// "helper" in the current package, (b) exports pkgTag against
// every package it visits, and (c) reports a diagnostic per call
// whose callee carries markFact and a diagnostic per import whose
// target carries pkgTag. The report shape ties the diagnostic to
// the call/import expression so tests can pin its position.
func newMarkAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:      "markanalyzer",
		Doc:       "test analyzer that propagates a marker fact across imports",
		FactTypes: []analysis.Fact{(*markFact)(nil), (*pkgTag)(nil)},
		Run:       markRun,
	}
}

func markRun(pass *analysis.Pass) (any, error) {
	pass.ExportPackageFact(&pkgTag{Path: pass.Pkg.Path()})
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			if obj == nil {
				continue
			}
			if fn.Name.Name == "Marked" || fn.Name.Name == "helper" || fn.Name.Name == "Twin" {
				pass.ExportObjectFact(obj, &markFact{})
			}
		}
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var callee types.Object
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				callee = pass.TypesInfo.Uses[fn.Sel]
			case *ast.Ident:
				callee = pass.TypesInfo.Uses[fn]
			}
			if callee == nil {
				return true
			}
			var fact markFact
			if pass.ImportObjectFact(callee, &fact) {
				pass.Reportf(call.Pos(), "marked call to %s", callee.Name())
			}
			return true
		})
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			target := pass.Pkg.Imports()
			for _, tp := range target {
				if tp.Path() != path {
					continue
				}
				var tag pkgTag
				if pass.ImportPackageFact(tp, &tag) {
					pass.Reportf(imp.Pos(), "package %s tagged as %s", path, tag.Path)
				}
			}
		}
	}
	// The result is non-nil so that a released result is
	// distinguishable from one the analyzer never produced.
	return struct{}{}, nil
}

// loadTestdataPkgs invokes packages.Load rooted at testdata/xpkg
// so a driver.Run call has a real package set to work over. It
// wraps the common config the tests use.
func loadTestdataPkgs(t *testing.T, tests bool, patterns ...string) []*packages.Package {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "xpkg"))
	assert.MustNoError(t, "filepath.Abs(testdata/xpkg)", err)
	cfg := &packages.Config{
		Mode:  packages.LoadAllSyntax | packages.NeedModule,
		Tests: tests,
		Dir:   root,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	assert.MustNoError(t, "packages.Load", err)
	assert.MustEqual(t, "packages.PrintErrors", packages.PrintErrors(pkgs), 0)
	return pkgs
}

func TestRun_FactPropagatesAcrossImport(t *testing.T) {
	pkgs := loadTestdataPkgs(t, false, "./caller")
	result, err := Run([]*analysis.Analyzer{newMarkAnalyzer()}, pkgs)
	assert.MustNoError(t, "Run", err)
	var buf bytes.Buffer
	assert.MustNoError(t, "PrintDiagnostics", PrintDiagnostics(&buf, result))
	// The caller root reports one marked call (callee.Marked) and
	// one imported package tag (its callee.Marked ancestor).
	assert.Contains(t, "printed diagnostics", buf.String(), "marked call to Marked")
	assert.Contains(t, "printed diagnostics", buf.String(), "package vowdrivertest.example/xpkg/callee")
	assert.NotContains(t, "printed diagnostics", buf.String(), "marked call to Plain")
}

func TestRun_VisibilityFilterDropsUnexportedFunction(t *testing.T) {
	pkgs := loadTestdataPkgs(t, false, "./caller")
	callerRoot := findPkg(t, pkgs, "vowdrivertest.example/xpkg/caller")
	result, err := Run([]*analysis.Analyzer{newMarkAnalyzer()}, pkgs)
	assert.MustNoError(t, "Run", err)
	// Find the caller-side job for markanalyzer.
	callerJob := findJob(t, result, "markanalyzer", callerRoot)
	// helper() was exported as a fact in the callee's table, but
	// it is an unexported non-method function so the visibility
	// filter must have dropped it during inheritance into the
	// caller's table. The Marked fact must survive.
	assert.Equal(t, `caller table holds a fact on "helper"`, hasFuncFact(callerJob, "helper"), false)
	assert.Equal(t, `caller table holds a fact on "Marked"`, hasFuncFact(callerJob, "Marked"), true)
}

func TestRun_ReleasesSyntaxAndNonRootResults(t *testing.T) {
	pkgs := loadTestdataPkgs(t, false, "./root")
	rootPkg := findPkg(t, pkgs, "vowdrivertest.example/xpkg/root")
	// Grab a handle to the non-root dependency package before
	// Run mutates the fields.
	callerPkg := rootPkg.Imports["vowdrivertest.example/xpkg/caller"]
	assert.MustNotNil(t, "caller package among root's imports", callerPkg)
	calleePkg := callerPkg.Imports["vowdrivertest.example/xpkg/callee"]
	assert.MustNotNil(t, "callee package among caller's imports", calleePkg)
	result, err := Run([]*analysis.Analyzer{newMarkAnalyzer()}, []*packages.Package{rootPkg})
	assert.MustNoError(t, "Run", err)
	// Every package (including the root) must have its Syntax
	// and TypesInfo dropped by the end of Run.
	for _, pkg := range []*packages.Package{rootPkg, callerPkg, calleePkg} {
		assert.Nil(t, pkg.PkgPath+" Syntax after Run", pkg.Syntax)
		assert.Nil(t, pkg.PkgPath+" TypesInfo after Run", pkg.TypesInfo)
		assert.NotNil(t, pkg.PkgPath+" Types after Run", pkg.Types)
		assert.NotNil(t, pkg.PkgPath+" Fset after Run", pkg.Fset)
	}
	// Non-root results must be cleared and root results must survive;
	// markanalyzer returns non-nil, so a nil is the driver's doing.
	for _, j := range result.Jobs {
		label := fmt.Sprintf("job %s@%s result after release", j.Analyzer.Name, j.Package.PkgPath)
		if j.Root {
			assert.NotNil(t, label, j.result)
			continue
		}
		assert.Nil(t, label, j.result)
	}
}

func TestRun_DependencyFailurePropagates(t *testing.T) {
	pkgs := loadTestdataPkgs(t, false, "./callee")
	// The failing analyzer errors unconditionally.
	failing := &analysis.Analyzer{
		Name: "failing",
		Doc:  "test analyzer that always errors",
		Run:  func(*analysis.Pass) (any, error) { return nil, errors.New("boom") },
	}
	// The dependent analyzer requires failing; it should never
	// run because its dependency failed.
	dependentRuns := 0
	dependent := &analysis.Analyzer{
		Name:     "dependent",
		Doc:      "test analyzer that requires failing",
		Requires: []*analysis.Analyzer{failing},
		Run: func(*analysis.Pass) (any, error) {
			dependentRuns++
			return nil, nil
		},
	}
	result, err := Run([]*analysis.Analyzer{dependent}, pkgs)
	assert.MustNoError(t, "Run", err)
	assert.Equal(t, "calls to the dependent analyzer's Run", dependentRuns, 0)
	sawFailing, sawDependent := false, false
	for _, j := range result.Jobs {
		switch j.Analyzer.Name {
		case "failing":
			sawFailing = true
			assert.ErrorContains(t, "failing job Err", j.Err, "boom")
		case "dependent":
			sawDependent = true
			assert.ErrorContains(t, "dependent job Err", j.Err, "dependency failed")
		}
	}
	assert.Equal(t, `result holds the "failing" job`, sawFailing, true)
	assert.Equal(t, `result holds the "dependent" job`, sawDependent, true)
	// Even on the failure path the release post-condition must
	// hold: syntax dropped, non-root results dropped, Types kept.
	for _, pkg := range pkgs {
		assert.Nil(t, pkg.PkgPath+" Syntax after a failing Run", pkg.Syntax)
		assert.Nil(t, pkg.PkgPath+" TypesInfo after a failing Run", pkg.TypesInfo)
		assert.NotNil(t, pkg.PkgPath+" Types after a failing Run", pkg.Types)
	}
}

func TestRun_ResultTypeMismatchIsRecorded(t *testing.T) {
	pkgs := loadTestdataPkgs(t, false, "./callee")
	mismatch := &analysis.Analyzer{
		Name:       "mismatch",
		Doc:        "test analyzer that returns the wrong result type",
		Run:        func(*analysis.Pass) (any, error) { return 42, nil },
		ResultType: reflect.TypeOf(""),
	}
	result, err := Run([]*analysis.Analyzer{mismatch}, pkgs)
	assert.MustNoError(t, "Run", err)
	assert.MustLen(t, "result.Jobs", result.Jobs, 1)
	assert.ErrorContains(t, "job Err", result.Jobs[0].Err, "ResultType")
}

func TestRun_ExportAfterRunPanics(t *testing.T) {
	pkgs := loadTestdataPkgs(t, false, "./callee")
	var capturedExport func(types.Object, analysis.Fact)
	var capturedObj types.Object
	capture := &analysis.Analyzer{
		Name:      "capture",
		Doc:       "test analyzer that leaks its export closure",
		FactTypes: []analysis.Fact{(*markFact)(nil)},
		Run: func(pass *analysis.Pass) (any, error) {
			capturedExport = pass.ExportObjectFact
			for _, f := range pass.Files {
				for _, decl := range f.Decls {
					fn, ok := decl.(*ast.FuncDecl)
					if !ok {
						continue
					}
					if obj := pass.TypesInfo.Defs[fn.Name]; obj != nil {
						capturedObj = obj
						return nil, nil
					}
				}
			}
			return nil, nil
		},
	}
	_, err := Run([]*analysis.Analyzer{capture}, pkgs)
	assert.MustNoError(t, "Run", err)
	assert.MustNotNil(t, "export closure the analyzer captured", capturedExport)
	assert.MustNotNil(t, "object the analyzer captured", capturedObj)
	defer func() {
		assert.MustNotNil(t, "recover() from an ExportObjectFact call after Run returned", recover())
	}()
	capturedExport(capturedObj, &markFact{})
}

func TestRun_DedupesTestAugmentedDiagnostics(t *testing.T) {
	pkgs := loadTestdataPkgs(t, true, "./dedup")
	twinReporter := &analysis.Analyzer{
		Name: "twinreporter",
		Doc:  "test analyzer that reports each Twin declaration",
		Run: func(pass *analysis.Pass) (any, error) {
			for _, f := range pass.Files {
				for _, decl := range f.Decls {
					fn, ok := decl.(*ast.FuncDecl)
					if !ok {
						continue
					}
					if fn.Name.Name == "Twin" {
						pass.Reportf(fn.Pos(), "twin found")
					}
				}
			}
			return nil, nil
		},
	}
	result, err := Run([]*analysis.Analyzer{twinReporter}, pkgs)
	assert.MustNoError(t, "Run", err)
	// Under Tests: true the loader produces both the base and
	// the test-augmentation package, so the analyzer sees Twin
	// through more than one root job.
	rootCount := 0
	for _, j := range result.Jobs {
		if j.Root {
			rootCount++
		}
	}
	if rootCount < 2 {
		t.Fatalf("expected at least 2 root jobs under Tests: true; got %d", rootCount)
	}
	var buf bytes.Buffer
	assert.MustNoError(t, "PrintDiagnostics", PrintDiagnostics(&buf, result))
	assert.Equal(t, `printed "twin found" diagnostics after dedup`, strings.Count(buf.String(), "twin found"), 1)
}

func TestRun_PackageFactInheritsAcrossImport(t *testing.T) {
	pkgs := loadTestdataPkgs(t, false, "./caller")
	result, err := Run([]*analysis.Analyzer{newMarkAnalyzer()}, pkgs)
	assert.MustNoError(t, "Run", err)
	var buf bytes.Buffer
	assert.MustNoError(t, "PrintDiagnostics", PrintDiagnostics(&buf, result))
	// The caller pass reports the imported package tag it read
	// through ImportPackageFact.
	assert.Contains(t, "printed diagnostics", buf.String(),
		"package vowdrivertest.example/xpkg/callee tagged as vowdrivertest.example/xpkg/callee")
}

func findPkg(t *testing.T, pkgs []*packages.Package, path string) *packages.Package {
	t.Helper()
	var found *packages.Package
	for _, p := range pkgs {
		if p.PkgPath == path {
			found = p
			break
		}
	}
	assert.MustNotNil(t, fmt.Sprintf("package %s in the load", path), found)
	return found
}

func findJob(t *testing.T, r *Result, analyzerName string, pkg *packages.Package) *Job {
	t.Helper()
	var found *Job
	for _, j := range r.Jobs {
		if j.Analyzer.Name == analyzerName && j.Package == pkg {
			found = j
			break
		}
	}
	assert.MustNotNil(t, fmt.Sprintf("job %s@%s in the result", analyzerName, pkg.PkgPath), found)
	return found
}

// hasFuncFact reports whether job's object-fact table holds an
// entry keyed by a function named name.
func hasFuncFact(job *Job, name string) bool {
	for k := range job.facts.objects {
		if fn, ok := k.obj.(*types.Func); ok && fn.Name() == name {
			return true
		}
	}
	return false
}

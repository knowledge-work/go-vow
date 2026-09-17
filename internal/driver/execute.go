package driver

import (
	"errors"
	"fmt"
	"go/types"
	"os"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

// executeAll walks every root in caller-supplied order and then
// runs releaseAll so the release post-condition holds regardless
// of which jobs took the early-exit branch.
func (g *graph) executeAll() {
	for _, root := range g.roots {
		g.execute(root)
	}
	g.releaseAll()
}

// execute runs one job at most once. Re-entry would signal a
// cycle, which analysis.Validate rejects up front.
func (g *graph) execute(j *Job) {
	if j.completed {
		return
	}
	for _, dep := range j.deps {
		g.execute(dep)
	}
	if failed := failedDeps(j.deps); len(failed) > 0 {
		j.Err = describeDepFailure(failed)
		j.completed = true
		g.completionOrder = append(g.completionOrder, j)
		// Per-job release is skipped here; releaseAll enforces
		// the run-wide post-condition regardless.
		return
	}

	resultOf := gatherResultOf(j)
	inheritFactsFromImports(j)

	pass := g.buildPass(j, resultOf)

	if j.Package.IllTyped && !j.Analyzer.RunDespiteErrors {
		j.Err = fmt.Errorf("skipping analyzer %s: package %s has type errors", j.Analyzer.Name, j.Package.PkgPath)
	} else {
		j.exportOpen = true
		result, runErr := j.Analyzer.Run(pass)
		j.exportOpen = false
		switch {
		case runErr != nil:
			j.Err = runErr
		case j.Analyzer.ResultType != nil && reflect.TypeOf(result) != j.Analyzer.ResultType:
			j.Err = fmt.Errorf("analyzer %s returned result of type %T; declared ResultType is %s",
				j.Analyzer.Name, result, j.Analyzer.ResultType)
		default:
			j.result = result
		}
	}

	j.completed = true
	g.completionOrder = append(g.completionOrder, j)
	// Clear pass.ResultOf so no live entry keeps a released
	// dep result reachable through Pass.
	clear(pass.ResultOf)
	g.release(j)
}

// failedDeps returns dependencies whose Err is set, in deps order
// so the recorded reason stays deterministic.
func failedDeps(deps []*Job) []*Job {
	var failed []*Job
	for _, d := range deps {
		if d.Err != nil {
			failed = append(failed, d)
		}
	}
	return failed
}

// describeDepFailure returns an error listing every failed
// dependency as "analyzer@package". deps must be in build order so
// the string is deterministic.
func describeDepFailure(failed []*Job) error {
	names := make([]string, len(failed))
	for i, d := range failed {
		names[i] = d.Analyzer.Name + "@" + d.Package.PkgPath
	}
	return errors.New("dependency failed: " + strings.Join(names, ", "))
}

// gatherResultOf builds the ResultOf table from same-package
// dependencies. Import-side fact edges do not surface here.
func gatherResultOf(j *Job) map[*analysis.Analyzer]any {
	result := map[*analysis.Analyzer]any{}
	for _, dep := range j.deps {
		if dep.Package == j.Package {
			result[dep.Analyzer] = dep.result
		}
	}
	return result
}

// inheritFactsFromImports merges every same-analyzer import
// dependency's fact table into j.facts, filtering object facts
// through the visibility rule.
func inheritFactsFromImports(j *Job) {
	for _, dep := range j.deps {
		if dep.Package == j.Package {
			continue
		}
		if dep.Analyzer != j.Analyzer {
			continue
		}
		j.facts.inheritFrom(dep.facts, dep.Package.Types)
	}
}

// buildPass populates every field on analysis.Pass so a Requires
// analyzer that reads any standard field sees a fully populated
// value.
func (g *graph) buildPass(j *Job, resultOf map[*analysis.Analyzer]any) *analysis.Pass {
	pkg := j.Package
	pass := &analysis.Pass{
		Analyzer:     j.Analyzer,
		Fset:         pkg.Fset,
		Files:        pkg.Syntax,
		OtherFiles:   pkg.OtherFiles,
		IgnoredFiles: pkg.IgnoredFiles,
		Pkg:          pkg.Types,
		TypesInfo:    pkg.TypesInfo,
		TypesSizes:   pkg.TypesSizes,
		TypeErrors:   pkg.TypeErrors,
		Module:       moduleOf(pkg),
		ResultOf:     resultOf,
		ReadFile:     func(name string) ([]byte, error) { return os.ReadFile(name) },
	}
	pass.Report = func(d analysis.Diagnostic) {
		j.Diagnostics = append(j.Diagnostics, d)
	}
	pass.ImportObjectFact = func(obj types.Object, ptr analysis.Fact) bool {
		return j.facts.importObject(obj, ptr)
	}
	pass.ImportPackageFact = func(target *types.Package, ptr analysis.Fact) bool {
		return j.facts.importPackage(target, ptr)
	}
	pass.ExportObjectFact = func(obj types.Object, fact analysis.Fact) {
		if !j.exportOpen {
			panic("driver: ExportObjectFact called after Run returned")
		}
		j.facts.exportObject(pkg.Types, obj, fact)
	}
	pass.ExportPackageFact = func(fact analysis.Fact) {
		if !j.exportOpen {
			panic("driver: ExportPackageFact called after Run returned")
		}
		j.facts.exportPackage(pkg.Types, fact)
	}
	pass.AllObjectFacts = func() []analysis.ObjectFact {
		panic("driver: Pass.AllObjectFacts is not supported (analyzers must inspect facts through ImportObjectFact)")
	}
	pass.AllPackageFacts = func() []analysis.PackageFact {
		panic("driver: Pass.AllPackageFacts is not supported (analyzers must inspect facts through ImportPackageFact)")
	}
	return pass
}

// moduleOf returns analysis.Module for the package's enclosing
// module, always non-nil (empty struct when the package has no
// module) so analyzers may dereference the pointer.
func moduleOf(pkg *packages.Package) *analysis.Module {
	if pkg.Module == nil {
		return &analysis.Module{}
	}
	return &analysis.Module{
		Path:      pkg.Module.Path,
		Version:   pkg.Module.Version,
		GoVersion: pkg.Module.GoVersion,
	}
}

// release decrements the two remaining-reader counters for the
// just-finished job. A dependency whose consumer count hits zero
// has its result nilled unless it is a root; a package whose
// remaining-job count hits zero has its Syntax and TypesInfo
// nilled.
func (g *graph) release(j *Job) {
	for _, dep := range j.deps {
		dep.consumers--
		if dep.consumers == 0 && !dep.Root {
			dep.result = nil
		}
	}
	// Drop the reference from j back to its deps so a released
	// dep's Job payload does not remain reachable through j.deps.
	j.deps = nil

	if state := g.pkgs[j.Package]; state != nil {
		state.remaining--
		if state.remaining == 0 {
			releasePackageSyntax(j.Package)
		}
	}
}

// releaseAll runs at the end of Run so the release post-condition
// holds even when jobs took the early-exit path. It nils every
// non-root Job.result and every package's Syntax and TypesInfo.
// Fact tables, package.Types, package.Fset, diagnostics, and root
// results survive.
func (g *graph) releaseAll() {
	for _, j := range g.jobs {
		if !j.Root {
			j.result = nil
		}
		j.deps = nil
	}
	for _, state := range g.pkgs {
		releasePackageSyntax(state.pkg)
	}
}

// releasePackageSyntax nils Syntax and TypesInfo, the two fields
// that dominate a package's live heap.
func releasePackageSyntax(pkg *packages.Package) {
	pkg.Syntax = nil
	pkg.TypesInfo = nil
}

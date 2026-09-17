package driver

import (
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

// nodeKey identifies one work unit by its analyzer and package.
type nodeKey struct {
	analyzer *analysis.Analyzer
	pkg      *packages.Package
}

// pkgState tracks how many jobs anchored to a package have not yet
// completed. On zero, the package's Syntax and TypesInfo are
// releasable.
type pkgState struct {
	pkg       *packages.Package
	remaining int
}

// graph is the closed set of jobs a Run instance schedules over.
// It memoizes jobs by (analyzer, package) and records their
// completion order.
type graph struct {
	jobs map[nodeKey]*Job
	pkgs map[*packages.Package]*pkgState
	// roots is the direct product of the caller-supplied
	// analyzers and packages, in the order the caller passed
	// them (packages outer, analyzers inner).
	roots []*Job
	// completionOrder records jobs as they finish. Because deps
	// finish before their consumer's execute returns, the slice
	// is postorder.
	completionOrder []*Job
}

// buildGraph materialises every job reachable from the root set
// and seeds the two remaining-reader counters release consults
// (per-job unfinished consumers, per-package unfinished jobs).
func buildGraph(analyzers []*analysis.Analyzer, pkgs []*packages.Package) *graph {
	g := &graph{
		jobs: map[nodeKey]*Job{},
		pkgs: map[*packages.Package]*pkgState{},
	}
	for _, pkg := range pkgs {
		for _, a := range analyzers {
			root := g.materialize(a, pkg)
			root.Root = true
			g.roots = append(g.roots, root)
		}
	}
	return g
}

// materialize returns the Job for (a, pkg), creating it and its
// dependencies on first request. Each fresh edge bumps the dep's
// consumer count.
func (g *graph) materialize(a *analysis.Analyzer, pkg *packages.Package) *Job {
	key := nodeKey{a, pkg}
	if existing, ok := g.jobs[key]; ok {
		return existing
	}
	j := &Job{
		Analyzer: a,
		Package:  pkg,
		facts:    newFactTable(),
	}
	g.jobs[key] = j
	if state, ok := g.pkgs[pkg]; ok {
		state.remaining++
	} else {
		g.pkgs[pkg] = &pkgState{pkg: pkg, remaining: 1}
	}

	// Same-package Requires feed this job's ResultOf.
	for _, req := range a.Requires {
		dep := g.materialize(req, pkg)
		j.deps = append(j.deps, dep)
		dep.consumers++
	}
	// Import edges exist only for FactTypes analyzers, which
	// inherit facts from their imports.
	if len(a.FactTypes) > 0 {
		for _, importPath := range sortedImportPaths(pkg.Imports) {
			imported := pkg.Imports[importPath]
			// A dependency loaded without syntax cannot run an
			// analyzer, and with no source in scope it holds no
			// markers, so no fact can originate there — the edge
			// would only schedule work that produces nothing. The
			// caller decides which packages carry syntax (a scoped
			// load leaves it off third-party dependencies); under
			// a whole-closure load every package has syntax and
			// this branch never fires. Skipping the edge severs
			// no fact chain either, as long as
			// config.AnalysisScope.FirstPartyPrefixes covers
			// every first-party package: facts travel along
			// import edges, so a syntax-less node could only
			// relay them by importing a fact-exporting package —
			// and a fact-exporting package is first-party code,
			// which nothing outside the scope imports (first-party
			// may depend on third-party, never the reverse). Miss
			// a first-party package in the prefixes and it lands
			// outside the scope while still importing first-party
			// code, so a fact chain through it breaks here.
			if len(imported.Syntax) == 0 {
				continue
			}
			dep := g.materialize(a, imported)
			j.deps = append(j.deps, dep)
			dep.consumers++
		}
	}
	return j
}

// sortedImportPaths returns pkg.Imports keys in ascending order so
// the graph shape stays deterministic across runs.
func sortedImportPaths(imports map[string]*packages.Package) []string {
	keys := make([]string, 0, len(imports))
	for k := range imports {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

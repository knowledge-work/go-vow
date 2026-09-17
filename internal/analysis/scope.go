package analysis

import (
	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// stdPresets is the registry of rule-only presets that ship inside
// the analyzer binary. The map key is the import path users write in
// a `vow:import <alias> "<path>"` annotation. Initialized once at
// package load — the underlying preset values are never mutated, so
// it is safe to share across passes.
var stdPresets = builtinStdPresets()

// scopeFromPackage builds a RuleScope from the `vow:import`
// annotations on every file's package doc comment. The scope spans
// the package because a package is conventionally given a single doc
// comment — godoclint and friends enforce it — so a per-file scope
// leaves the siblings with nowhere to spell the import.
//
// An alias bound twice keeps its first binding in pass.Files order,
// which the driver fixes, so the winner is the same on every run.
// Rebinding it to the same path is quiet; to a different path it is
// reported, since a package-wide scope can honour only one.
func scopeFromPackage(pass *analysis.Pass) dsl.RuleScope {
	aliases := map[string]map[string]dsl.RuleDef{}
	boundTo := map[string]string{}
	for _, file := range pass.Files {
		if file.Doc == nil {
			continue
		}
		for _, u := range parseImportAnnotations(file.Doc) {
			p, ok := stdPresets[u.path]
			if !ok {
				vowReportf(
					pass,
					u.pos,
					"vow[sentinel-error]: vow:import alias %q references unknown preset path %q",
					u.alias, u.path,
				)
				continue
			}
			if prev, bound := boundTo[u.alias]; bound {
				if prev != u.path {
					vowReportf(
						pass,
						u.pos,
						"vow[sentinel-error]: vow:import alias %q is already bound to %q in this package; keeping the first binding",
						u.alias, prev,
					)
				}
				continue
			}
			boundTo[u.alias] = u.path
			aliases[u.alias] = p.Rules
		}
	}
	return dsl.RuleScope{Aliases: aliases}
}

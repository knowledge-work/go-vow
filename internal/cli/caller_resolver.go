package cli

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

// ScopePackages groups the two analyzer-scope inputs the
// `--with-callers` driver flag computes: the import paths of the
// packages that own the changed files (Changed) and the import
// paths of the 1-hop importers that mention an identifier declared
// in the changed files (Callers). Both slices are sorted and contain
// unique entries; Callers additionally excludes any package
// already in Changed, so the two slices form a partition of the
// scope set. The caller-side analyzer pass needs both lists so
// the analyzer's lint scope can be narrowed to {Changed ∪ Callers}
// instead of the whole module.
type ScopePackages struct {
	Changed []string
	Callers []string
}

// ResolveScopePackages resolves the analyzer-scope set in two
// phases, neither of which type-checks anything.
//
//	Phase 1 — module-wide lightweight load. Only package names,
//	  file paths, and the import graph are needed to discover the
//	  changed packages and the upper bound of caller candidates.
//	  No AST, no types, no SSA.
//	Phase 2 — declared-identifier text match. The changed files are
//	  parsed (syntax only) for the identifiers they declare, and a
//	  candidate package stays in the caller set iff one of its files
//	  mentions one of those identifiers as a whole word.
//
// The text match over-approximates: a mention inside a comment, a
// string, or an unrelated same-named identifier keeps the package.
// That is deliberate — callers are an analysis scope, so a false
// positive only costs the checker one extra package, while a miss
// would silence a diagnostic. Direct references cannot be missed:
// every Go reference to a declared identifier spells that identifier
// in the referencing file (dot imports included), so a package with
// no textual mention holds no direct reference. Indirect references
// that never spell a changed-file name — promoted members reached
// through an embedded type, or function/interface values received
// from a third package — are out of the resolver's scope, as they
// are for the import graph's 1-hop bound itself.
//
// The synthetic packages `Tests: true` materializes (`pkg.test`
// binaries and the bracketed `pkg [pkg.test]` / `pkg_test
// [pkg.test]` variants) are removed structurally: only paths that
// Phase 1 observed as pattern-loadable source packages survive
// into the returned slices. go test
// identifies tests by file suffix and function signature — never
// by import path — so no path-string heuristic can distinguish a
// synthetic test package from a real package whose import path
// happens to end in `_test`; the identity check is the only
// reliable discriminator.
//
// excludePackageSuffixes lists import-path suffixes the driver
// additionally removes as an adopter-intent knob (e.g. generated
// mock packages). The driver supplies the list via the second
// argument so adopters can set it through the
// `caller_resolver.exclude_package_suffixes` knob in their
// vow.yaml config; passing nil keeps every loadable candidate.
//
// A nil return signals that the input has no resolvable changed
// files; the driver treats that as a no-op.
func ResolveScopePackages(changedFiles []string, excludePackageSuffixes []string) (*ScopePackages, error) {
	if len(changedFiles) == 0 {
		return nil, nil
	}
	timing := NewTimingLogger()
	overall := time.Now()
	defer timing.LogPhase("ResolveScopePackages.total", overall)

	phase1Cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedImports,
		Tests: true,
	}
	phase1Start := time.Now()
	phase1Pkgs, err := packages.Load(phase1Cfg, "./...")
	timing.LogPhase("phase1.packages.Load", phase1Start)
	if err != nil {
		return nil, fmt.Errorf("load packages (phase 1): %w", err)
	}
	if errCount := countLoadErrors(phase1Pkgs); errCount > 0 {
		return nil, fmt.Errorf("load packages (phase 1): %d load error(s) prevent complete caller resolution", errCount)
	}

	changedStart := time.Now()
	changedPkgs, changedGoFiles := changedPackages(phase1Pkgs, changedFiles)
	timing.LogPhase("changedPackages", changedStart)
	// The same nil-result path as the top-of-function guard, but only
	// reachable after the lightweight Phase 1 load mapped the files to
	// packages — the two early returns cannot be consolidated because
	// this one needs Phase 1 to compute changedPkgs.
	if len(changedPkgs) == 0 {
		return nil, nil
	}

	// Restrict every outgoing path set to the loadable allowlist
	// before applying the adopter's suffix knob: the downstream
	// driver hands {Changed, Callers} to the checker as positional
	// args, and a synthetic entry the loader rejects (a `pkg.test`
	// binary or a `pkg_test` external-test variant) would abort the
	// run the resolver exists to narrow.
	loadable := loadablePackagePaths(phase1Pkgs)
	filteredChanged := filterBySuffix(filterLoadable(sortedKeys(changedPkgs), loadable), excludePackageSuffixes)

	reverseStart := time.Now()
	candidateCallers := reverseImportTraversal(phase1Pkgs, changedPkgs)
	timing.LogPhase("reverseImportTraversal", reverseStart)
	if len(candidateCallers) == 0 {
		return &ScopePackages{
			Changed: filteredChanged,
			Callers: sortedKeys(nil),
		}, nil
	}

	identsStart := time.Now()
	idents, err := declaredIdents(changedGoFiles)
	timing.LogPhase("phase2.declaredIdents", identsStart)
	if err != nil {
		return nil, fmt.Errorf("parse changed files: %w", err)
	}
	if len(idents) == 0 {
		// A file that declares nothing referenceable (e.g. only a
		// package clause) cannot have callers.
		return &ScopePackages{
			Changed: filteredChanged,
			Callers: sortedKeys(nil),
		}, nil
	}

	matchStart := time.Now()
	callers := make(map[string]struct{})
	// Test-augmented variants of a package (surfaced by Tests: true
	// under the same PkgPath) repeat the base compilation's files, so
	// scan results are memoized per file path.
	scanned := make(map[string]bool)
	for _, pkg := range phase1Pkgs {
		if _, isCandidate := candidateCallers[pkg.PkgPath]; !isCandidate {
			continue
		}
		if _, matched := callers[pkg.PkgPath]; matched {
			continue
		}
		for _, file := range pkg.CompiledGoFiles {
			mentions, seen := scanned[file]
			if !seen {
				var err error
				mentions, err = fileMentionsAny(file, idents)
				if err != nil {
					return nil, fmt.Errorf("scan candidate files: %w", err)
				}
				scanned[file] = mentions
			}
			if mentions {
				callers[pkg.PkgPath] = struct{}{}
				break
			}
		}
	}
	timing.LogPhase("phase2.identTextMatch", matchStart)

	return &ScopePackages{
		Changed: filteredChanged,
		Callers: filterBySuffix(filterLoadable(sortedKeys(callers), loadable), excludePackageSuffixes),
	}, nil
}

// loadablePackagePaths returns the set of import paths that Phase 1
// observed as real, pattern-loadable source packages. Under
// `Tests: true` the loader also materializes synthetic packages
// that `packages.Load` rejects as positional patterns, and each
// synthetic shape fails one of the two structural checks:
//
//   - bracketed variants (`pkg [pkg.test]`, `pkg_test [pkg.test]`)
//     carry an ID distinct from their PkgPath;
//   - the `pkg.test` binary carries no compiled Go files in the
//     Phase 1 metadata-only mode.
//
// A real package whose import path merely ends in `_test` (a legal
// directory name) passes both checks and stays loadable — path
// spelling never enters the decision.
func loadablePackagePaths(pkgs []*packages.Package) map[string]struct{} {
	loadable := make(map[string]struct{}, len(pkgs))
	for _, pkg := range pkgs {
		if pkg.ID != pkg.PkgPath || len(pkg.CompiledGoFiles) == 0 {
			continue
		}
		loadable[pkg.PkgPath] = struct{}{}
	}
	return loadable
}

// filterLoadable returns the paths present in the loadable set,
// preserving input order. The resolver applies it to every path
// slice that leaves the function so synthetic test packages never
// reach the driver's positional args regardless of the adopter's
// suffix configuration.
func filterLoadable(paths []string, loadable map[string]struct{}) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := loadable[p]; !ok {
			continue
		}
		out = append(out, p)
	}
	return out
}

// changedPackages returns the import paths whose CompiledGoFiles
// contain any of changedFiles, together with the subset of
// changedFiles that Phase 1 recognises as Go sources. Paths without
// an owning package (deleted files, go.mod, docs) drop out of both
// results, so downstream parsing only ever sees the same file set
// the scope computation is based on.
func changedPackages(pkgs []*packages.Package, changedFiles []string) (map[string]struct{}, []string) {
	fileOwner := make(map[string]*packages.Package)
	for _, pkg := range pkgs {
		for _, src := range pkg.CompiledGoFiles {
			fileOwner[src] = pkg
		}
	}

	owned := make(map[string]struct{})
	ownedFiles := make([]string, 0, len(changedFiles))
	for _, file := range changedFiles {
		pkg, ok := fileOwner[file]
		if !ok {
			continue
		}
		owned[pkg.PkgPath] = struct{}{}
		ownedFiles = append(ownedFiles, file)
	}
	return owned, ownedFiles
}

// reverseImportTraversal returns the import paths of every package
// that imports at least one of changedPkgs. Go disallows unused
// imports, so an importing package references at least one
// identifier from the imported package; the result is an upper
// bound on the callers, which the Phase 2 text match narrows to the
// packages that mention an identifier declared in the changed files
// themselves. Packages already in changedPkgs are excluded so the
// candidate set forms a partition with the changed set. The walk
// visits every entry in pkgs — including the synthetic `[*.test]`
// variants surfaced by `Tests: true` — so an in-package test-only
// caller is captured through its variant package.
func reverseImportTraversal(pkgs []*packages.Package, changedPkgs map[string]struct{}) map[string]struct{} {
	candidates := make(map[string]struct{})
	for _, pkg := range pkgs {
		if _, owned := changedPkgs[pkg.PkgPath]; owned {
			continue
		}
		for impPath := range pkg.Imports {
			if _, owned := changedPkgs[impPath]; owned {
				candidates[pkg.PkgPath] = struct{}{}
				break
			}
		}
	}
	return candidates
}

// countLoadErrors sums the per-package load errors recorded by
// packages.Load; the loader returns nil even when individual packages
// fail to load, so callers consult this count to detect incomplete loads.
func countLoadErrors(pkgs []*packages.Package) int {
	var n int
	for _, pkg := range pkgs {
		n += len(pkg.Errors)
	}
	return n
}

// filterBySuffix returns paths whose suffix does not match any of
// the supplied exclusion suffixes. An empty exclusion list returns
// the input unchanged so disabling the filter is observable in the
// caller's input rather than buried in a sentinel.
func filterBySuffix(paths []string, exclude []string) []string {
	if len(exclude) == 0 {
		return paths
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if matchesAnySuffix(p, exclude) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// matchesAnySuffix reports whether path ends with any of the
// supplied suffixes. The check is a string-suffix compare so
// adopters supply a literal suffix such as `_mock` rather than a
// regex; synthetic test packages are already removed structurally
// before this knob applies. An empty suffix is ignored rather than
// matched: HasSuffix reports true for it against every path, so one
// empty entry would exclude everything.
func matchesAnySuffix(path string, suffixes []string) bool {
	return slices.ContainsFunc(suffixes, func(suffix string) bool {
		return suffix != "" && strings.HasSuffix(path, suffix)
	})
}

// sortedKeys returns the keys of a `map[string]struct{}` set as a
// sorted slice; the driver hands the result to the analyzer as a
// stable, deduplicated import-path list.
func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

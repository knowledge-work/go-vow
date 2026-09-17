// Command vow runs the vow linter on the supplied Go packages.
//
// Usage:
//
//	vow [--changed-files file1.go file2.go ...] [--with-callers]
//	    [--config-file path | --config-yaml content] [-test] <package>...
//
// Without driver flags, vow lints every non-test Go file in the named
// packages; -test extends the load to include _test.go files. With
// --changed-files, vow restricts self-validation to the listed paths; an
// empty expansion (for instance when `git diff` returns nothing) collapses to
// the package-level lint. --with-callers extends the run with caller-side
// checks and requires --changed-files; standalone use is rejected with exit
// code 2. Under --with-callers, vow computes the scope as the changed
// packages together with their 1-hop callers and **replaces** any positional
// package arguments with that scope so the go/analysis driver does not
// re-load the whole module — supplying `./...` (or any other positional
// package set) alongside --with-callers is therefore a no-op on the
// positional side. --config-file points the analyzer at a single vow.yaml file and
// --config-yaml passes the same yaml content inline; either one overrides
// the per-directory discovery walk for the entire run. The two flags are
// mutually exclusive — adopters who want repo-tracked policy choose
// --config-file, and adopters who want an ad-hoc override (e.g. a CI step
// that opts a project out for one job) choose --config-yaml.
//
// Caller-resolver knobs (`caller_resolver.*` in the config) are
// consulted only through `--config-file` / `--config-yaml`;
// per-directory `vow.yaml` files do not propagate into the resolver
// because the resolver runs once per driver invocation rather than
// once per package.
//
// The analyzer implementation lives in internal/analysis.
package main

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	vowanalysis "github.com/knowledge-work/go-vow/internal/analysis"
	"github.com/knowledge-work/go-vow/internal/cli"
	"github.com/knowledge-work/go-vow/internal/config"
	"github.com/knowledge-work/go-vow/internal/driver"
)

// driverFlags carries every value parseDriverArgs extracts from the
// vow-side of the command line. Grouping the fields in a struct keeps
// the function signature stable as new driver flags land.
type driverFlags struct {
	Changed      cli.ChangedFileSet
	ConfigFile   string
	ConfigYAML   string
	IncludeTests bool
	VetMode      bool
}

func main() {
	if runsAsVettool(os.Args[1:], os.Getenv(vetToolEnv) != "") {
		runVettool()
		return
	}
	timing := cli.NewTimingLogger()
	parseStart := time.Now()
	flags, remaining, err := parseDriverArgs(os.Args[1:])
	timing.LogPhase("parseDriverArgs", parseStart)
	if err != nil {
		// Write directly through *os.File to keep the Closable obligation on
		// stderr rather than passing it as an io.Writer.
		os.Stderr.WriteString("vow: " + err.Error() + "\n")
		os.Exit(2)
	}
	if len(flags.Changed.Files) > 0 {
		validateStart := time.Now()
		absFiles, err := cli.ValidateChangedFiles(flags.Changed.Files)
		timing.LogPhase("ValidateChangedFiles", validateStart)
		if err != nil {
			os.Stderr.WriteString("vow: --changed-files: " + err.Error() + "\n")
			os.Exit(2)
		}
		flags.Changed.Files = absFiles
	}
	configStart := time.Now()
	override, err := loadConfigOverride(flags)
	timing.LogPhase("loadConfigOverride", configStart)
	if err != nil {
		os.Stderr.WriteString("vow: " + err.Error() + "\n")
		os.Exit(2)
	}
	if flags.Changed.WithCallers && len(flags.Changed.Files) > 0 {
		excludeSuffixes := callerResolverExcludeSuffixes(override)
		resolveStart := time.Now()
		scope, err := cli.ResolveScopePackages(flags.Changed.Files, excludeSuffixes)
		timing.LogPhase("ResolveScopePackages.outer", resolveStart)
		if err != nil {
			os.Stderr.WriteString("vow: resolve callers: " + err.Error() + "\n")
			os.Exit(2)
		}
		if scope != nil {
			flags.Changed.CallerPackages = scope.Callers
			flags.Changed.ChangedPackages = scope.Changed
		}
		// Narrow the analyzer's lint target to the changed packages
		// plus their 1-hop callers so the go/analysis driver loads
		// only the subset relevant to the run; the whole-module
		// `./...` default would re-load every package the resolver
		// already inspected. This override touches the positional
		// args only — cli.Active() (consumed by the per-call-site
		// narrow_filter) continues to publish the same Files /
		// CallerPackages shape regardless of the positional narrowing.
		narrow := scopePositionals(scope)
		if len(narrow) == 0 {
			// The named files resolved to no lintable package, so the
			// run has nothing to check. Falling through would leave
			// the positional args in place — or hand an empty set to
			// the whole-module fallback below — and widen the run to
			// every package in the module, which is the opposite of
			// what --changed-files asked for and the most expensive
			// shape the driver has. A diff confined to _test.go files
			// lands here whenever the run excludes test files, which
			// is the default.
			os.Stderr.WriteString(fmt.Sprintf(
				"vow: --changed-files (%d file(s)) resolved to no lintable package; nothing to lint\n",
				len(flags.Changed.Files)))
			os.Exit(0)
		}
		remaining = narrow
	}
	cli.SetActive(flags.Changed)
	if len(remaining) == 0 {
		// Match singlechecker's whole-module fallback so an invocation
		// without positional packages keeps linting the current module.
		remaining = []string{"./..."}
	}
	if flags.VetMode {
		// The fallback above has to run first: `go vet` with no pattern
		// analyzes the current directory alone, where a vow run covers
		// the module.
		if flags.ConfigFile == "" && flags.ConfigYAML == "" {
			os.Stderr.WriteString("vow: --vet-mode requires --config-file or --config-yaml; the per-directory vow.yaml walk cannot run under it\n")
			os.Exit(2)
		}
		os.Exit(runVetPipeline(vetConfigYAML(flags), flags.Changed, flags.IncludeTests, remaining))
	}
	timing.LogEntry("PHASE=driver.Run.entry remaining=%v", remaining)

	loadStart := time.Now()
	// Under a first-party scope, roots carries only the packages the
	// patterns named — the packages whose diagnostics print — while
	// loaded additionally carries the in-scope dependencies whose
	// syntax participates for fact propagation. Without the knob the
	// two sets coincide.
	var roots, loaded []*packages.Package
	if prefixes := analysisScopeFirstPartyPrefixes(override); len(prefixes) > 0 {
		roots, loaded, err = loadScopedPackages("", flags.IncludeTests, remaining, prefixes)
	} else {
		roots, err = loadTargetPackages("", flags.IncludeTests, remaining)
		loaded = roots
	}
	timing.LogPhase("packages.Load", loadStart)
	if err != nil {
		os.Stderr.WriteString("vow: load packages: " + err.Error() + "\n")
		os.Exit(2)
	}
	if len(roots) == 0 {
		os.Stderr.WriteString("vow: " + strings.Join(remaining, " ") + " matched no packages\n")
		os.Exit(2)
	}
	loadErrCount := packages.PrintErrors(loaded)

	analyzeStart := time.Now()
	result, err := driver.Run(
		[]*analysis.Analyzer{vowanalysis.NewDefault(override)},
		roots,
	)
	timing.LogPhase("driver.Run", analyzeStart)
	if err != nil {
		os.Stderr.WriteString("vow: analyze: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Buffer diagnostics so stderr keeps its Closable obligation
	// on *os.File instead of being exposed as an io.Writer.
	var diagBuf bytes.Buffer
	if err := driver.PrintDiagnostics(&diagBuf, result); err != nil {
		os.Stderr.WriteString("vow: print diagnostics: " + err.Error() + "\n")
		os.Exit(1)
	}
	os.Stderr.Write(diagBuf.Bytes())

	os.Exit(resolveExitCode(result, loadErrCount))
}

// scopePositionals flattens a resolved --with-callers scope into the
// positional package set the driver lints. A nil scope — the
// resolver's signal that the changed files map to no package it can
// load — flattens to the empty set, which is the same answer as a
// scope whose two halves are both empty, so the caller reads one
// condition instead of two.
//
// vow:nil (?) ?
func scopePositionals(scope *cli.ScopePackages) []string {
	if scope == nil {
		return nil
	}
	total := len(scope.Changed) + len(scope.Callers)
	if total == 0 {
		return nil
	}
	out := make([]string, 0, total)
	out = append(out, scope.Changed...)
	out = append(out, scope.Callers...)
	return out
}

// loadTargetPackages invokes packages.Load with the driver's fixed
// mode set. Test files participate in the load only when includeTests
// is true; the default excludes them. dir selects the working
// directory the loader resolves patterns against; an empty string
// defers to the process cwd.
//
// vow:nil (,,?) ?,
func loadTargetPackages(dir string, includeTests bool, patterns []string) ([]*packages.Package, error) {
	return packages.Load(&packages.Config{
		// NeedModule keeps pass.Module non-nil for analyzers
		// that read module metadata.
		Mode:  packages.LoadAllSyntax | packages.NeedModule,
		Tests: includeTests,
		Dir:   dir,
	}, patterns...)
}

// analysisScopeFirstPartyPrefixes returns the import-path prefixes
// the adopter declared as the only places vow markers may appear.
// The knob rides on `--config-file` / `--config-yaml` like the
// caller-resolver section; per-directory vow.yaml files cannot
// reach it because the load happens once per invocation. An empty
// return keeps the whole-closure load.
//
// vow:nil (?) ?
func analysisScopeFirstPartyPrefixes(override *config.Config) []string {
	if override == nil {
		return nil
	}
	return override.AnalysisScope.FirstPartyPrefixes
}

// scopedLoadMode loads syntax and full type information for the
// packages the patterns name, while their dependencies contribute
// types from export data only. It is LoadAllSyntax minus NeedDeps,
// spelled out because the meaning lives in the omission.
const scopedLoadMode = packages.NeedName | packages.NeedFiles |
	packages.NeedCompiledGoFiles | packages.NeedImports |
	packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
	packages.NeedTypesSizes | packages.NeedModule

// loadScopedPackages loads patterns for analysis under a
// first-party scope. A metadata-only pass walks the dependency
// closure of the patterns and promotes every dependency matching a
// scope prefix to a load pattern of its own, so those packages —
// the only dependencies that can carry vow markers — arrive with
// syntax and get analyzer jobs for fact propagation. Everything
// else is left to export data. roots carries the packages the
// original patterns matched (the diagnostics that print); loaded
// additionally carries the promoted dependencies.
//
// vow:nil (,,?,?) ?,?,
func loadScopedPackages(dir string, includeTests bool, patterns, firstPartyPrefixes []string) (roots, loaded []*packages.Package, err error) {
	timing := cli.NewTimingLogger()
	metaStart := time.Now()
	meta, err := packages.Load(&packages.Config{
		Mode:  packages.NeedName | packages.NeedImports | packages.NeedDeps,
		Tests: includeTests,
		Dir:   dir,
	}, patterns...)
	timing.LogPhase("scopeMetadata.packages.Load", metaStart)
	if err != nil {
		return nil, nil, fmt.Errorf("load packages (scope metadata): %w", err)
	}
	rootPaths := map[string]bool{}
	for _, pkg := range meta {
		rootPaths[pkg.PkgPath] = true
	}
	extras := scopeExtraPatterns(meta, rootPaths, firstPartyPrefixes)
	timing.LogEntry("PHASE=scopeExtras count=%d", len(extras))

	full, err := packages.Load(&packages.Config{
		Mode:  scopedLoadMode,
		Tests: includeTests,
		Dir:   dir,
	}, append(append([]string{}, patterns...), extras...)...)
	if err != nil {
		return nil, nil, err
	}
	for _, pkg := range full {
		if rootPaths[pkg.PkgPath] {
			roots = append(roots, pkg)
		}
	}
	return roots, full, nil
}

// scopeExtraPatterns walks the metadata closure and returns the
// import paths of every dependency that matches a scope prefix and
// is not already a root, sorted for a deterministic load. The
// prefix skips the roots themselves — they are already patterns —
// and, structurally, everything outside the prefixes, which is the
// set the scoped load exists to leave unparsed.
//
// vow:nil (?,?,?) ?
func scopeExtraPatterns(meta []*packages.Package, rootPaths map[string]bool, prefixes []string) []string {
	seen := map[string]bool{}
	packages.Visit(meta, nil, func(pkg *packages.Package) {
		if pkg.PkgPath == "" || rootPaths[pkg.PkgPath] || seen[pkg.PkgPath] {
			return
		}
		if matchesScopePrefix(pkg.PkgPath, prefixes) {
			seen[pkg.PkgPath] = true
		}
	})
	extras := make([]string, 0, len(seen))
	for path := range seen {
		extras = append(extras, path)
	}
	slices.Sort(extras)
	return extras
}

// matchesScopePrefix reports whether path falls under any of the
// prefixes on import-path boundaries: `example.com/foo` covers
// `example.com/foo` and `example.com/foo/...`, never
// `example.com/foobar`.
//
// vow:nil (,?)
func matchesScopePrefix(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		prefix = strings.TrimSuffix(prefix, "/")
		if prefix == "" {
			continue
		}
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// resolveExitCode maps a run's outcome onto the three-tier
// convention: 1 when any load or analyzer step failed, 3 when a
// clean run surfaced at least one root diagnostic, 0 otherwise.
// Only root diagnostics count; counting runs before deduplication
// so a diagnostic reported through two roots contributes once per
// root.
//
// vow:nil (!,)
func resolveExitCode(r *driver.Result, loadErrCount int) int {
	numErrors := loadErrCount
	rootDiags := 0
	for j := range r.All() {
		if j.Err != nil {
			numErrors++
		}
		if j.Root {
			rootDiags += len(j.Diagnostics)
		}
	}
	switch {
	case numErrors > 0:
		return 1
	case rootDiags > 0:
		return 3
	default:
		return 0
	}
}

// callerResolverExcludeSuffixes returns the import-path suffixes
// the resolver drops from its scope on top of the structural
// removal of synthetic test packages. The override (loaded from
// `--config-file` / `--config-yaml`) wins when its
// `caller_resolver.exclude_package_suffixes` field is set
// (including the empty slice); otherwise the built-in default
// (empty) applies, since loader-rejected test-package paths are
// filtered by identity rather than by suffix.
//
// vow:nil (?) ?
func callerResolverExcludeSuffixes(override *config.Config) []string {
	if override != nil && override.CallerResolver.ExcludePackageSuffixes != nil {
		return *override.CallerResolver.ExcludePackageSuffixes
	}
	return config.DefaultExcludePackageSuffixes()
}

// loadConfigOverride resolves the optional --config-file / --config-yaml
// flag into a parsed Config. It returns nil when neither flag is set.
//
// vow:nil () ?,
func loadConfigOverride(flags driverFlags) (*config.Config, error) {
	switch {
	case flags.ConfigFile != "":
		cfg, err := config.ReadFile(flags.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("--config-file: %w", err)
		}
		return cfg, nil
	case flags.ConfigYAML != "":
		cfg, err := config.Parse([]byte(flags.ConfigYAML))
		if err != nil {
			return nil, fmt.Errorf("--config-yaml: %w", err)
		}
		return cfg, nil
	}
	return nil, nil
}

// parseDriverArgs separates vow driver flags from args. It returns the
// extracted driverFlags (--changed-files / --with-callers /
// --config-file / --config-yaml / -test) and the residual tokens
// forwarded to the go/analysis driver. --changed-files collects variadic
// paths up to the next dash-prefixed token (including "--") or the end
// of args. --config-file and --config-yaml are mutually exclusive;
// specifying both is an error. -test (or --test) opts test files into
// the analysis; without it, the loader excludes them.
//
// vow:nil (?) ,?,
func parseDriverArgs(args []string) (driverFlags, []string, error) {
	flags := driverFlags{}
	remaining := []string{}
	sawChangedFiles := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--changed-files":
			sawChangedFiles = true
			// Consume the variadic path list; the outer loop resumes at the next flag.
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags.Changed.Files = append(flags.Changed.Files, args[i])
			}
		case "--with-callers":
			flags.Changed.WithCallers = true
		case "--config-file":
			if i+1 >= len(args) {
				return driverFlags{}, nil, fmt.Errorf("--config-file requires a path argument")
			}
			i++
			flags.ConfigFile = args[i]
		case "--config-yaml":
			if i+1 >= len(args) {
				return driverFlags{}, nil, fmt.Errorf("--config-yaml requires a yaml content argument")
			}
			i++
			flags.ConfigYAML = args[i]
		case "-test", "--test":
			flags.IncludeTests = true
		case "--vet-mode":
			flags.VetMode = true
		default:
			remaining = append(remaining, args[i])
		}
	}
	if flags.Changed.WithCallers && !sawChangedFiles {
		return driverFlags{}, nil, fmt.Errorf("--with-callers requires --changed-files")
	}
	if flags.ConfigFile != "" && flags.ConfigYAML != "" {
		return driverFlags{}, nil, fmt.Errorf("--config-file and --config-yaml are mutually exclusive")
	}
	if len(remaining) == 0 {
		remaining = nil
	}
	return flags, remaining, nil
}

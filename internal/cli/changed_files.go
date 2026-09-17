// Package cli wires command-line flags into vow analyzer runs.
//
// The CLI driver parses --changed-files and --with-callers from os.Args before
// handing the residual arguments to the underlying go/analysis driver, and
// resolves the caller package list when --with-callers is set. Downstream
// analyzer passes read the resulting ChangedFileSet via the package-level
// accessors below.
package cli

import "sync"

// ChangedFileSet carries the Go source paths from --changed-files, the
// --with-callers flag, and the resolved caller package paths that import or
// invoke the changed files. An empty Files slice signals package-level lint
// behavior to downstream passes.
//
// WithCallers requires the --changed-files flag to accompany it; the CLI
// parser rejects standalone --with-callers. An empty post-expansion file
// list (for instance when `git diff` returns nothing) leaves Files and
// CallerPackages empty without error. CallerPackages is populated by the
// driver-side resolver only when WithCallers is set and remains empty
// otherwise.
type ChangedFileSet struct {
	// Files lists the Go source paths the driver restricts self-validation to.
	Files []string
	// WithCallers extends the run with caller-side checks.
	WithCallers bool
	// CallerPackages lists the import paths of packages that call into the
	// changed files, resolved by the driver before delegating to the
	// go/analysis driver. The list excludes the packages that own the
	// changed files themselves.
	CallerPackages []string
	// ChangedPackages lists the import paths of the packages that own the
	// changed files. CallerPackages covers only the 1-hop importers, so
	// without this list a call site in an unchanged file of the owning
	// package matches neither and loses every caller-side diagnostic.
	ChangedPackages []string
}

// Empty reports whether the set carries no changed-file paths.
func (s ChangedFileSet) Empty() bool {
	return len(s.Files) == 0
}

var (
	activeMu  sync.RWMutex
	activeSet ChangedFileSet
)

// SetActive publishes set so analyzer passes can read it via Active.
func SetActive(set ChangedFileSet) {
	activeMu.Lock()
	defer activeMu.Unlock()
	activeSet = set
}

// Active returns the ChangedFileSet currently in effect for analyzer passes.
func Active() ChangedFileSet {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeSet
}

var (
	annotateMu sync.RWMutex
	annotate   bool
)

// SetAnnotateNarrow publishes whether passes attach the information a
// narrow-scope decision needs to each diagnostic instead of applying
// the decision themselves. A driver that analyzes packages in separate
// processes cannot hand the changed-file set to those processes without
// making it part of what their results are cached on, so it asks for the
// inputs and decides in the process that owns the set.
func SetAnnotateNarrow(on bool) {
	annotateMu.Lock()
	defer annotateMu.Unlock()
	annotate = on
}

// AnnotateNarrow reports whether passes attach narrow-scope inputs to
// their diagnostics.
func AnnotateNarrow() bool {
	annotateMu.RLock()
	defer annotateMu.RUnlock()
	return annotate
}

// Package driver runs one or more go/analysis analyzers over a set
// of go/packages Package values in a single goroutine, freeing
// each package's syntax and each job's intermediate result as soon
// as no unfinished job still reads them.
//
// The public API is Run, Result, Job, and PrintDiagnostics. The
// package depends only on the standard library and on the public
// packages under golang.org/x/tools/go/... .
package driver

import (
	"fmt"
	"io"
	"iter"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

// A Job pairs one analyzer with one package and records the outcome
// of applying that analyzer to that package.
//
// After Run returns, callers may read Diagnostics, Err, Root,
// Analyzer, and Package; every other field is driver-internal.
type Job struct {
	Analyzer *analysis.Analyzer
	Package  *packages.Package
	// Root is true when the job belongs to the direct product of
	// the analyzers and packages passed to Run. Only root jobs
	// contribute diagnostics to the printed output.
	Root bool
	// Diagnostics holds every diagnostic the analyzer's Run
	// function reported through Pass.Report / Pass.Reportf.
	// Partial output is preserved on the error path.
	Diagnostics []analysis.Diagnostic
	// Err is the analyzer's Run error, an internal error the
	// driver recorded around the call (ResultType mismatch,
	// dependency failure, ill-typed package without
	// RunDespiteErrors), or nil on success.
	Err error

	// deps lists the jobs this one waits on: same-package
	// Requires edges (feed ResultOf) and import-side edges for
	// FactTypes analyzers (feed fact inheritance).
	deps []*Job
	// consumers counts the still-unfinished jobs that read this
	// one's result. On zero the result is releasable unless the
	// job is a root.
	consumers int
	// result is the analyzer's Run return value. It is nilled
	// when consumers hits zero on a non-root job.
	result any
	// facts is the per-job fact table. Cross-package inheritance
	// fills it before Run runs.
	facts *factTable
	// exportOpen gates ExportObjectFact / ExportPackageFact:
	// new entries only land while Run is on the stack. After Run
	// returns the closures panic.
	exportOpen bool
	// completed is true once this job's execute call has
	// returned, whether the analyzer ran or the early exit
	// recorded a dependency failure.
	completed bool
}

// Result is a Run's outcome. Jobs lists every visited (analyzer,
// package) pair in the order jobs completed, so dependencies
// precede their consumers.
type Result struct {
	Jobs []*Job
}

// All yields the jobs in postorder (dependencies before their
// consumers).
func (r *Result) All() iter.Seq[*Job] {
	return func(yield func(*Job) bool) {
		for _, j := range r.Jobs {
			if !yield(j) {
				return
			}
		}
	}
}

// Run applies each analyzer to each package one job at a time in a
// single goroutine, honoring same-package Requires edges and, for
// analyzers with FactTypes, import-side edges into the same
// analyzer on each imported package. analysis.Validate runs on the
// analyzers before any scheduling.
//
// Run mutates pkg.Syntax and pkg.TypesInfo to nil once no
// still-scheduled job reads them, so the caller must not read
// those fields after Run returns. pkg.Types, pkg.Fset, fact
// tables, diagnostics, and root results survive.
func Run(analyzers []*analysis.Analyzer, pkgs []*packages.Package) (*Result, error) {
	if err := analysis.Validate(analyzers); err != nil {
		return nil, err
	}
	g := buildGraph(analyzers, pkgs)
	g.executeAll()
	return &Result{Jobs: g.completionOrder}, nil
}

// PrintDiagnostics writes each job's error and each root job's
// diagnostics to w, one line per entry. Errors take precedence
// over diagnostics on the same job. Diagnostics with identical
// resolved position, end position, analyzer, and message print
// only once, so test-augmentation twins do not surface twice.
func PrintDiagnostics(w io.Writer, r *Result) error {
	seen := map[dedupKey]struct{}{}
	for _, j := range r.Jobs {
		if j.Err != nil {
			if _, err := fmt.Fprintf(w, "%s: %v\n", j.Analyzer.Name, j.Err); err != nil {
				return err
			}
			continue
		}
		if !j.Root {
			continue
		}
		for _, d := range j.Diagnostics {
			pos := j.Package.Fset.Position(d.Pos)
			end := j.Package.Fset.Position(d.End)
			k := dedupKey{
				Filename: pos.Filename, Line: pos.Line, Column: pos.Column,
				EndFilename: end.Filename, EndLine: end.Line, EndColumn: end.Column,
				Analyzer: j.Analyzer, Message: d.Message,
			}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			if _, err := fmt.Fprintf(w, "%s: %s\n", pos, d.Message); err != nil {
				return err
			}
			for _, rel := range d.Related {
				rp := j.Package.Fset.Position(rel.Pos)
				if _, err := fmt.Fprintf(w, "\t%s: %s\n", rp, rel.Message); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// dedupKey identifies a single (position, end, analyzer, message)
// tuple.
type dedupKey struct {
	Filename, EndFilename string
	Line, EndLine         int
	Column, EndColumn     int
	Analyzer              *analysis.Analyzer
	Message               string
}

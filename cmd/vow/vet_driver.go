package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"

	"github.com/knowledge-work/go-vow/internal/cli"
)

// vetDiagnostic mirrors one entry in `go vet -json`'s per-analyzer
// list. Only the fields vow prints are decoded.
type vetDiagnostic struct {
	Posn    string `json:"posn"`
	Message string `json:"message"`
	Related []struct {
		Message string `json:"message"`
	} `json:"related"`
}

// vetResult is one analyzer's outcome for one package. `go vet -json`
// writes a diagnostic list when the analyzer ran and an object carrying
// its error when it did not, so a decoder that admits only the list
// throws away the whole report the moment one package fails.
type vetResult struct {
	Diagnostics []vetDiagnostic
	Err         string
}

// UnmarshalJSON accepts both shapes a result takes.
//
// vow:nil (?) ?
func (r *vetResult) UnmarshalJSON(raw []byte) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var failure struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(trimmed, &failure); err != nil {
			return err
		}
		r.Err = failure.Error
		return nil
	}
	return json.Unmarshal(trimmed, &r.Diagnostics)
}

// vetPackagePath strips the decoration the go command puts on a test
// variant's identifier, so a scope of import paths matches the key the
// report is filed under. An external test package is reported as
// `<path>_test`, and some releases add a bracketed suffix.
//
// vow:nil ()
func vetPackagePath(id string) string {
	if bracket := strings.Index(id, " ["); bracket >= 0 {
		id = id[:bracket]
	}
	id = strings.TrimSuffix(id, ".test")
	return strings.TrimSuffix(id, "_test")
}

// runVetPipeline lints patterns by driving `go vet` with this binary as
// the vettool, and returns the process exit code. The go command then
// analyzes one package at a time and supplies each dependency's facts
// from its build cache, where the in-process driver instead loads the
// dependency closure with syntax and analyzes it again on every run.
//
// scope names the packages whose diagnostics survive. `go vet` prints
// for every package it analyzes, while a vow run reports only for the
// packages it targets — the distinction the in-process driver draws
// with a per-job flag. An empty scope keeps every package.
//
// vow:nil (,,,?)
func runVetPipeline(configYAML string, narrow cli.ChangedFileSet, includeTests bool, patterns []string) int {
	self, err := os.Executable()
	if err != nil {
		os.Stderr.WriteString("vow: locate own binary: " + err.Error() + "\n")
		return 1
	}
	args := []string{"vet", "-json", "-vettool=" + self}
	if configYAML != "" {
		args = append(args, "-vow."+vetConfigFlag+"="+configYAML)
	}
	// The flag rides on every run, narrow or not: its value is what the
	// go command keys a package's result on, so making it conditional
	// would keep one cached result per shape of run instead of one per
	// package.
	args = append(args, "-vow."+vetAnnotateFlag)
	args = append(args, vetPatterns(patterns)...)
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), vetToolEnv+"=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	// `go vet -json` writes the report to stdout and everything else —
	// load failures, tool failures — to stderr, so the stderr text is the
	// only account of a failure.
	report, parseErr := parseVetReport(stdout.Bytes())
	if parseErr != nil {
		os.Stderr.Write(stderr.Bytes())
		os.Stderr.WriteString("vow: read go vet report: " + parseErr.Error() + "\n")
		return 1
	}
	printed, failed := printVetDiagnostics(report, narrow, includeTests)
	if failed > 0 {
		// An analyzer that did not run leaves that package unchecked, so
		// the run cannot claim a verdict for it.
		return 1
	}
	if runErr != nil {
		// A partial report is still a failed run: some package did not
		// finish, and reporting only what the others said would present
		// an incomplete answer as a complete one.
		os.Stderr.Write(stderr.Bytes())
		os.Stderr.WriteString("vow: go vet: " + runErr.Error() + "\n")
		return 1
	}
	if printed > 0 {
		return 3
	}
	return 0
}

// parseVetReport decodes `go vet -json`'s report, which nests
// diagnostics under the package path and then the analyzer name. The go
// command emits one such object per batch of units rather than one for
// the whole run, so the objects are decoded as a stream and merged.
// Empty output decodes to an empty report, keeping a run with nothing
// to say distinct from a malformed one.
//
// vow:nil (?) ?,
func parseVetReport(raw []byte) (map[string]map[string]vetResult, error) {
	report := map[string]map[string]vetResult{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	for {
		chunk := map[string]map[string]vetResult{}
		err := decoder.Decode(&chunk)
		if errors.Is(err, io.EOF) {
			return report, nil
		}
		if err != nil {
			return nil, err
		}
		for path, byAnalyzer := range chunk {
			if report[path] == nil {
				report[path] = map[string]vetResult{}
			}
			for name, result := range byAnalyzer {
				report[path][name] = result
			}
		}
	}
}

// printVetDiagnostics writes the diagnostics of the in-scope packages
// to stderr in the driver's `position: message` shape and returns how
// many it wrote and how many analyzers failed. Packages are visited in
// sorted order so a run's output does not depend on map iteration.
//
// includeTests keeps the diagnostics of _test.go files. The go command
// always analyzes test files, where a vow run reaches them only under
// -test, so dropping them here is what keeps the two paths reporting
// the same set.
//
// vow:nil (?,,) ,
func printVetDiagnostics(report map[string]map[string]vetResult, narrow cli.ChangedFileSet, includeTests bool) (printed, failed int) {
	paths := make([]string, 0, len(report))
	for path := range report {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	scope := narrowPackages(narrow)
	for _, path := range paths {
		if len(scope) > 0 && !scope[vetPackagePath(path)] {
			continue
		}
		byAnalyzer := report[path]
		names := make([]string, 0, len(byAnalyzer))
		for name := range byAnalyzer {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			result := byAnalyzer[name]
			if result.Err != "" {
				os.Stderr.WriteString("vow: " + name + " failed on " + path + ": " + result.Err + "\n")
				failed++
				continue
			}
			for _, diag := range result.Diagnostics {
				if !includeTests && isTestFileDiagnostic(diag.Posn) {
					continue
				}
				if dropByNarrowScope(diag, vetPackagePath(path), narrow) {
					continue
				}
				os.Stderr.WriteString(diag.Posn + ": " + diag.Message + "\n")
				printed++
			}
		}
	}
	return printed, failed
}

// isTestFileDiagnostic reports whether a diagnostic position names a Go
// test file. The position carries `file:line:col`, so the file ends at
// the first colon that follows it.
//
// vow:nil ()
func isTestFileDiagnostic(posn string) bool {
	file := posn
	if colon := strings.Index(posn, ".go:"); colon >= 0 {
		file = posn[:colon+len(".go")]
	}
	return strings.HasSuffix(file, "_test.go")
}

// vetPatterns returns the package patterns for a vet run, falling back
// to the whole module when the run named none. `go vet` with no pattern
// analyzes the current directory alone, where a vow run covers the
// module, so the fallback lives here rather than with the caller.
//
// vow:nil (?) ?
func vetPatterns(patterns []string) []string {
	if len(patterns) == 0 {
		return []string{"./..."}
	}
	return patterns
}

// narrowPackages returns the package paths whose diagnostics a narrow
// run reports: the packages that own the changed files together with
// their callers.
//
// vow:nil () ?
func narrowPackages(narrow cli.ChangedFileSet) map[string]bool {
	if narrow.Empty() {
		return nil
	}
	scope := map[string]bool{}
	for _, path := range narrow.ChangedPackages {
		scope[path] = true
	}
	for _, path := range narrow.CallerPackages {
		scope[path] = true
	}
	return scope
}

// dropByNarrowScope reports whether a diagnostic falls outside the
// narrow scope, applying the same rule the in-process driver applies
// inside the analyzer. The inputs the rule needs beyond the position and
// the package — whether the diagnostic sits inside a call, and where its
// callee is declared — ride along as related information, because the
// changed-file set cannot travel to the analysis without becoming part
// of what the go command caches the analysis on.
//
// A diagnostic with no annotation is kept: the analyzer attaches one to
// every diagnostic it emits under the annotation switch, so a missing
// one means the rule has no opinion, and dropping on no opinion would
// lose a report silently.
//
// vow:nil (,,)
func dropByNarrowScope(diag vetDiagnostic, pkgPath string, narrow cli.ChangedFileSet) bool {
	if narrow.Empty() {
		return false
	}
	if matchesPath(narrow.Files, positionFile(diag.Posn)) {
		return false
	}
	inCallerPkg := matchesPath(narrow.CallerPackages, pkgPath)
	if !inCallerPkg && !matchesPath(narrow.ChangedPackages, pkgPath) {
		return true
	}
	annotation, found := narrowAnnotationOf(diag)
	switch {
	case !found:
		return false
	case annotation == "call=none":
		return !inCallerPkg
	case annotation == "callee=unknown":
		return true
	case strings.HasPrefix(annotation, "callee-file="):
		return !matchesPath(narrow.Files, strings.TrimPrefix(annotation, "callee-file="))
	}
	return false
}

// narrowAnnotationOf returns the narrow-scope annotation the analyzer
// attached to diag, without its prefix.
//
// vow:nil () ,
func narrowAnnotationOf(diag vetDiagnostic) (string, bool) {
	const prefix = "vow:narrow "
	for _, related := range diag.Related {
		if strings.HasPrefix(related.Message, prefix) {
			return strings.TrimPrefix(related.Message, prefix), true
		}
	}
	return "", false
}

// positionFile returns the file part of a `file:line:col` position.
//
// vow:nil ()
func positionFile(posn string) string {
	if suffix := strings.Index(posn, ".go:"); suffix >= 0 {
		return posn[:suffix+len(".go")]
	}
	return posn
}

// matchesPath reports whether candidate equals any entry in paths.
//
// vow:nil (?,)
func matchesPath(paths []string, candidate string) bool {
	if candidate == "" {
		return false
	}
	return slices.Contains(paths, candidate)
}

// vetConfigYAML returns the config text to hand the vettool. A config
// named by path is read here rather than opened by the tool: the go
// command keys a cached result on the flags the tool received, so the
// content has to travel as the flag's value for an edited config to
// take effect. An empty return leaves the tool with its zero Config,
// which also means the per-directory vow.yaml walk does not run under
// this mode.
func vetConfigYAML(flags driverFlags) string {
	switch {
	case flags.ConfigYAML != "":
		return flags.ConfigYAML
	case flags.ConfigFile != "":
		raw, err := os.ReadFile(flags.ConfigFile)
		if err != nil {
			os.Stderr.WriteString("vow: --config-file: " + err.Error() + "\n")
			os.Exit(2)
		}
		return string(raw)
	}
	return ""
}

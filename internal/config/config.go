// Package config models the per-directory `vow.yaml` configuration
// that lets adopters narrow the analyzer's default policy. The driver
// resolves a file's nearest-ancestor vow.yaml at analysis time and
// passes the parsed Config into the obligation walks that consult it.
//
// Initial surface area: the closable obligation pass reads
// Closable.BuiltinClosables to decide which built-in closable types
// participate at this package's site. Other obligations will grow
// analogous knobs as adoption pressure surfaces them.
package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config models a single vow.yaml file. Fields default to the
// zero-value semantics, so a vow.yaml that omits a section preserves
// the embedded default for that section.
type Config struct {
	// Closable controls the closable obligation pass.
	Closable Closable `yaml:"closable"`
	// CallerResolver controls the driver's `--with-callers` flow.
	// Adopters consult this section through `--config-file` /
	// `--config-yaml` overrides only; per-directory `vow.yaml`
	// files do not propagate into the resolver because the
	// resolver runs once per driver invocation rather than once
	// per package.
	CallerResolver CallerResolver `yaml:"caller_resolver"`
	// NilDecl controls the nil-declaration passes.
	NilDecl NilDecl `yaml:"nil_decl"`
	// AnalysisScope controls which packages the driver loads with
	// syntax and analyzes. Adopters consult this section through
	// `--config-file` / `--config-yaml` overrides only, like
	// CallerResolver: the load happens once per driver invocation,
	// before any per-directory discovery could run.
	AnalysisScope AnalysisScope `yaml:"analysis_scope"`
}

// AnalysisScope models the driver's package-loading scope knobs.
type AnalysisScope struct {
	// FirstPartyPrefixes lists the import-path prefixes under
	// which vow markers may appear. When the list is non-empty,
	// the driver parses and analyzes only the target packages and
	// the dependencies matching one of these prefixes; every
	// other dependency contributes type information from export
	// data and is neither parsed nor analyzed. vow facts
	// originate exclusively from markers, so a dependency outside
	// every prefix can contribute nothing to the run — the
	// adopter asserts that by listing the prefixes. An absent or
	// empty list keeps the default whole-closure load.
	//
	// A prefix matches a package on import-path boundaries:
	// `example.com/foo` covers `example.com/foo` and
	// `example.com/foo/...`, never `example.com/foobar`.
	FirstPartyPrefixes []string `yaml:"first_party_prefixes"`
}

// NilDecl models the nil-declaration section's knobs.
type NilDecl struct {
	// RequireDeclarations asks the analyzer to report a function
	// whose signature carries a nillable parameter or return that
	// its `vow:nil` marker leaves without a nullness decl. The
	// default (absent, false) leaves the check off, so a scope
	// opts in by setting the key.
	RequireDeclarations bool `yaml:"require_declarations"`
}

// Closable models the closable-section knobs.
type Closable struct {
	// BuiltinClosables overrides the closable preset's built-in
	// type list (`*os.File`, `*net.TCPConn`, `*bufio.Writer`,
	// `io.Closer`) for the scope this config applies to. A nil
	// pointer (the field is absent from yaml) keeps the embedded
	// list; a non-nil pointer — including an empty list — replaces
	// it. Author-declared `vow:define @Closable` types still
	// participate regardless of this knob; only the ambient
	// stdlib-and-convention list is controlled.
	BuiltinClosables *[]string `yaml:"built_in_closables"`
}

// CallerResolver models the `--with-callers` resolver knobs.
type CallerResolver struct {
	// ExcludePackageSuffixes lists import-path suffixes the
	// resolver drops from its scope in addition to the synthetic
	// test packages it always removes structurally. The knob
	// expresses adopter intent (e.g. keeping generated mock
	// packages out of the caller scope); it is no longer needed
	// to keep the loader from rejecting test-package paths. A
	// nil pointer (the field is absent from yaml) keeps the
	// embedded default; a non-nil pointer — including an empty
	// list — replaces it.
	ExcludePackageSuffixes *[]string `yaml:"exclude_package_suffixes"`
}

// DefaultExcludePackageSuffixes returns the suffix list the driver
// applies when the adopter has not supplied an override. The
// default is empty: the resolver removes Go's synthetic test
// packages by their structural identity, so no path-suffix
// heuristic is required — and a heuristic would misfire, because a
// real package's import path may legally end in `_test`.
//
// vow:nil () ?
func DefaultExcludePackageSuffixes() []string {
	return nil
}

// Parse decodes a vow.yaml payload into a Config. An empty payload
// returns the zero-value Config, which carries the embedded defaults
// for every section. Unknown top-level keys are rejected so a typo
// (`closables:` instead of `closable:`) surfaces as an error rather
// than as a silent no-op.
//
// vow:nil (?) ?,
func Parse(raw []byte) (*Config, error) {
	cfg := &Config{}
	if len(raw) == 0 {
		return cfg, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse vow.yaml: %w", err)
	}
	return cfg, nil
}

// ReadFile reads the file at path and parses it as a vow.yaml. The
// path must exist; callers handle the not-found case before reaching
// this helper, because the absence of a file means "no config" rather
// than "empty config" at the discovery layer.
//
// vow:nil () ?,
func ReadFile(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read vow.yaml at %s: %w", path, err)
	}
	return Parse(raw)
}

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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/knowledge-work/go-vow/internal/dsl"
	"github.com/knowledge-work/go-vow/internal/seq"
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
	// Presets lists preset files the analyzer loads next to its
	// built-in presets, each path absolute or relative to the
	// directory of the vow.yaml that names it. A preset named like a
	// built-in preset replaces it; any other adds to the set.
	Presets []string `yaml:"presets"`
	// LoadedPresets holds the presets Presets names, in the same
	// order. ReadFile and LoadPresets fill it; Parse leaves it empty,
	// since a payload read from no file has no directory to resolve
	// the paths against.
	LoadedPresets []LoadedPreset `yaml:"-"`
}

// LoadedPreset is one preset file a vow.yaml names, read and parsed.
type LoadedPreset struct {
	// Source is the file's content. --vet-mode hands it to the
	// vettool, which keys its cached result on its flags and so cannot
	// read the file itself.
	Source []byte
	// Preset is the preset Source parses to.
	Preset *dsl.Preset
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
// for every section. Unknown keys are rejected at every level so a typo
// (`closables:` instead of `closable:`, or `require_declaration:`
// under `nil_decl:`) surfaces as an error rather than as a silent no-op.
//
// vow:nil (?) ?,
func Parse(raw []byte) (*Config, error) {
	cfg, err := decode(raw)
	if err != nil {
		return nil, fmt.Errorf("parse vow.yaml: %w", err)
	}
	return cfg, nil
}

// decode is Parse without the error prefix, so a caller that knows the
// file's path can name the file instead.
//
// vow:nil (?) ?,
func decode(raw []byte) (*Config, error) {
	cfg := &Config{}
	if len(raw) == 0 {
		return cfg, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		// A file of comments alone holds no document, which the decoder
		// reports as EOF; it configures nothing, as an empty file does.
		if errors.Is(err, io.EOF) {
			return cfg, nil
		}
		return nil, err
	}
	return cfg, nil
}

// ReadFile reads the file at path and parses it as a vow.yaml, loading
// the preset files it names. The path must exist; callers handle the
// not-found case before reaching this helper, because the absence of a
// file means "no config" rather than "empty config" at the discovery
// layer.
//
// vow:nil () ?,
func ReadFile(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read vow.yaml at %s: %w", path, err)
	}
	cfg, err := decode(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.LoadPresets(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// LoadPresets reads and parses the preset files Presets names into
// LoadedPresets, resolving a relative path against dir. Two files
// that declare one preset name are an error, as CheckPresetNames
// reports.
func (c *Config) LoadPresets(dir string) error {
	if len(c.Presets) == 0 {
		return nil
	}
	loaded, err := seq.ChainOf(c.Presets...).MapOrErr(func(path string) (LoadedPreset, error) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return LoadedPreset{}, fmt.Errorf("presets: %w", err)
		}
		preset, err := ParsePreset(raw)
		if err != nil {
			return LoadedPreset{}, fmt.Errorf("presets: %s: %w", path, err)
		}
		return LoadedPreset{Source: raw, Preset: preset}, nil
	}).ToSlice()
	if err != nil {
		return err
	}
	if err := CheckPresetNames(loaded); err != nil {
		return err
	}
	c.LoadedPresets = loaded
	return nil
}

// CheckPresetNames reports two loaded presets that declare one name.
// A preset replaces the built-in preset of its name, so of two such
// presets one would either replace the other without a word or run
// beside it.
//
// vow:nil (?)
func CheckPresetNames(loaded []LoadedPreset) error {
	seen := map[string]bool{}
	dup, found := seq.ChainOf(loaded...).Find(func(p LoadedPreset) bool {
		if seen[p.Preset.Name] {
			return true
		}
		seen[p.Preset.Name] = true
		return false
	})
	if found {
		return fmt.Errorf("presets: two files declare the preset name %q", dup.Preset.Name)
	}
	return nil
}

// ParsePreset parses the content of a preset file a vow.yaml names. On
// top of what dsl.Parse checks, every subject has to select a concept
// through `vow:define @<Concept>`, and the preset may configure
// nothing that vow reads only from its built-in presets: rules, which
// vow:import reads only from the libraries built into vow, and the
// Closable check, which is built in and takes its types from
// closable.built_in_closables in vow.yaml.
//
// vow:nil (?) ?,
func ParsePreset(raw []byte) (*dsl.Preset, error) {
	preset, err := dsl.Parse(raw)
	if err != nil {
		return nil, err
	}
	if len(preset.Rules) > 0 {
		return nil, fmt.Errorf("preset %q declares rules, which vow:import does not read from a loaded preset", preset.Name)
	}
	subject, found := seq.ChainOf(preset.Subjects...).Find(func(s dsl.Subject) bool {
		_, ok := s.Concept()
		return !ok
	})
	if found {
		return nil, fmt.Errorf("preset %q subject %q selects no concept; write annotation:vow:define @<Concept>", preset.Name, subject.Match)
	}
	if preset.Name == "closable" {
		return nil, fmt.Errorf("preset %q would replace the built-in closable preset; rename it, and set the Closable types with closable.built_in_closables in vow.yaml", preset.Name)
	}
	if len(preset.BuiltinClosables) > 0 {
		return nil, fmt.Errorf("preset %q sets the Closable types, which closable.built_in_closables in vow.yaml sets", preset.Name)
	}
	if seq.ChainOf(preset.Obligations...).Any(func(o dsl.Obligation) bool { return o.Type == "must-close" }) {
		return nil, fmt.Errorf("preset %q declares must-close, which no check reads: the Closable check is built in", preset.Name)
	}
	return preset, nil
}

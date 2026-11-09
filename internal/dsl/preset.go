// Package dsl defines the data model for vow preset files and provides
// loaders. Presets express *value-level obligations* as pairs of
// (subject selector, obligation predicate); a rule engine in
// internal/analysis dispatches each obligation type to a matching
// implementation.
//
// The DSL is intentionally minimal: subjects are described by a single
// pattern string and obligations carry only a type tag plus an untyped
// detail bag. The rule engine is responsible for interpreting the
// detail bag against the obligation type.
package dsl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"slices"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/knowledge-work/go-vow/internal/seq"
)

// Preset is the top-level structure of a preset YAML file.
type Preset struct {
	Name             string             `yaml:"name"`
	Version          string             `yaml:"version"`
	Description      string             `yaml:"description"`
	Subjects         []Subject          `yaml:"subjects"`
	BuiltinClosables []string           `yaml:"built_in_closables"`
	Obligations      []Obligation       `yaml:"obligations"`
	Severity         string             `yaml:"severity"`
	Rules            map[string]RuleDef `yaml:"rules"`
}

// Subject selects the values an obligation applies to.
//
// Match is a pattern string interpreted by the rule engine; the syntax
// is "<scheme>:<argument>". The currently defined scheme is
// "annotation:" — the argument is a doc-comment marker (e.g.
// "vow:define @Sentinel") that must appear on a package-level var declaration.
type Subject struct {
	Match string `yaml:"match"`
}

// Obligation declares what must happen to subject values.
//
// Type identifies the rule engine to dispatch to (e.g. "must-consume").
// Detail is a free-form bag whose schema is determined by Type — see
// the rule engine for valid keys.
type Obligation struct {
	Type   string         `yaml:"type"`
	Detail map[string]any `yaml:"detail"`
}

// Load reads a preset file from disk, parses it as YAML, and validates
// the minimum invariants the rule engine relies on.
func Load(path string) (*Preset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("dsl: read %s: %w", path, err)
	}
	p, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("dsl: %s: %w", path, err)
	}
	return p, nil
}

// Parse decodes a preset from in-memory YAML bytes. It is used both by
// Load (for files on disk) and by callers that embed presets at build
// time via //go:embed. A key the preset schema does not define is
// rejected, so a misspelled key surfaces as an error rather than as a
// setting that silently does nothing.
func Parse(raw []byte) (*Preset, error) {
	var p Preset
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	// An empty document decodes as EOF; validate then reports what the
	// preset lacks.
	if err := dec.Decode(&p); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if err := p.validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	return &p, nil
}

// validate checks the structural invariants a parsed preset must
// satisfy: the name is non-empty, the contribution set covers at
// least one of subjects/obligations or rules, paired contribution
// fields are both populated, the severity, when set, is the only one
// there is, every subject uses the annotation scheme, every obligation
// is of a type vow reads and carries the detail that type reads, and
// every rule has a name, a body, and distinct parameters.
//
// vow:cond * -> ErrInvalidPreset | nil
func (p *Preset) validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("preset name is empty: %w", ErrInvalidPreset)
	}
	// A preset must contribute *something* — either obligations on a
	// set of subjects or reusable rule templates. Rule-only presets
	// (e.g. preset/std/result) skip the subject / obligation gates.
	hasObligationContract := len(p.Subjects) > 0 || len(p.Obligations) > 0
	hasRules := len(p.Rules) > 0
	if p.Severity != "" && p.Severity != "warning" {
		return fmt.Errorf("preset %q severity %q: vow reads no severity; write warning or omit it: %w", p.Name, p.Severity, ErrInvalidPreset)
	}
	if !hasObligationContract && !hasRules {
		return fmt.Errorf("preset %q declares neither subjects/obligations nor rules: %w", p.Name, ErrInvalidPreset)
	}
	if hasObligationContract {
		if len(p.Subjects) == 0 {
			return fmt.Errorf("preset %q declares obligations without subjects: %w", p.Name, ErrInvalidPreset)
		}
		if len(p.Obligations) == 0 {
			return fmt.Errorf("preset %q declares subjects without obligations: %w", p.Name, ErrInvalidPreset)
		}
	}
	for i, s := range p.Subjects {
		if strings.TrimSpace(s.Match) == "" {
			return fmt.Errorf("preset %q subject[%d] has empty match: %w", p.Name, i, ErrInvalidPreset)
		}
		if s.AnnotationMarker() == "" {
			return fmt.Errorf("preset %q subject[%d] match %q does not name a marker after annotation:, the only scheme: %w", p.Name, i, s.Match, ErrInvalidPreset)
		}
	}
	for i, o := range p.Obligations {
		if strings.TrimSpace(o.Type) == "" {
			return fmt.Errorf("preset %q obligation[%d] has empty type: %w", p.Name, i, ErrInvalidPreset)
		}
		if err := o.validateDetail(); err != nil {
			return fmt.Errorf("preset %q obligation[%d] (%s): %v: %w", p.Name, i, o.Type, err, ErrInvalidPreset)
		}
	}
	for name, rule := range p.Rules {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("preset %q has a rule with an empty name: %w", p.Name, ErrInvalidPreset)
		}
		if strings.TrimSpace(rule.Body) == "" {
			return fmt.Errorf("preset %q rule %q has an empty body: %w", p.Name, name, ErrInvalidPreset)
		}
		seen := make(map[string]struct{}, len(rule.Params))
		for _, param := range rule.Params {
			if _, dup := seen[param]; dup {
				return fmt.Errorf("preset %q rule %q has duplicate param %q: %w", p.Name, name, param, ErrInvalidPreset)
			}
			seen[param] = struct{}{}
		}
	}
	return nil
}

// AnnotationMarker returns the doc-comment marker requested by an
// annotation-scheme subject pattern, or "" if the subject uses a
// different scheme.
func (s Subject) AnnotationMarker() string {
	const prefix = "annotation:"
	if strings.HasPrefix(s.Match, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(s.Match, prefix))
	}
	return ""
}

// Concept returns the concept an annotation-scheme subject selects
// through `vow:define @<Concept>`, and false for a subject that names
// any other marker.
func (s Subject) Concept() (string, bool) {
	name, ok := strings.CutPrefix(s.AnnotationMarker(), "vow:define @")
	if !ok || !isName(name) {
		return "", false
	}
	return name, true
}

// validateDetail checks an obligation against its type: the type must
// be one vow reads, and the detail must hold only the
// keys that type reads, in the shape it reads them. The readers below
// skip what they cannot decode, so without this check a malformed
// entry would do nothing and say nothing.
func (o Obligation) validateDetail() error {
	switch o.Type {
	case "must-consume":
		if err := rejectUnknownKeys("detail", o.Detail, "consumers"); err != nil {
			return err
		}
		return validateNameList("detail.consumers", "function name", o.Detail["consumers"])
	case "passthrough-emit":
		if err := rejectUnknownKeys("detail", o.Detail, "functions"); err != nil {
			return err
		}
		functions, ok := o.Detail["functions"].([]any)
		if !ok {
			return errors.New("detail.functions must be a list of wrap signatures")
		}
		return seq.WithIndex(seq.ChainOf(functions...)).EachOrErr(func(f seq.Indexed[any]) error {
			return validatePassthroughEntry(fmt.Sprintf("detail.functions[%d]", f.Index), f.Value)
		})
	case "must-close":
		if err := rejectUnknownKeys("detail", o.Detail, "discharges"); err != nil {
			return err
		}
		if discharges, ok := o.Detail["discharges"]; ok {
			return validateNameList("detail.discharges", "discharge name", discharges)
		}
	default:
		return errors.New("not an obligation type vow reads; use must-consume or passthrough-emit (must-close belongs to the built-in closable preset)")
	}
	return nil
}

// validatePassthroughEntry checks one wrap signature of a
// passthrough-emit obligation against the shape
// PassthroughEmitFunctions reads. where names the entry in the error.
func validatePassthroughEntry(where string, raw any) error {
	entry, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be a map with pattern, arg-position, and return-slot", where)
	}
	if err := rejectUnknownKeys(where, entry, "pattern", "arg-position", "return-slot"); err != nil {
		return err
	}
	if !isName(entry["pattern"]) {
		return fmt.Errorf("%s.pattern must be a qualified function name without spaces", where)
	}
	if pos, ok := readPresetInt(entry["arg-position"]); !ok || pos < 0 {
		return fmt.Errorf("%s.arg-position must be a 0-based parameter index", where)
	}
	if slot, ok := readPresetInt(entry["return-slot"]); !ok || slot < 1 {
		return fmt.Errorf("%s.return-slot must be a 1-based return position", where)
	}
	return nil
}

// validateNameList checks that raw is a list whose every entry is a
// name in the sense of isName. what is the singular noun the error
// uses for an entry, and where names the list.
func validateNameList(where, what string, raw any) error {
	list, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s must be a list, each entry a %s", where, what)
	}
	bad, found := seq.WithIndex(seq.ChainOf(list...)).Find(func(v seq.Indexed[any]) bool {
		return !isName(v.Value)
	})
	if found {
		return fmt.Errorf("%s[%d] must be a %s without spaces", where, bad.Index, what)
	}
	return nil
}

// isName reports whether v is a non-empty string without whitespace.
// The analyzer compares a configured name with the call site as
// written, so a name holding a space could never match one.
func isName(v any) bool {
	name, ok := v.(string)
	return ok && name != "" && !strings.ContainsFunc(name, unicode.IsSpace)
}

// rejectUnknownKeys reports the first key of entries, in sorted order,
// that is not among known. where names the map in the error.
func rejectUnknownKeys(where string, entries map[string]any, known ...string) error {
	key, found := seq.ChainOf(slices.Sorted(maps.Keys(entries))...).Find(func(key string) bool {
		return !slices.Contains(known, key)
	})
	if found {
		return fmt.Errorf("%s has unknown key %q", where, key)
	}
	return nil
}

// MustConsumeConsumers returns the list of qualified function names
// declared as obligation consumers, or nil if Type != "must-consume".
func (o Obligation) MustConsumeConsumers() []string {
	if o.Type != "must-consume" {
		return nil
	}
	raw, ok := o.Detail["consumers"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// PassthroughEmitFunction declares one wrap signature the
// passthrough-emit obligation registers. Pattern is the
// fully-qualified function name (`<import-path>.Name`);
// ArgPosition is the 0-based parameter slot the wrap reads its
// subject from; ReturnSlot is the 1-based return position the
// wrapped value lands in. The caller-side destination recogniser
// matches on Pattern alone; ArgPosition and ReturnSlot describe
// the wrap signature so a richer evaluator can validate that
// the subject named on the marker flows through the matching
// slots.
type PassthroughEmitFunction struct {
	Pattern     string
	ArgPosition int
	ReturnSlot  int
}

// PassthroughEmitFunctions returns the wrap signatures declared
// on the obligation, or nil when Type is not "passthrough-emit".
// An entry whose pattern is empty, whose arg-position is
// negative, whose return-slot is non-positive, or whose YAML
// shape does not decode as the expected types drops silently so
// a partial obligation contributes the entries that parse
// cleanly without breaking on the bad ones. Parse rejects such an
// entry, so only an Obligation built in code can carry one.
func (o Obligation) PassthroughEmitFunctions() []PassthroughEmitFunction {
	if o.Type != "passthrough-emit" {
		return nil
	}
	raw, ok := o.Detail["functions"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]PassthroughEmitFunction, 0, len(list))
	for _, v := range list {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		pattern, _ := entry["pattern"].(string)
		argPos, argOK := readPresetInt(entry["arg-position"])
		retSlot, retOK := readPresetInt(entry["return-slot"])
		if pattern == "" || !argOK || !retOK || argPos < 0 || retSlot < 1 {
			continue
		}
		out = append(out, PassthroughEmitFunction{
			Pattern:     pattern,
			ArgPosition: argPos,
			ReturnSlot:  retSlot,
		})
	}
	return out
}

// readPresetInt reads a YAML scalar that may decode as int,
// int64, or float64 (the latter when yaml.v3 inferred a numeric
// type from the source). The helper reports ok=false when the
// value is missing, not numeric, or not a whole number so the
// caller can drop the offending entry without crashing on the type
// assertion.
func readPresetInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n != math.Trunc(n) {
			return 0, false
		}
		return int(n), true
	}
	return 0, false
}

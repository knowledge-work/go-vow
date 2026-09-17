package dsl

import "fmt"

// TermSeverity classifies a TermDiagnostic emitted by ValidateTerm.
// The name is prefixed with `Term` to disambiguate from `Preset.Severity`,
// which carries a free-form runtime diagnostic level — the two concerns
// live in the same package but describe different scopes.
type TermSeverity int

const (
	// TermSeverityWarning is a soft signal: the term is parseable and the
	// rule engine will accept it, but the author left a nilability
	// choice implicit.
	TermSeverityWarning TermSeverity = iota
	// TermSeverityError marks a diagnostic that should prevent the preset
	// from being loaded — used when ValidateOptions.StrictAnnotation
	// promotes warnings to hard failures.
	TermSeverityError
)

// String renders TermSeverity in lowercase for logging.
func (s TermSeverity) String() string {
	switch s {
	case TermSeverityError:
		return "error"
	default:
		return "warning"
	}
}

// TermDiagnostic is a single advisory or error returned by ValidateTerm.
type TermDiagnostic struct {
	Severity TermSeverity
	Message  string
}

// ValidateOptions controls how ValidateTerm classifies otherwise-soft
// findings. The zero value is the lenient configuration (bare terms
// generate warnings only).
type ValidateOptions struct {
	// StrictAnnotation upgrades the "bare type" advisory to an error.
	// Presets that want every term to carry an explicit qualifier set
	// this to true.
	StrictAnnotation bool
}

// ValidateTerm returns any policy diagnostics that apply to a parsed
// Term. It does not re-check the syntactic invariants that ParseTerm
// already enforces (qualifier without base, value-typed Optional, ...);
// those are caller-supplied or compile-time errors at this point.
//
// The diagnostics are returned in stable order so callers can format
// them deterministically.
func ValidateTerm(t Term, opts ValidateOptions) []TermDiagnostic {
	var out []TermDiagnostic
	if t.isBareType() {
		sev := TermSeverityWarning
		if opts.StrictAnnotation {
			sev = TermSeverityError
		}
		out = append(out, TermDiagnostic{
			Severity: sev,
			Message:  fmt.Sprintf("bare type %q: prefer a value-level predicate (`nonzero T`, `nil T`, ...) to make nilability explicit", t.Type),
		})
	}
	return out
}

// isBareType reports whether the term is a named type with no
// value-level predicate — the only condition ValidateTerm currently
// flags. Placeholders (`_`) are excluded: they intentionally stand
// for "any value at this position" and do not warrant a nilability
// advisory.
func (t Term) isBareType() bool {
	return !t.Placeholder && t.Type != "" && t.Pred == nil
}

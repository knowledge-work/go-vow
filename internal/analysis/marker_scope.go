package analysis

import "strings"

// stripMarkerScope inspects line for a marker reference whose
// optional `[subject]` scope qualifies the marker's payload against
// a single name from the enclosing function's signature. The helper
// returns the subject (empty when no scope is present), the
// trimmed payload that follows the marker tokens, and whether the
// line carries the marker at all.
//
// Supported shapes:
//
//   - "vow:emit X" — bare marker (no scope) — subject == "", payload == "X".
//   - "vow:emit[cb] Close" — scoped marker — subject == "cb", payload == "Close".
//   - "vow:emit" — bare marker with empty payload — subject == "", payload == "".
//   - "vow:emit[cb]" — scoped marker with empty payload — subject == "cb", payload == "".
//
// A line whose marker prefix matches but whose scope brackets are
// malformed (missing `]`, empty `[]`) reports ok=false so the
// downstream caller treats the line as an unrecognised marker
// rather than registering an unintended `[` payload prefix as a
// subject name.
//
// A line that carries a chain of multiple `[X][Y]...` scope
// qualifiers reports ok=false as well, so single-subject callers
// reject chain syntax at the parser boundary; the chain-aware
// helper stripMarkerScopeChain is the entry point that recognises
// nested higher-order callbacks.
//
// The helper is used by every marker family that opts into the
// single-subject scope grammar so the same `[subject]` lexer
// behaviour applies uniformly to vow:use, vow:emit, vow:cond, and
// vow:nil.
func stripMarkerScope(line, marker string) (subject, payload string, ok bool) {
	chain, payload, ok := stripMarkerScopeChain(line, marker)
	if !ok {
		return "", "", false
	}
	switch len(chain) {
	case 0:
		return "", payload, true
	case 1:
		return chain[0], payload, true
	}
	return "", "", false
}

// stripMarkerScopeChain inspects line for a marker reference whose
// optional `[X][Y]...` scope qualifier chains through nested
// higher-order callbacks. The helper returns the chain (nil when no
// scope is present), the trimmed payload that follows the scope
// tokens, and whether the line carries the marker at all.
//
// Supported shapes:
//
//   - "vow:emit X" — bare marker (no scope) — chain == nil, payload == "X".
//   - "vow:emit[cb] Close" — single-level scope — chain == ["cb"], payload == "Close".
//   - "vow:emit[outer][inner] Close" — chained scope — chain == ["outer", "inner"], payload == "Close".
//   - "vow:emit" — empty payload — chain == nil, payload == "".
//   - "vow:emit[cb]" — single-level empty payload — chain == ["cb"], payload == "".
//
// A line whose scope brackets are malformed (missing `]`, empty
// `[]`, or non-`[` content between the marker and the payload)
// reports ok=false so callers treat the line as unrecognised.
func stripMarkerScopeChain(line, marker string) (chain []string, payload string, ok bool) {
	if line == marker {
		return nil, "", true
	}
	if !strings.HasPrefix(line, marker) {
		return nil, "", false
	}
	rest := line[len(marker):]
	if rest == "" {
		return nil, "", true
	}
	switch rest[0] {
	case ' ', '\t':
		return nil, strings.TrimSpace(rest), true
	case '[':
	default:
		return nil, "", false
	}
	for len(rest) > 0 && rest[0] == '[' {
		closeIdx := strings.IndexByte(rest, ']')
		if closeIdx < 0 {
			return nil, "", false
		}
		segment := strings.TrimSpace(rest[1:closeIdx])
		if segment == "" {
			return nil, "", false
		}
		chain = append(chain, segment)
		rest = rest[closeIdx+1:]
	}
	return chain, strings.TrimSpace(rest), true
}

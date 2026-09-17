package dsl

import (
	"fmt"
	"strings"
)

// RuleRef is a parsed `@<alias>.<rule>[<args>]` (or `@<rule>[<args>]`)
// reference, used inside a return-annotation payload to invoke a
// parametric rule template defined in some preset.
type RuleRef struct {
	Alias    string   // empty for local references (no `<alias>.`)
	RuleName string   // mandatory
	TypeArgs []string // raw textual arguments, split at top level
}

// RuleScope is the lookup surface a return-annotation parser consults
// when expanding `@rule` references. Aliases comes from `vow:import`
// alias annotations; Local hosts rules defined in the same preset as
// the annotated function.
type RuleScope struct {
	Aliases map[string]map[string]RuleDef
	Local   map[string]RuleDef
}

// Lookup resolves (alias, name) to a rule definition. Both forms of
// "not found" produce distinct errors so the diagnostic can point at
// the actual mistake.
//
// vow:cond * -> _, ErrUnknownReference | nil
func (s RuleScope) Lookup(alias, name string) (RuleDef, error) {
	if alias == "" {
		rule, ok := s.Local[name]
		if !ok {
			return RuleDef{}, fmt.Errorf("rule %q is not defined locally: %w", name, ErrUnknownReference)
		}
		return rule, nil
	}
	rules, ok := s.Aliases[alias]
	if !ok {
		return RuleDef{}, fmt.Errorf("alias %q is not in scope (missing vow:import?): %w", alias, ErrUnknownReference)
	}
	rule, ok := rules[name]
	if !ok {
		return RuleDef{}, fmt.Errorf("alias %q has no rule named %q: %w", alias, name, ErrUnknownReference)
	}
	return rule, nil
}

// Expand walks payload and replaces every `@alias.Rule` reference
// with its expanded body. Non-`@` segments are copied verbatim,
// so the result is a return-annotation string that can be fed
// directly to ParsePresetBody. Bare references (no alias) are
// emitted verbatim; downstream parsers decide how to handle them.
func (s RuleScope) Expand(payload string) (string, error) {
	var out strings.Builder
	i := 0
	for i < len(payload) {
		if payload[i] != '@' {
			out.WriteByte(payload[i])
			i++
			continue
		}
		ref, consumed, err := parseRuleRefAt(payload[i:])
		if err != nil {
			return "", err
		}
		if ref.Alias == "" {
			out.WriteString(payload[i : i+consumed])
			i += consumed
			continue
		}
		rule, err := s.Lookup(ref.Alias, ref.RuleName)
		if err != nil {
			return "", err
		}
		expanded, err := rule.ExpandBody(ref.TypeArgs)
		if err != nil {
			return "", fmt.Errorf("@%s: %w", ref.qualifiedName(), err)
		}
		out.WriteString(expanded)
		i += consumed
	}
	return out.String(), nil
}

func (r RuleRef) qualifiedName() string {
	if r.Alias == "" {
		return r.RuleName
	}
	return r.Alias + "." + r.RuleName
}

// parseRuleRefAt parses a `@<alias>.<rule>[<args>]` token starting at
// s[0] and returns the parsed reference plus the number of bytes
// consumed. The caller passes in the remaining slice; nothing outside
// the closing `]` is inspected.
//
// vow:cond * -> _, _, ErrSyntax | nil
func parseRuleRefAt(s string) (RuleRef, int, error) {
	if !strings.HasPrefix(s, "@") {
		return RuleRef{}, 0, fmt.Errorf("expected '@', got %q: %w", peek(s), ErrSyntax)
	}
	i := 1
	first, n := readIdent(s[i:])
	if n == 0 {
		return RuleRef{}, 0, fmt.Errorf("@: missing rule or alias name: %w", ErrSyntax)
	}
	i += n

	var ref RuleRef
	if i < len(s) && s[i] == '.' {
		ref.Alias = first
		i++
		second, m := readIdent(s[i:])
		if m == 0 {
			return RuleRef{}, 0, fmt.Errorf("@%s.: missing rule name: %w", ref.Alias, ErrSyntax)
		}
		ref.RuleName = second
		i += m
	} else {
		ref.RuleName = first
	}

	if i >= len(s) || s[i] != '[' {
		if ref.Alias == "" {
			// Zero-arg bare form: `@<name>` without brackets. The
			// grammar admits this shape for bare references that
			// take no type arguments. TypeArgs is left nil so
			// the resolver path treats the reference identically
			// to the bracket-with-empty-payload form
			// `@<name>[]`. Aliased references still require
			// `[args]` because preset rules are parameterised by
			// construction.
			return ref, i, nil
		}
		return RuleRef{}, 0, fmt.Errorf("@%s: missing '[<args>]': %w", ref.qualifiedName(), ErrSyntax)
	}
	i++ // step over '['

	// Find the matching ']' while respecting nested brackets and the
	// existing brace/paren/string scanner state.
	end, err := findMatchingBracket(s, i)
	if err != nil {
		return RuleRef{}, 0, fmt.Errorf("@%s: %w", ref.qualifiedName(), err)
	}
	inner := s[i:end]
	if strings.TrimSpace(inner) == "" {
		// `@rule[]` is the canonical reference shape for zero-param
		// rules (e.g. the any-sentinel escape hatch). The argument
		// list is empty but the brackets are still required so the
		// surface form stays uniform with parameterised references.
		ref.TypeArgs = nil
		return ref, end + 1, nil
	}
	args, err := splitAtTopLevel(inner, ',')
	if err != nil {
		return RuleRef{}, 0, fmt.Errorf("@%s args: %w", ref.qualifiedName(), err)
	}
	ref.TypeArgs = args
	return ref, end + 1, nil // include the closing ']'
}

// readIdent reads a Go-style identifier from the start of s.
//
// The first byte must satisfy isIdentStart (letter or underscore);
// subsequent bytes may also include digits. This rejects shapes like
// `@1foo` where the alias / rule name would start with a digit —
// such inputs return ("", 0) so the caller can surface a structured
// parse error.
func readIdent(s string) (string, int) {
	if len(s) == 0 || !isIdentStart(s[0]) {
		return "", 0
	}
	i := 1
	for i < len(s) && isIdentChar(s[i]) {
		i++
	}
	return s[:i], i
}

func isIdentChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '_':
		return true
	}
	return false
}

// findMatchingBracket scans s starting just after the opening `[` and
// returns the position of the matching `]`. Nested `[`/`]` increment
// and decrement a depth counter; parens and strings are transparently
// traversed using the same conventions as splitAtTopLevel so an
// argument like `map[string]int` does not throw off the counter.
//
// vow:cond * -> _, ErrSyntax | nil
func findMatchingBracket(s string, start int) (int, error) {
	depth := 1
	parens := 0
	inString := false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case inString:
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
		case c == '(':
			parens++
		case c == ')':
			parens--
		case c == '[' && parens == 0:
			depth++
		case c == ']' && parens == 0:
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("unbalanced '[': %w", ErrSyntax)
}

func peek(s string) string {
	if s == "" {
		return ""
	}
	return string(s[0])
}

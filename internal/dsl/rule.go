package dsl

import (
	"fmt"
	"strings"
)

// RuleDef declares a named, parametric template that other preset
// authors can reference via `@<alias>.<rule>[<args>]`. The body is a
// fragment of return-annotation syntax whose parameter names appear
// as bare identifiers (matching the names declared in Params, e.g.
// `T`, `E`); each is replaced with the corresponding type argument
// at the call site.
//
// Substitution happens at the *string* level: the expanded body is
// fed back through ParsePresetBody, so the expanded form must
// obey the usual syntax. This may be promoted this to an
// AST-level expansion if structural validation requires it.
type RuleDef struct {
	Params []string `yaml:"params"`
	Body   string   `yaml:"body"`
}

// ExpandBody applies the supplied type arguments to the rule template
// and returns the resulting return-annotation fragment.
//
// Template syntax: a parameter is named by its bare identifier within
// the body — e.g. a rule with `params: [T, E]` writes its body as
// `(nonzero T, nil) | (nil, nonzero E)`. Substitution is a single forward pass
// that walks identifiers; tokens that match a declared parameter are
// replaced with the corresponding argument verbatim, everything else
// is emitted as-is.
//
// The single-pass walk eliminates the order-dependent rewriting
// hazard of repeated `strings.ReplaceAll` (where an argument that
// happens to contain another parameter name would be expanded a
// second time).
//
// vow:cond * -> _, ErrInvalidGrammar | nil
func (r RuleDef) ExpandBody(args []string) (string, error) {
	if len(args) != len(r.Params) {
		return "", fmt.Errorf("rule expects %d type arguments, got %d: %w", len(r.Params), len(args), ErrInvalidGrammar)
	}
	params := make(map[string]string, len(r.Params))
	for i, p := range r.Params {
		params[p] = args[i]
	}
	return substituteBareParams(r.Body, params), nil
}

// substituteBareParams replaces every identifier token in body that
// names a declared parameter with the corresponding argument string,
// in a single left-to-right pass. Non-identifier characters and
// identifiers that are not parameter names are copied verbatim, so
// `errors.Is`, `nil`, and other syntactic tokens survive intact.
func substituteBareParams(body string, params map[string]string) string {
	var out strings.Builder
	i := 0
	for i < len(body) {
		if !isIdentStart(body[i]) {
			out.WriteByte(body[i])
			i++
			continue
		}
		j := i
		for j < len(body) && isIdentChar(body[j]) {
			j++
		}
		name := body[i:j]
		if value, ok := params[name]; ok {
			out.WriteString(value)
		} else {
			out.WriteString(name)
		}
		i = j
	}
	return out.String()
}

func isIdentStart(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c == '_':
		return true
	}
	return false
}

package analysis

import (
	"strconv"
	"strings"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// emitMarker is the doc-comment marker that declares how a function
// produces the subjects its callers track. The payload is a comma-
// separated list of specs, where each spec takes one of three
// shapes:
//
//   - name-based: `vow:emit ErrFoo` — the function may return the
//     named subject from one or more of its returns. The shape
//     carries no return-slot binding.
//   - positional: `vow:emit $1 ErrFoo` — the function returns the
//     named subject at the 1-based return slot, so the caller
//     binds the subject to a specific position even when the
//     signature exposes multiple return values of the same type.
//   - passthrough: `vow:emit err -> $1` — the function reads the
//     named parameter and emits it through the 1-based return
//     slot. The shape declares a wrap-style propagation surface
//     without naming a specific subject.
//
// The optional `[X][Y]...` chain qualifier names the signature
// slots the payload applies to: a single segment retargets at a
// callback parameter (`vow:emit[cb] Close`); a chained qualifier
// walks through nested higher-order callbacks
// (`vow:emit[outer][inner] Close`) so the declaration carries
// through a callback that itself exposes a callback slot.
const emitMarker = "vow:emit"

// EmitKind discriminates the three emit-spec shapes.
type EmitKind int

const (
	// EmitNameBased carries a bare subject identifier with no
	// return-slot binding.
	EmitNameBased EmitKind = iota
	// EmitPositional carries a subject identifier paired with a
	// `$N` return slot.
	EmitPositional
	// EmitPassthrough carries a named parameter paired with a
	// `$N` return slot via the `->` arrow. The shape declares a
	// wrap relationship rather than a specific subject.
	EmitPassthrough
)

// EmitSpec is one entry of a vow:emit marker payload. The Kind
// discriminator selects which of the slot fields carry the
// meaningful value:
//
//   - EmitNameBased: Subject holds the sentinel identifier;
//     FromParam stays empty and ReturnSlot stays zero.
//   - EmitPositional: Subject holds the sentinel identifier and
//     ReturnSlot holds the 1-based return position; FromParam
//     stays empty.
//   - EmitPassthrough: FromParam holds the parameter name and
//     ReturnSlot holds the 1-based return position; Subject stays
//     empty because the wrap relationship does not pin a specific
//     subject — the parameter binding establishes the propagation
//     surface.
type EmitSpec struct {
	Kind       EmitKind
	Subject    string
	FromParam  string
	ReturnSlot int
}

// String renders the spec back to its surface form so a parsed
// payload round-trips through the marker grammar.
func (s EmitSpec) String() string {
	switch s.Kind {
	case EmitPositional:
		return "$" + strconv.Itoa(s.ReturnSlot) + " " + s.Subject
	case EmitPassthrough:
		return s.FromParam + " -> $" + strconv.Itoa(s.ReturnSlot)
	}
	return s.Subject
}

// parseEmitMarkerLine inspects a trimmed comment-body line and
// reports the emit specs a vow:emit declaration carries, when
// present. The first return value is the optional signature-scope
// chain qualifier; an empty chain means the marker carried no
// `[scope]` and the declaration applies to the enclosing function
// in the usual way. Whitespace-only or empty payloads carry no
// specs and are reported as present=true with specs=nil so the
// caller can surface a diagnostic against an authored-but-empty
// marker instead of silently skipping it.
//
// Each comma-separated entry parses through parseEmitSpec, which
// dispatches by the presence of a `->` arrow (passthrough) and a
// `$N` prefix (positional). A malformed entry stops the parse at
// that spec and reports ok=false on parseEmitSpec; the caller
// drops the whole payload because a partial parse would credit a
// subset of the author's declarations without surfacing the
// authoring error.
func parseEmitMarkerLine(line string) (scope dsl.SubjectChain, specs []EmitSpec, present bool) {
	chain, body, ok := stripMarkerScopeChain(line, emitMarker)
	if !ok {
		return nil, nil, false
	}
	scope = dsl.SubjectChain(chain)
	body = trimInlineComment(body)
	if body == "" {
		return scope, nil, true
	}
	parts := strings.Split(body, ",")
	out := make([]EmitSpec, 0, len(parts))
	for _, p := range parts {
		entry := strings.TrimSpace(p)
		if entry == "" {
			continue
		}
		spec, ok := parseEmitSpec(entry)
		if !ok {
			return scope, nil, true
		}
		out = append(out, spec)
	}
	if len(out) == 0 {
		return scope, nil, true
	}
	return scope, out, true
}

// parseEmitSpec parses one comma-separated entry of a vow:emit
// payload into an EmitSpec. The dispatcher inspects the entry for
// the `->` arrow first: a present arrow routes to passthrough
// parsing (named parameter on the left, `$N` return slot on the
// right); an absent arrow routes to the subject branch where a
// `$N` prefix promotes the spec to EmitPositional and a bare
// identifier stays EmitNameBased. The helper reports ok=false on
// a malformed entry so the caller drops the whole payload rather
// than crediting a partial parse.
func parseEmitSpec(entry string) (EmitSpec, bool) {
	if arrowIdx := strings.Index(entry, "->"); arrowIdx >= 0 {
		left := strings.TrimSpace(entry[:arrowIdx])
		right := strings.TrimSpace(entry[arrowIdx+2:])
		if left == "" || right == "" {
			return EmitSpec{}, false
		}
		slot, ok := parsePositionalReturnSlot(right)
		if !ok {
			return EmitSpec{}, false
		}
		return EmitSpec{Kind: EmitPassthrough, FromParam: left, ReturnSlot: slot}, true
	}
	fields := strings.Fields(entry)
	switch len(fields) {
	case 1:
		if strings.HasPrefix(fields[0], "$") {
			// A bare `$N` token without a paired subject identifier
			// is not a complete positional spec; the form needs a
			// subject after the slot. Rejecting it here keeps the
			// name-based branch from silently registering `$N` as a
			// subject name and surfacing a misleading "body never
			// returns $N" diagnostic downstream.
			return EmitSpec{}, false
		}
		return EmitSpec{Kind: EmitNameBased, Subject: fields[0]}, true
	case 2:
		slot, ok := parsePositionalReturnSlot(fields[0])
		if !ok {
			return EmitSpec{}, false
		}
		return EmitSpec{Kind: EmitPositional, Subject: fields[1], ReturnSlot: slot}, true
	}
	return EmitSpec{}, false
}

// parsePositionalReturnSlot reads a `$N` token where N is a
// 1-based positive integer and returns the slot index. A token
// that lacks the `$` prefix, carries a non-integer body, or
// resolves to a non-positive value reports ok=false so the caller
// rejects the entry.
func parsePositionalReturnSlot(tok string) (int, bool) {
	if !strings.HasPrefix(tok, "$") {
		return 0, false
	}
	n, err := strconv.Atoi(tok[1:])
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// trimInlineComment drops everything from the first `//` token
// onward in payload so the parser tolerates the inline `// want`
// annotation that analysistest fixtures place on the same line as
// the marker. A leading `//` (which the lineCommentBody helper
// already strips) cannot appear in payload, so the helper only
// looks past the first non-whitespace character.
func trimInlineComment(payload string) string {
	if idx := strings.Index(payload, "//"); idx >= 0 {
		payload = payload[:idx]
	}
	return strings.TrimSpace(payload)
}

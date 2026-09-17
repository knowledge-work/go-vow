package dsl

import (
	"fmt"
	"strings"
)

// PositionDecl is the parsed nullness contract attached to a single
// signature position (receiver, parameter, or return). The decl
// carries an optional Nullness and an optional nest body.
//
// A nil *PositionDecl represents the platform state — the position
// is at platform, vow does not track it, and there is no token an
// author writes to spell "platform" explicitly. The parser yields
// a nil pointer for an empty position; downstream code keeps the
// nil-pointer-as-platform convention so the absence of a decl and
// "the author left this position out" share one representation.
type PositionDecl struct {
	Nullness Nullness
	// InnerLayers carries the nullness of the layers underneath
	// Nullness, outermost first. A type holds nil at one layer per
	// dereference the value admits, so `**Value` carries two and
	// `!?` pins the outer pointer non-nil while admitting nil at the
	// pointer it addresses. Nullness alone describes the outermost
	// layer, which is every layer a single-pointer type has, so this
	// stays empty for the common shapes.
	InnerLayers []Nullness
	// Nest carries the optional nest body — slice (`[]`), map
	// (`[key]`), or generic-type-parameter list (`[T1, T2, ...]`) —
	// that may follow the nullness token. The parser leaves Nest
	// nil when the position carries no nest body; downstream
	// consumers inspect len(Nest.InnerDecls) to read the shape and
	// rely on container-type information for the single-element
	// disambiguation.
	Nest *NestDecl
}

// Nullness enumerates the three nullness states that a position
// decl carries. NullnessNone is the empty token (no `!` / `?`),
// reserved for nest-only decls here; the current
// parser raises an error if it encounters NullnessNone outside a
// nest context.
type Nullness int

const (
	NullnessNone     Nullness = iota // no explicit nullness token
	NullnessNonNil                   // `!`
	NullnessNillable                 // `?`
)

// String renders the nullness back to its surface form.
func (n Nullness) String() string {
	switch n {
	case NullnessNonNil:
		return "!"
	case NullnessNillable:
		return "?"
	}
	return ""
}

// NestDecl carries the parsed nest body. InnerDecls holds the
// comma-separated decls between `[` and `]`; Value holds the
// trailing decl after `]`. The shape decoded by the parser is
// uniform across slice, map, and generic surfaces; the analyzer
// reads container-type information to discriminate the single-
// inner-decl case.
type NestDecl struct {
	InnerDecls []*PositionDecl
	Value      *PositionDecl
}

// NilSignature is the parsed shape of a `vow:nil` payload. The
// signature mirrors a Go function signature so each layer of the
// declaration aligns visually with the Go declaration above which
// the marker sits.
//
// Subject names a signature-scope identifier the declaration applies
// to when the marker carried a `[subject]` scope qualifier (e.g.
// `vow:nil[cb] !` redirects the contract at the parameter named
// `cb`). A chained scope (`vow:nil[outer][inner] (!)`) walks
// through nested higher-order callbacks and retargets the
// contract at the innermost callback's signature. An empty
// Subject means the marker carried no scope and the declaration
// applies to the enclosing function in the usual way.
type NilSignature struct {
	Recv    *PositionDecl   // nil when the source carried no recv-decl prefix
	Params  []*PositionDecl // index-aligned with the Go signature's parameter list
	Returns []*PositionDecl // index-aligned with the Go signature's return list
	Subject SubjectChain    // nil or empty when the marker carried no `[subject]` scope
}

// ParseNilSignature parses a `vow:nil` payload into a NilSignature.
// The accepted form is:
//
//	[ <recv-decl> "." ] "(" [ decl { "," decl } ] ")" [ <return-list> ]
//
// where every <decl> is an optional run of nullness tokens — one per
// layer of the type that can hold nil, outermost first — followed by
// an optional nest body. The nest body uses one unified shape — `[`
// optional inner decls separated by commas `]` optional outer decl —
// that the parser accepts for every nest surface (slice, map,
// generic); analyzer-side container-type resolution disambiguates
// the surfaces that share the same payload.
//
// vow:cond * -> _, ErrSyntax | nil
func ParseNilSignature(payload string) (*NilSignature, error) {
	src := strings.TrimSpace(payload)
	if src == "" {
		return nil, fmt.Errorf("vow:nil payload is empty: %w", ErrSyntax)
	}
	p := &nilSigParser{src: src}
	sig, err := p.parse()
	if err != nil {
		return nil, fmt.Errorf("vow:nil %q: %w", payload, err)
	}
	return sig, nil
}

// ParseFieldNilDecl parses a single decl payload as it appears on a
// struct-field `vow:nil` marker. The accepted form is a single decl
// (optional nullness tokens plus optional nest body) — the same
// grammar a parameter or return slot accepts on the signature-mirror
// surface. An empty payload is ill-formed at the field surface
// because the author wrote `vow:nil` with nothing to declare; the
// platform state is spelled by omitting the marker entirely.
//
// vow:cond * -> _, ErrSyntax | nil
func ParseFieldNilDecl(payload string) (*PositionDecl, error) {
	src := strings.TrimSpace(payload)
	if src == "" {
		return nil, fmt.Errorf("vow:nil field payload is empty: %w", ErrSyntax)
	}
	decl, err := parseDeclFromString(src, "field")
	if err != nil {
		return nil, fmt.Errorf("vow:nil %q: %w", payload, err)
	}
	return decl, nil
}

type nilSigParser struct {
	src string
	pos int
}

// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *nilSigParser) parse() (*NilSignature, error) {
	sig := &NilSignature{}
	recv, hasRecv, err := p.tryRecv()
	if err != nil {
		return nil, err
	}
	if hasRecv {
		sig.Recv = recv
	}
	params, err := p.parseParamList()
	if err != nil {
		return nil, err
	}
	sig.Params = params
	p.skipSpace()
	if !p.atEnd() {
		returns, err := p.parseReturnList()
		if err != nil {
			return nil, err
		}
		sig.Returns = returns
	}
	p.skipSpace()
	if !p.atEnd() {
		return nil, fmt.Errorf("unexpected trailing input at position %d: %w", p.pos, ErrSyntax)
	}
	return sig, nil
}

// tryRecv consumes a recv-decl + `.` prefix when present. Returns
// (decl, true, nil) when a recv-decl is consumed, (nil, false, nil)
// when no recv-decl appears (the next non-space byte is `(`).
//
// vow:cond * -> _, _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *nilSigParser) tryRecv() (*PositionDecl, bool, error) {
	p.skipSpace()
	dotIdx := p.findRecvDot()
	if dotIdx < 0 {
		return nil, false, nil
	}
	prefix := strings.TrimSpace(p.src[p.pos:dotIdx])
	decl, err := parseDeclFromString(prefix, "recv-decl")
	if err != nil {
		return nil, false, err
	}
	p.pos = dotIdx + 1
	return decl, true, nil
}

// findRecvDot returns the index of the `.` that separates a
// recv-decl from the parameter list, or -1 when the payload has no
// recv-decl prefix. The search stops at the first `(` so a `.` that
// would appear inside the parameter list never accidentally
// matches.
func (p *nilSigParser) findRecvDot() int {
	for i := p.pos; i < len(p.src); i++ {
		switch p.src[i] {
		case '.':
			return i
		case '(':
			return -1
		}
	}
	return -1
}

// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *nilSigParser) parseParamList() ([]*PositionDecl, error) {
	p.skipSpace()
	if p.atEnd() || p.src[p.pos] != '(' {
		return nil, fmt.Errorf("expected `(` at position %d: %w", p.pos, ErrSyntax)
	}
	p.pos++
	end, err := p.findMatchingParen()
	if err != nil {
		return nil, err
	}
	inner := p.src[p.pos:end]
	p.pos = end + 1
	return parseDeclListFromString(inner, "param")
}

// vow:cond * -> _, ErrSyntax | nil
func (p *nilSigParser) findMatchingParen() (int, error) {
	depth := 1
	for i := p.pos; i < len(p.src); i++ {
		switch p.src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("unbalanced `(`: %w", ErrSyntax)
}

// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *nilSigParser) parseReturnList() ([]*PositionDecl, error) {
	rest := strings.TrimSpace(p.src[p.pos:])
	p.pos = len(p.src)
	return parseDeclListFromString(rest, "return")
}

func (p *nilSigParser) skipSpace() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *nilSigParser) atEnd() bool {
	return p.pos >= len(p.src)
}

// parseDeclListFromString splits a comma-separated decl list, with
// platform slots represented by empty entries. Position name is
// used for error messages so the surface text points back at the
// offending layer.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func parseDeclListFromString(src, label string) ([]*PositionDecl, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, nil
	}
	parts, err := splitDeclListAtTopLevel(src, ',')
	if err != nil {
		return nil, fmt.Errorf("%s list: %w", label, err)
	}
	out := make([]*PositionDecl, 0, len(parts))
	for i, part := range parts {
		decl, err := parseDeclFromString(strings.TrimSpace(part), fmt.Sprintf("%s %d", label, i))
		if err != nil {
			return nil, err
		}
		out = append(out, decl)
	}
	return out, nil
}

// splitDeclListAtTopLevel splits s on every occurrence of sep that
// sits outside both parenthesised groups and bracketed nest bodies.
// The bracket-aware behavior keeps a multi-element nest payload
// (for instance `[<decl1>, <decl2>]`) intact during the outer comma
// split.
//
// vow:cond * -> _, ErrSyntax | nil
func splitDeclListAtTopLevel(s string, sep byte) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []string
	parens := 0
	brackets := 0
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '(':
			parens++
		case c == ')':
			parens--
			if parens < 0 {
				return nil, fmt.Errorf("unbalanced ): %w", ErrSyntax)
			}
		case c == '[':
			brackets++
		case c == ']':
			brackets--
			if brackets < 0 {
				return nil, fmt.Errorf("unbalanced ]: %w", ErrSyntax)
			}
		case c == sep && parens == 0 && brackets == 0:
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	if parens != 0 {
		return nil, fmt.Errorf("unbalanced (: %w", ErrSyntax)
	}
	if brackets != 0 {
		return nil, fmt.Errorf("unbalanced [: %w", ErrSyntax)
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out, nil
}

// parseDeclFromString reads a single decl. An empty input yields a
// nil pointer (platform slot). The decl is an optional nullness
// token (`!` / `?`) followed by an optional nest body in the
// unified `[` inner-decls `]` outer-decl shape. Nests compose
// recursively so chained nest bodies parse without depth limits.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func parseDeclFromString(src, label string) (*PositionDecl, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, nil
	}
	dp := &declParser{src: src, label: label}
	decl, err := dp.parse()
	if err != nil {
		return nil, err
	}
	if decl == nil && !dp.atEnd() {
		return nil, fmt.Errorf("%s: expected `!` / `nonnil`, `?` / `nil`, or `[`, got %q: %w", label, dp.src[dp.pos:], ErrSyntax)
	}
	if !dp.atEnd() {
		return nil, fmt.Errorf("%s: trailing text after decl %q: %w", label, dp.src[dp.pos:], ErrSyntax)
	}
	if decl == nil {
		return nil, fmt.Errorf("%s: expected `!` / `nonnil`, `?` / `nil`, or an empty slot, got %q: %w", label, src, ErrSyntax)
	}
	return decl, nil
}

// declParser walks a single decl byte by byte so an optional
// nullness token and an optional nest body fold into one
// recursive shape.
type declParser struct {
	src   string
	pos   int
	label string
}

// parse consumes a single decl: optional nullness + optional
// nest. Returns nil when no token is consumed (an empty slot
// stays platform); returns an error when the input starts with a
// token but does not match a recognised shape.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *declParser) parse() (*PositionDecl, error) {
	p.skipSpace()
	if p.atEnd() {
		return nil, nil
	}
	decl := &PositionDecl{}
	if nullness, ok := p.consumeNullness(); ok {
		decl.Nullness = nullness
		decl.InnerLayers = p.consumeInnerLayers()
	}
	p.skipSpace()
	if !p.atEnd() && p.peek() == '[' {
		nest, err := p.parseNest()
		if err != nil {
			return nil, err
		}
		decl.Nest = nest
	} else if decl.Nullness == NullnessNone {
		// No nullness token and no nest — the caller treats this as
		// "no recognised content" and surfaces a structured error.
		return nil, nil
	}
	return decl, nil
}

// parseNest reads a unified `[` inner-decls `]` outer-decl body.
// The inner-decls are zero or more comma-separated position decls;
// the surfaces a unified payload supports — slice (`[]`), map
// (`[<decl>]`), single-type-parameter generic (`[<decl>]`),
// multi-type-parameter generic (`[<decl1>, <decl2>, ...]`) — all
// parse into the same shape, and analyzer-side container-type
// resolution discriminates the single-inner-decl case. A trailing
// comma before `]` raises a structured error so a malformed input
// surfaces a precise complaint.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *declParser) parseNest() (*NestDecl, error) {
	if p.atEnd() || p.peek() != '[' {
		return nil, fmt.Errorf("%s: nest: expected `[`: %w", p.label, ErrSyntax)
	}
	p.pos++ // consume `[`
	p.skipSpace()
	if p.atEnd() {
		return nil, fmt.Errorf("%s: nest: unbalanced `[`: %w", p.label, ErrSyntax)
	}
	var inner []*PositionDecl
	if p.peek() != ']' {
		for {
			decl, err := p.parse()
			if err != nil {
				return nil, err
			}
			inner = append(inner, decl)
			p.skipSpace()
			if p.atEnd() {
				return nil, fmt.Errorf("%s: nest: unbalanced `[`: %w", p.label, ErrSyntax)
			}
			if p.peek() == ']' {
				break
			}
			if p.peek() != ',' {
				return nil, fmt.Errorf("%s: nest: expected `,` or `]`: %w", p.label, ErrSyntax)
			}
			p.pos++ // consume `,`
			p.skipSpace()
			if p.atEnd() || p.peek() == ']' {
				return nil, fmt.Errorf("%s: nest: trailing `,` before `]`: %w", p.label, ErrSyntax)
			}
		}
	}
	p.pos++ // consume `]`
	value, err := p.parse()
	if err != nil {
		return nil, err
	}
	return &NestDecl{InnerDecls: inner, Value: value}, nil
}

// consumeNullness reads the optional nullness token of the outermost
// layer. Both the symbol form (`!` / `?`) and the keyword form
// (`nonnil` / `nil`) resolve to the same Nullness value; the canonical
// printer keeps emitting the symbol form because the symbol is the
// high-frequency surface on the position-decl mirror, while the
// keyword form is available for authors who prefer the word at call
// sites where the punctuation reads less clearly.
func (p *declParser) consumeNullness() (Nullness, bool) {
	if p.atEnd() {
		return NullnessNone, false
	}
	if nullness, ok := p.consumeNullnessKeyword(); ok {
		return nullness, true
	}
	switch p.peek() {
	case '!':
		p.pos++
		return NullnessNonNil, true
	case '?':
		p.pos++
		return NullnessNillable, true
	}
	return NullnessNone, false
}

// consumeNullnessKeyword accepts a keyword form (`nonnil` / `nil`)
// when the lookahead starts with the keyword and the byte that
// follows is a decl terminator. The terminator check stops the
// parser from matching the keyword as a prefix of an unrelated
// identifier (e.g. `nillable`) that might appear if the grammar
// grows.
func (p *declParser) consumeNullnessKeyword() (Nullness, bool) {
	if p.matchKeyword("nonnil") {
		p.pos += len("nonnil")
		return NullnessNonNil, true
	}
	if p.matchKeyword("nil") {
		p.pos += len("nil")
		return NullnessNillable, true
	}
	return NullnessNone, false
}

// matchKeyword reports whether the current position holds the
// keyword followed by a decl terminator. Terminators are end of
// input, ASCII whitespace, the nest brackets (`[` opens a nest
// after the nullness; `]` closes the inner-decls list the
// recursive parse is reading), and the list bytes (`,` / `)`)
// that the outer splitter strips before this parser runs — the
// list bytes are included so the predicate stays correct even if
// a caller hands a payload without the outer split.
func (p *declParser) matchKeyword(kw string) bool {
	if p.pos+len(kw) > len(p.src) {
		return false
	}
	if p.src[p.pos:p.pos+len(kw)] != kw {
		return false
	}
	if p.pos+len(kw) == len(p.src) {
		return true
	}
	switch p.src[p.pos+len(kw)] {
	case ' ', '\t', ',', ')', '[', ']':
		return true
	}
	return false
}

// consumeInnerLayers reads the nullness symbols that follow the
// outermost one, in outermost-first order, and returns them in that
// order. Only the symbol form stacks: `nonnilnonnil` has no readable
// boundary, so a keyword spells exactly one layer. The symbols must be
// adjacent — a space ends the run and whatever follows is left for the
// caller's terminator check to reject.
func (p *declParser) consumeInnerLayers() []Nullness {
	var layers []Nullness
	for !p.atEnd() {
		switch p.peek() {
		case '!':
			layers = append(layers, NullnessNonNil)
		case '?':
			layers = append(layers, NullnessNillable)
		default:
			return layers
		}
		p.pos++
	}
	return layers
}

func (p *declParser) skipSpace() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *declParser) atEnd() bool {
	return p.pos >= len(p.src)
}

func (p *declParser) peek() byte {
	return p.src[p.pos]
}

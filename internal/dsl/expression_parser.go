package dsl

import (
	"fmt"
	"strconv"
)

// parseExpression parses one operand of a logical arrow-rule. The
// surface recogniser currently accepts two shapes:
//
//   - A nil-check: `<reference> == nil` / `<reference> != nil`.
//   - A comparison: `<reference> <op> <literal>` where the operator
//     is one of `==`, `!=`, `<`, `<=`, `>`, `>=` and the literal is
//     an integer, string, or boolean keyword. (The `nil` literal is
//     special-cased above so `x == nil` produces ExprNilCheck.)
//
// The AST reserves an ExprBareRef shape as an unused slot. The
// surface parser does not produce that shape; every operand must
// include a comparison operator and a right-hand side.
//
// Token streams that the caller passes here have already been
// produced by the `lexCond` mode, so the postfix steps (`.field`,
// `[idx]`, `["key"]`) surface as distinct tokens. The parser
// rejects any leftover token after the operand because the arrow-
// rule grammar expects each half to be exactly one expression.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func parseExpression(toks []token) (*Expression, error) {
	toks = stripTrailingEOF(toks)
	if len(toks) == 0 {
		return nil, fmt.Errorf("empty expression: %w", ErrSyntax)
	}
	p := exprParser{toks: toks}
	expr, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	if !p.atEnd() {
		return nil, fmt.Errorf("unexpected token %q after expression at offset %d: %w", p.peek().Val, p.peek().Pos, ErrSyntax)
	}
	return expr, nil
}

// exprParser is a tiny recursive-descent parser over a pre-tokenised
// operand stream. It mirrors the existing return-annotation parser
// in shape — index-based cursor with peek/advance — so the
// logical-arrow path stays familiar to anyone who has read
// parseAnnotation.
type exprParser struct {
	toks []token
	pos  int
}

func (p *exprParser) peek() token    { return p.toks[p.pos] }
func (p *exprParser) advance() token { t := p.toks[p.pos]; p.pos++; return t }
func (p *exprParser) atEnd() bool    { return p.pos >= len(p.toks) }

// parseOperand drives the operand grammar: parse the postfix
// reference, then require a comparison operator. The grammar
// reserves the ExprBareRef shape on the AST as an unused slot;
// the surface parser rejects a standalone reference so authors
// get a precise diagnostic instead of a confusing downstream
// error.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *exprParser) parseOperand() (*Expression, error) {
	ref, err := p.parseReference()
	if err != nil {
		return nil, err
	}
	if p.atEnd() {
		return nil, fmt.Errorf("bare reference %q is not accepted; pair it with a comparison operator: %w", ref.String(), ErrSyntax)
	}
	op, ok := compareOpFromToken(p.peek().Kind)
	if !ok {
		return nil, fmt.Errorf("expected comparison operator after reference, got %q at offset %d: %w", p.peek().Val, p.peek().Pos, ErrSyntax)
	}
	p.advance()
	return p.parseComparisonRHS(ref, op)
}

// parseReference consumes a head identifier (or a `$<name>` /
// `$<digit>` positional reference) followed by an optional chain of
// postfix steps. The chain terminates at the first token that is
// neither a dot nor an opening bracket.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *exprParser) parseReference() (Reference, error) {
	if p.atEnd() {
		return Reference{}, fmt.Errorf("expected reference, got end of expression: %w", ErrSyntax)
	}
	var ref Reference
	switch p.peek().Kind {
	case tDollar:
		p.advance()
		if p.atEnd() || p.peek().Kind != tIdent {
			return Reference{}, fmt.Errorf("`$` must be followed by an identifier or digits at offset %d: %w", p.toks[p.pos-1].Pos, ErrSyntax)
		}
		ref = Reference{Kind: RefDollar, Name: p.advance().Val}
	case tIdent:
		ref = Reference{Kind: RefIdent, Name: p.advance().Val}
	default:
		return Reference{}, fmt.Errorf("expected identifier or `$<name>` at offset %d, got %q: %w", p.peek().Pos, p.peek().Val, ErrSyntax)
	}
	for !p.atEnd() {
		switch p.peek().Kind {
		case tDot:
			dot := p.advance()
			if p.atEnd() || p.peek().Kind != tIdent {
				return Reference{}, fmt.Errorf("`.` must be followed by a field name at offset %d: %w", dot.Pos, ErrSyntax)
			}
			fieldTok := p.advance()
			if !isGoIdentifier(fieldTok.Val) {
				return Reference{}, fmt.Errorf("field name %q at offset %d is not a Go identifier: %w", fieldTok.Val, fieldTok.Pos, ErrSyntax)
			}
			ref.Path = append(ref.Path, PathStep{Kind: StepField, Name: fieldTok.Val})
		case tLBracket:
			step, err := p.parseIndexStep()
			if err != nil {
				return Reference{}, err
			}
			ref.Path = append(ref.Path, step)
		default:
			return ref, nil
		}
	}
	return ref, nil
}

// parseIndexStep consumes `[<key>]` and classifies the key as one
// of three shapes. A decimal-integer literal (with an optional
// leading minus) becomes StepIndex. A double-quoted string literal
// becomes StepStringKey. A Go identifier becomes StepIdentKey so
// the resolver later looks the name up against the enclosing
// signature scope. Other lexically-`tIdent` runs — hexadecimal
// literals, leading-zero decimals, mixed-shape tokens — are
// rejected at parse time so the surface stays unambiguous and
// String round-trips byte-for-byte through the rendered form.
//
// vow:cond * -> _, ErrSyntax | nil
func (p *exprParser) parseIndexStep() (PathStep, error) {
	open := p.advance() // tLBracket
	if p.atEnd() {
		return PathStep{}, fmt.Errorf("unclosed `[` at offset %d: %w", open.Pos, ErrSyntax)
	}
	var step PathStep
	switch p.peek().Kind {
	case tStr:
		step = PathStep{Kind: StepStringKey, Name: p.advance().Val}
	case tIdent:
		keyTok := p.advance()
		val := keyTok.Val
		if idx, ok := parseDecimalIntLiteral(val); ok {
			step = PathStep{Kind: StepIndex, Index: idx}
		} else if isGoIdentifier(val) {
			step = PathStep{Kind: StepIdentKey, Name: val}
		} else {
			return PathStep{}, fmt.Errorf("index key %q at offset %d is neither a decimal integer literal nor a Go identifier: %w", val, keyTok.Pos, ErrSyntax)
		}
	default:
		return PathStep{}, fmt.Errorf("expected index key after `[` at offset %d, got %q: %w", open.Pos, p.peek().Val, ErrSyntax)
	}
	if p.atEnd() || p.peek().Kind != tRBracket {
		return PathStep{}, fmt.Errorf("expected `]` to close index opened at offset %d: %w", open.Pos, ErrSyntax)
	}
	p.advance() // tRBracket
	return step, nil
}

// parseComparisonRHS consumes the right-hand side after the
// comparison operator. A bare `nil` token produces ExprNilCheck
// when no `|` continuation follows; a single literal produces
// ExprComparison with one member; a `|`-separated sequence
// produces ExprComparison with multiple members where each
// alternative is a literal or the bare `nil` token. Equality and
// inequality are the only operators allowed against `nil` or
// against a multi-member sum; the ordered comparisons (`<`, `<=`,
// `>`, `>=`) reject both shapes at parse time so the diagnostic
// surfaces the mismatch on the line that wrote it rather than
// during downstream evaluation.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *exprParser) parseComparisonRHS(ref Reference, op CompareOp) (*Expression, error) {
	first, err := p.parseComparisonMember(op)
	if err != nil {
		return nil, err
	}
	members := []ExprMember{first}
	for !p.atEnd() && p.peek().Kind == tPipe {
		p.advance()
		next, err := p.parseComparisonMember(op)
		if err != nil {
			return nil, err
		}
		members = append(members, next)
	}
	if len(members) > 1 {
		if op != OpEq && op != OpNeq {
			return nil, fmt.Errorf("ordered comparison `%s` does not accept a value-level sum on the right-hand side; only `==` and `!=` distribute over `|`: %w", op.String(), ErrInvalidGrammar)
		}
		return &Expression{Kind: ExprComparison, Ref: ref, Op: op, Members: members}, nil
	}
	if first.Nil {
		return &Expression{Kind: ExprNilCheck, Ref: ref, Op: op}, nil
	}
	return &Expression{Kind: ExprComparison, Ref: ref, Op: op, Members: members}, nil
}

// parseComparisonMember consumes one alternative of a comparison
// right-hand-side sum and returns the member. The accepted shapes
// are the bare `nil` token (ExprMember{Nil: true}) and a non-nil
// literal — an identifier-shaped token (`0`, `0xff`, `true`,
// `false`, `tag`, ...) or a quoted string. Ordered comparisons
// reject `nil` at this layer so the diagnostic identifies the
// nil member rather than the sum surface.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *exprParser) parseComparisonMember(op CompareOp) (ExprMember, error) {
	if p.atEnd() {
		return ExprMember{}, fmt.Errorf("expected literal after comparison operator: %w", ErrSyntax)
	}
	tok := p.advance()
	if tok.Kind == tIdent && tok.Val == "nil" {
		if op != OpEq && op != OpNeq {
			return ExprMember{}, fmt.Errorf("ordered comparison `%s nil` is not meaningful; use `== nil` or `!= nil`: %w", op.String(), ErrInvalidGrammar)
		}
		return ExprMember{Nil: true}, nil
	}
	if tok.Kind != tIdent && tok.Kind != tStr {
		return ExprMember{}, fmt.Errorf("expected literal after `%s`, got %q at offset %d: %w", op.String(), tok.Val, tok.Pos, ErrSyntax)
	}
	return ExprMember{Literal: tok.Val}, nil
}

// compareOpFromToken maps a token kind to the matching CompareOp.
// Returns (_, false) when the token does not start a comparison so
// the caller can surface a grammar diagnostic instead of silently
// mis-parsing.
func compareOpFromToken(k tokKind) (CompareOp, bool) {
	switch k {
	case tEq:
		return OpEq, true
	case tNeq:
		return OpNeq, true
	case tLt:
		return OpLt, true
	case tLte:
		return OpLte, true
	case tGt:
		return OpGt, true
	case tGte:
		return OpGte, true
	}
	return 0, false
}

// parseDecimalIntLiteral reports whether the token spelling is a
// decimal integer literal (optionally signed) and returns the
// parsed value. Hexadecimal (`0xff`), octal (`0o7`), and leading-
// zero (`01`) shapes return ok=false so the caller surfaces a
// structured diagnostic instead of silently re-classifying the
// token as an identifier-typed key.
func parseDecimalIntLiteral(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	body := s
	if body[0] == '-' || body[0] == '+' {
		body = body[1:]
	}
	if body == "" {
		return 0, false
	}
	if len(body) > 1 && body[0] == '0' {
		return 0, false
	}
	for i := 0; i < len(body); i++ {
		if body[i] < '0' || body[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// isGoIdentifier reports whether s is a syntactic Go identifier.
// Used by the postfix step parser so a non-identifier run after
// `.` or inside `[...]` is rejected at parse time.
func isGoIdentifier(s string) bool {
	if s == "" {
		return false
	}
	first := s[0]
	if !(first == '_' || (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			continue
		}
		return false
	}
	return true
}

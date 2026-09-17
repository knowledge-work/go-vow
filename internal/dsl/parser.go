package dsl

import (
	"fmt"
	"strings"
)

// This file implements the precedence-based return-annotation
// parser that backs ParsePresetBody.
//
// Operator precedence (lowest binding first, increasing rightward):
//
//	';'   — annotation separator (reserved; not parsed)
//	','   — position separator
//	'|'   — sum alternation within a position
//	'?'   — sum-level / term-level qualifier suffix
//	'!'   — obligation marker (orthogonal to nil-handling)
//	'()'  — grouping (overrides precedence; also doubles as tuple)
//
// Grammar (lowest precedence first):
//
//	annotation := positions
//	positions  := alt ( ',' alt )*
//	alt        := term ( '|' term )*
//	term       := '(' positions ')'                  // tuple or grouping; see below
//	            | predicate type-opt
//	            | type
//	            | placeholder
//	            | literal
//	placeholder := '_'
//	predicate   := '!' predicate-atom | predicate-atom
//	predicate-atom := 'zero' | 'nil'
//	type        := <Go-type-like identifier with dots, stars, brackets>
//	literal     := nil-keyword | bool | int | string
//
// `(...)` plays two roles distinguished by a token-stream
// lookahead: an outer-depth `,` first marks the span as a tuple
// (`(A, B)` builds a TupleMember), while an outer-depth `|`
// first marks it as a grouping (`(A | B)?` builds a DirectSum).
//
// The parser accepts the `T?` suffix qualifier and folds it into
// the canonical AST shape the downstream analyzer consumes. The
// non-zero invariant is spelt as the `nonzero T` prefix predicate;
// the parser rejects the `T!` suffix with a diagnostic naming the
// `nonzero T` replacement.
//
// The any-sentinel matcher (`<sentinel>`) is rejected with a
// structured diagnostic that guides authors toward listing
// concrete sentinel references on `vow:cond`. Other
// angle-bracketed tokens are likewise rejected because the surface
// has no other angle-bracket production.

// parseReturnAnnotationNew is the entry point of the precedence
// parser. It is wired into ParsePresetBody so callers
// share a single public API. The returned ReturnAnnotation
// always carries either Positions or TupleSum so downstream code
// in internal/analysis has a single consumption path.
func parseReturnAnnotationNew(payload string) (*ReturnAnnotation, error) {
	toks, err := tokenize(payload, lexReturn)
	if err != nil {
		return nil, fmt.Errorf("vow:cond %q: %w", payload, err)
	}
	p := parser{src: payload, toks: toks}
	annotation, err := p.parseAnnotation()
	if err != nil {
		return nil, fmt.Errorf("vow:cond %q: %w", payload, err)
	}
	return annotation, nil
}

// ----------------------------------------------------------------------
// Tokens
// ----------------------------------------------------------------------

// lexerMode selects which token vocabulary the lexer recognises.
// The `vow:cond` surface keeps the historical behaviour where
// square brackets glue composite types into a single identifier run.
// The `vow:cond` surface activates the expanded grammar with
// independent bracket / dot / dollar / comparison / logical-arrow
// tokens so postfix expressions and logical implications parse as
// distinct sequences.
type lexerMode int

const (
	lexReturn lexerMode = iota
	lexCond
)

type tokKind int

const (
	tEOF tokKind = iota
	tComma
	tSemicolon // rule separator `;`
	tPipe
	tLParen
	tRParen
	tBang
	tQuestion
	tArrow // structural separator `->`
	tStar  // complete wildcard `*` (matches every value, label-aware
	//        wildcards `_` are spelt as tIdent and handled later)
	tIdent // identifier-shaped run; refined by the parser
	tStr   // double-quoted string literal, quotes included

	// Tokens below are produced only by the `vow:cond` lexer mode.
	// The `vow:cond` mode never emits them so the historical token
	// stream that parseAnnotation consumes stays unchanged.
	tLBracket // `[`, postfix index opener
	tRBracket // `]`, postfix index closer
	tDot      // `.`, postfix field accessor
	tDollar   // `$`, return-positional reference prefix
	tEq       // `==`
	tNeq      // `!=`
	tLt       // `<`
	tLte      // `<=`
	tGt       // `>`
	tGte      // `>=`
	tImply    // `=>`, logical implication
	tEquiv    // `<=>`, logical biconditional
)

type token struct {
	Kind tokKind
	Val  string
	Pos  int
}

// tokenize produces a flat token stream. The lexer is deliberately
// thin — it treats every non-punctuation run as a single tIdent and
// leaves keyword/literal/type classification to the parser. `<` and
// `>` are NOT stop chars in `lexReturn` so the `<sentinel>`
// sentinel-matcher token is lexed atomically and rejected later by
// the parser with a precise diagnostic. The `lexCond` mode promotes
// brackets, comparison operators, logical arrows, and a handful of
// other punctuation runs to dedicated token kinds so postfix
// expressions and logical implications surface as token sequences.
//
// vow:cond * -> _, (ErrSyntax | _)?
func tokenize(s string, mode lexerMode) ([]token, error) {
	var out []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
			continue
		case c == ',':
			out = append(out, token{Kind: tComma, Val: ",", Pos: i})
			i++
		case c == ';':
			out = append(out, token{Kind: tSemicolon, Val: ";", Pos: i})
			i++
		case c == '|':
			out = append(out, token{Kind: tPipe, Val: "|", Pos: i})
			i++
		case c == '(':
			out = append(out, token{Kind: tLParen, Val: "(", Pos: i})
			i++
		case c == ')':
			out = append(out, token{Kind: tRParen, Val: ")", Pos: i})
			i++
		case c == '?':
			out = append(out, token{Kind: tQuestion, Val: "?", Pos: i})
			i++
		case c == '!':
			// In `lexCond` mode the `!=` operator pairs with the
			// other comparison tokens. The lone `!` prefix
			// predicate keeps its meaning in both modes.
			if mode == lexCond && i+1 < len(s) && s[i+1] == '=' {
				out = append(out, token{Kind: tNeq, Val: "!=", Pos: i})
				i += 2
				continue
			}
			out = append(out, token{Kind: tBang, Val: "!", Pos: i})
			i++
		case c == '-':
			// `->` is the structural separator between the
			// parameter-requirement and return-requirement halves
			// of a structural rule. Negative integer literals such
			// as `-1` keep their atomic-ident lex (scanIdent treats
			// `-` as part of the identifier run), so the two-char
			// arrow is the only case the tokenizer special-cases
			// here.
			if i+1 < len(s) && s[i+1] == '>' {
				out = append(out, token{Kind: tArrow, Val: "->", Pos: i})
				i += 2
				continue
			}
			val, end, err := scanIdent(s, i, mode)
			if err != nil {
				return nil, err
			}
			out = append(out, token{Kind: tIdent, Val: val, Pos: i})
			i = end
		case c == '*':
			// `*` is the complete wildcard when standalone (followed
			// by whitespace, a separator, or end of input). When it
			// is immediately followed by an identifier-continuation
			// character it is part of a pointer type token — `*int`,
			// `**T`, `*pkg.T` — and scanIdent must absorb the entire
			// run to keep the type atomic.
			if i+1 >= len(s) || isIdentStop(s[i+1], mode) {
				out = append(out, token{Kind: tStar, Val: "*", Pos: i})
				i++
				continue
			}
			val, end, err := scanIdent(s, i, mode)
			if err != nil {
				return nil, err
			}
			out = append(out, token{Kind: tIdent, Val: val, Pos: i})
			i = end
		case c == '"':
			end, err := scanString(s, i)
			if err != nil {
				return nil, err
			}
			out = append(out, token{Kind: tStr, Val: s[i:end], Pos: i})
			i = end
		case mode == lexCond && c == '[':
			out = append(out, token{Kind: tLBracket, Val: "[", Pos: i})
			i++
		case mode == lexCond && c == ']':
			out = append(out, token{Kind: tRBracket, Val: "]", Pos: i})
			i++
		case mode == lexCond && c == '.':
			out = append(out, token{Kind: tDot, Val: ".", Pos: i})
			i++
		case mode == lexCond && c == '$':
			out = append(out, token{Kind: tDollar, Val: "$", Pos: i})
			i++
		case mode == lexCond && c == '=':
			// `=>` is the logical implication arrow, `==` is the
			// equality comparison. A bare `=` is invalid; the lexer
			// reports it via scanIdent so the caller surfaces a
			// structured diagnostic with offset information.
			if i+1 < len(s) && s[i+1] == '>' {
				out = append(out, token{Kind: tImply, Val: "=>", Pos: i})
				i += 2
				continue
			}
			if i+1 < len(s) && s[i+1] == '=' {
				out = append(out, token{Kind: tEq, Val: "==", Pos: i})
				i += 2
				continue
			}
			return nil, fmt.Errorf("unexpected `=` at offset %d (write `==` or `=>`): %w", i, ErrSyntax)
		case mode == lexCond && c == '<':
			// `<=>` is the logical biconditional, `<=` is the
			// less-or-equal comparison, a lone `<` is less-than.
			if i+2 < len(s) && s[i+1] == '=' && s[i+2] == '>' {
				out = append(out, token{Kind: tEquiv, Val: "<=>", Pos: i})
				i += 3
				continue
			}
			if i+1 < len(s) && s[i+1] == '=' {
				out = append(out, token{Kind: tLte, Val: "<=", Pos: i})
				i += 2
				continue
			}
			out = append(out, token{Kind: tLt, Val: "<", Pos: i})
			i++
		case mode == lexCond && c == '>':
			if i+1 < len(s) && s[i+1] == '=' {
				out = append(out, token{Kind: tGte, Val: ">=", Pos: i})
				i += 2
				continue
			}
			out = append(out, token{Kind: tGt, Val: ">", Pos: i})
			i++
		default:
			val, end, err := scanIdent(s, i, mode)
			if err != nil {
				return nil, err
			}
			out = append(out, token{Kind: tIdent, Val: val, Pos: i})
			i = end
		}
	}
	out = append(out, token{Kind: tEOF, Val: "", Pos: i})
	return out, nil
}

// scanString consumes a double-quoted string literal, honouring
// backslash escapes the same way Go's lexer does. It returns the
// end index (one past the closing quote).
//
// vow:cond * -> _, ErrSyntax | nil
func scanString(s string, start int) (int, error) {
	i := start + 1
	for i < len(s) {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if c == '"' {
			return i + 1, nil
		}
		i++
	}
	return 0, fmt.Errorf("unterminated string literal starting at offset %d: %w", start, ErrSyntax)
}

// scanIdent consumes a non-punctuation run. In `lexReturn` mode it
// may contain balanced square brackets so composite types like
// `map[string]int` stay glued together. In `lexCond` mode brackets,
// dots, and comparison-operator leaders become hard stops so the
// expanded grammar surfaces them as distinct tokens, with one
// carve-out: a run that starts with `@` keeps the bracket-absorbing
// behaviour so rule references such as `@Use[X]` survive as a single
// tIdent that the existing string-level parseRuleRefAt path can
// consume. The scan stops at whitespace, top-level separators,
// qualifier suffixes, braces, or a string quote.
//
// vow:cond * -> _, _, ErrSyntax | nil
func scanIdent(s string, start int, mode lexerMode) (string, int, error) {
	absorbBrackets := mode == lexReturn || (mode == lexCond && s[start] == '@')
	i := start
	bracketDepth := 0
	for i < len(s) {
		c := s[i]
		if absorbBrackets {
			if c == '[' {
				bracketDepth++
				i++
				continue
			}
			if c == ']' {
				if bracketDepth == 0 {
					break
				}
				bracketDepth--
				i++
				continue
			}
			if bracketDepth > 0 {
				i++
				continue
			}
		}
		if isIdentStop(c, mode) {
			break
		}
		i++
	}
	if bracketDepth != 0 {
		return "", 0, fmt.Errorf("unbalanced [ in token starting at offset %d: %w", start, ErrSyntax)
	}
	if i == start {
		return "", 0, fmt.Errorf("unexpected character %q at offset %d: %w", s[start], start, ErrSyntax)
	}
	return s[start:i], i, nil
}

func isIdentStop(c byte, mode lexerMode) bool {
	switch c {
	case ' ', '\t', '\n', '\r',
		',', ';', '|',
		'(', ')',
		'{', '}',
		'!', '?',
		'"':
		return true
	}
	if mode == lexCond {
		switch c {
		case '[', ']', '.', '$', '=', '<', '>':
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------
// Parser
// ----------------------------------------------------------------------

type parser struct {
	src  string
	toks []token
	pos  int
}

func (p *parser) peek() token       { return p.toks[p.pos] }
func (p *parser) peekN(n int) token { return p.toks[p.pos+n] }
func (p *parser) advance() token    { t := p.toks[p.pos]; p.pos++; return t }
func (p *parser) atEnd() bool       { return p.toks[p.pos].Kind == tEOF }

func (p *parser) accept(k tokKind) bool {
	if p.toks[p.pos].Kind == k {
		p.pos++
		return true
	}
	return false
}

// parseAnnotation drives the top-level production: a comma-separated
// position list. A single position whose only term is a tuple-sum is
// lifted to the TupleSum form so the downstream analyzer keeps using
// the existing TupleSum path.
//
// vow:cond * -> _, (ErrSyntax | _)?
func (p *parser) parseAnnotation() (*ReturnAnnotation, error) {
	if p.atEnd() {
		return nil, fmt.Errorf("empty position list: %w", ErrSyntax)
	}
	positions, err := p.parsePositions()
	if err != nil {
		return nil, err
	}
	if !p.atEnd() {
		return nil, fmt.Errorf("unexpected token %q after position list: %w", p.peek().Val, ErrSyntax)
	}
	sum, lifted, err := liftTupleSum(positions)
	if err != nil {
		return nil, err
	}
	if lifted {
		return &ReturnAnnotation{TupleSum: sum}, nil
	}
	return &ReturnAnnotation{Positions: positions}, nil
}

// parsePositions parses one or more comma-separated alternatives.
// A trailing or doubled comma surfaces as the same "empty position"
// diagnostic the existing analyzer tests pin.
//
// vow:cond * -> _, (ErrSyntax | _)?
func (p *parser) parsePositions() ([]PositionSpec, error) {
	var out []PositionSpec
	for {
		spec, err := p.parseAlternation()
		if err != nil {
			return nil, err
		}
		out = append(out, spec)
		if !p.accept(tComma) {
			break
		}
		next := p.peek().Kind
		if next == tComma || next == tEOF || next == tRParen {
			return nil, fmt.Errorf("empty position: %w", ErrSyntax)
		}
	}
	return out, nil
}

// parseAlternation handles the pipe level. The grouping form
// `(A | B)?` is peeled off first via a lookahead that
// distinguishes a position-list tuple (`(A, B)`) from a grouped
// alternation (`(A | B)`). A bare term with no following pipe
// collapses back to a Single-shaped PositionSpec.
//
// A trailing `?` after a bare type binds to the position as a
// whole rather than to the term:
//
//   - A sole bare term (`T?`) collapses into a 1-member Optional
//     DirectSum via collapseOptionalTerm.
//   - A pipe-separated sum (`A | B?`) binds the qualifier to the
//     whole sum, so `A | B?` and `(A | B)?` produce the same
//     DirectSum + Qualifier AST.
//
// The `_` placeholder and prefix-predicate forms handle their own
// trailing `?` in parseIdentLed — the placeholder drops it and a
// prefix predicate rejects it — so only bare types reach this
// position-level binding. The binding matches the surface intent:
// a single `?` makes the position admit nil regardless of which
// alternative it trails.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | _)?
func (p *parser) parseAlternation() (PositionSpec, error) {
	if p.peek().Kind == tLParen && p.looksLikeParenGrouping() {
		return p.parseParenGrouping()
	}
	first, err := p.parseTermOrTuple()
	if err != nil {
		return PositionSpec{}, err
	}
	if p.peek().Kind != tPipe {
		if first.Term != nil && p.peek().Kind == tQuestion {
			p.advance()
			return collapseOptionalTerm(first)
		}
		return wrapSingleAlternative(first), nil
	}
	members := []SumMember{first}
	for p.accept(tPipe) {
		switch p.peek().Kind {
		case tComma, tEOF, tPipe, tRParen:
			return PositionSpec{}, fmt.Errorf("empty member after `|`: %w", ErrSyntax)
		}
		next, err := p.parseTermOrTuple()
		if err != nil {
			return PositionSpec{}, err
		}
		members = append(members, next)
	}
	q := p.acceptTrailingQualifier()
	return PositionSpec{Sum: &DirectSum{Members: members, Qualifier: q}}, nil
}

// looksLikeParenGrouping reports whether the parenthesised span
// starting at the current position carries an alternation rather
// than a comma-separated tuple. The decision is purely syntactic
// and looks for one of three markers:
//
//   - A `|` at the same nesting depth as the opening paren
//     (encountered before any same-depth `,`) marks the span as
//     a grouping.
//   - A same-depth `,` first marks it as a tuple.
//   - Reaching the closing `)` with neither marker but a trailing
//     suffix qualifier (`?`, `!`) marks the span as a single-member
//     grouping so the qualifier can attach to the resulting
//     DirectSum.
//   - Reaching the closing `)` with no marker and no trailing
//     qualifier leaves the span as a degenerate single-position
//     tuple, which liftTupleSum turns into the canonical
//     parenthesised-payload TupleSum shape.
//
// The decision is deterministic on token-stream alone, so the
// parser commits to one of parseParenGrouping or parseTuple
// without having to backtrack. Nested parens are skipped via the
// depth counter so an inner tuple's commas do not leak out.
func (p *parser) looksLikeParenGrouping() bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case tLParen:
			depth++
		case tRParen:
			depth--
			if depth == 0 {
				if i+1 < len(p.toks) && p.toks[i+1].Kind == tQuestion {
					return true
				}
				return false
			}
		case tComma:
			if depth == 1 {
				return false
			}
		case tPipe:
			if depth == 1 {
				return true
			}
		case tEOF:
			return false
		}
	}
	return false
}

// parseParenGrouping consumes `(A | B)?` and exposes it as a
// PositionSpec. Single-member alternations land here as
// one-member DirectSums so the author's explicit "this position
// is a sum" marker survives round-tripping through DirectSum.String.
//
// The grouping enforces: at most one trailing suffix qualifier,
// no immediate identifier after the closing `)` (which would imply
// a stray type token), and no conflicting qualifiers when the
// inner alternation already carried one.
func (p *parser) parseParenGrouping() (PositionSpec, error) {
	if !p.accept(tLParen) {
		return PositionSpec{}, fmt.Errorf("expected ( at offset %d", p.peek().Pos)
	}
	if p.peek().Kind == tRParen {
		return PositionSpec{}, fmt.Errorf("grouped sum has no members")
	}
	inner, err := p.parseAlternation()
	if err != nil {
		return PositionSpec{}, err
	}
	if !p.accept(tRParen) {
		return PositionSpec{}, fmt.Errorf("unbalanced ( (no matching ))")
	}
	q := p.acceptTrailingQualifier()
	if extra := p.acceptTrailingQualifier(); extra != QualifierNone {
		return PositionSpec{}, fmt.Errorf("unexpected suffix on direct sum: direct sums accept at most one suffix qualifier (?)")
	}
	if p.peek().Kind == tIdent {
		return PositionSpec{}, fmt.Errorf("unexpected suffix %q on direct sum: direct sums accept at most one suffix qualifier (?)", p.peek().Val)
	}
	sum := promoteToSum(inner)
	if q != QualifierNone {
		if sum.Qualifier != QualifierNone && sum.Qualifier != q {
			return PositionSpec{}, fmt.Errorf("conflicting qualifiers on direct sum")
		}
		sum.Qualifier = q
	}
	return PositionSpec{Sum: sum}, nil
}

// parseTermOrTuple consumes one alternative of a pipe-separated
// sum. It dispatches on the leading token between the leaf
// forms allowed at this level — a parenthesised position list (a
// tuple) or an identifier-led term. The `nonzero` / `nonnil`
// keyword spellings enter through the identifier-led path; this
// dispatch rejects `!`-led `!zero` / `!nil` spellings.
//
// vow:cond * -> _, (ErrSyntax | ErrInvalidGrammar | ErrRetiredSurface)?
func (p *parser) parseTermOrTuple() (SumMember, error) {
	switch p.peek().Kind {
	case tLParen:
		return p.parseTuple()
	case tBang:
		return SumMember{}, fmt.Errorf("use the `nonzero` or `nonnil` keyword instead of the `!`-led `!zero` / `!nil` spelling: %w", ErrRetiredSurface)
	case tStar:
		return p.parseStar()
	case tIdent:
		return p.parseIdentLed()
	case tStr:
		tok := p.advance()
		lit, ok := parseLiteral(tok.Val)
		if !ok {
			return SumMember{}, fmt.Errorf("malformed string literal %q: %w", tok.Val, ErrSyntax)
		}
		return SumMember{Literal: lit}, nil
	case tEOF:
		return SumMember{}, fmt.Errorf("unexpected end of input: %w", ErrSyntax)
	}
	return SumMember{}, fmt.Errorf("unexpected token %q at offset %d: %w", p.peek().Val, p.peek().Pos, ErrSyntax)
}

// parseTuple consumes a parenthesised position list and returns it
// as a tuple SumMember. The lift to TupleSum (at the top level)
// happens later in liftTupleSum so the analyzer keeps seeing the
// existing TupleSum shape.
//
// vow:cond * -> _, (ErrSyntax | _)?
func (p *parser) parseTuple() (SumMember, error) {
	if !p.accept(tLParen) {
		return SumMember{}, fmt.Errorf("expected ( at offset %d: %w", p.peek().Pos, ErrSyntax)
	}
	if p.peek().Kind == tRParen {
		return SumMember{}, fmt.Errorf("tuple is empty: %w", ErrSyntax)
	}
	positions, err := p.parsePositions()
	if err != nil {
		return SumMember{}, err
	}
	if !p.accept(tRParen) {
		return SumMember{}, fmt.Errorf("unbalanced ( (no matching )): %w", ErrSyntax)
	}
	tuple, err := positionsToTuple(positions)
	if err != nil {
		return SumMember{}, err
	}
	return SumMember{Tuple: tuple}, nil
}

// parseStar consumes the `*` wildcard token and the optional type
// that follows. Bare `*` is the trivial position constraint
// (matches every value); `* error` pins the wildcard to a Go type
// so the analyzer can use the type axis when matching, while still
// admitting every value of that type.
//
// A trailing suffix qualifier on `*` is rejected — the wildcard
// has no per-value constraint to negate and the `?` / `!`
// modifiers belong on suffix-form terms, not on the wildcard
// surface.
//
// vow:cond * -> _, ErrInvalidGrammar | nil
func (p *parser) parseStar() (SumMember, error) {
	p.advance() // consume *
	term := &Term{Wildcard: true}
	if p.peek().Kind == tIdent && !isReservedWord(p.peek().Val) {
		term.Type = p.advance().Val
	}
	if p.peek().Kind == tQuestion {
		return SumMember{}, fmt.Errorf("`*` wildcard does not take a suffix qualifier: %w", ErrInvalidGrammar)
	}
	return SumMember{Term: term}, nil
}

// parseIdentLed handles every alternative that starts with an
// identifier-shaped token. That covers bare types (`Result`,
// `*int`), placeholders (`_`), predicate keywords (`zero`,
// `nil`), and literals (`nil`, `true`, `false`, `0xff`), plus
// the removed forms (`<sentinel>`, `<...>`) we reject
// explicitly.
//
// vow:cond * -> _, (ErrSyntax | ErrRetiredSurface | ErrInvalidGrammar)?
func (p *parser) parseIdentLed() (SumMember, error) {
	tok := p.peek()
	val := tok.Val
	if val == "<sentinel>" {
		return SumMember{}, fmt.Errorf("the `<sentinel>` matcher is unsupported; list the concrete sentinel references in the vow:cond signature instead: %w", ErrRetiredSurface)
	}
	if strings.HasPrefix(val, "<") && strings.HasSuffix(val, ">") {
		return SumMember{}, fmt.Errorf("angle-bracketed token %q is not part of the DSL surface: %w", val, ErrSyntax)
	}
	if val == "_" {
		p.advance()
		if p.peek().Kind == tBang {
			return SumMember{}, fmt.Errorf("term `_!`: spell the predicate as `nonzero _` instead of the `T!` suffix: %w", ErrInvalidGrammar)
		}
		p.acceptTrailingQualifier()
		return SumMember{Term: &Term{Placeholder: true}}, nil
	}
	if val == "zero" {
		p.advance()
		return p.finishPredicateTerm(Zero{})
	}
	if val == "nonzero" {
		p.advance()
		return p.finishPredicateTerm(Not{Inner: Zero{}})
	}
	if val == "nonnil" {
		p.advance()
		return p.finishPredicateTerm(Not{Inner: Nil{}})
	}
	if lit, ok := parseLiteral(val); ok {
		p.advance()
		return SumMember{Literal: lit}, nil
	}
	// Otherwise treat as a bare type. A trailing `?` is left in the
	// token stream: it is a position-level qualifier that the
	// alternation layer folds into a DirectSum, so a bare `T?`
	// collapses to a 1-member Optional sum rather than a flag on the
	// Term.
	p.advance()
	if p.peek().Kind == tBang {
		return SumMember{}, fmt.Errorf("term %q: spell the predicate as `nonzero %s` instead of the `T!` suffix: %w", val+"!", val, ErrInvalidGrammar)
	}
	return SumMember{Term: &Term{Type: val}}, nil
}

// finishPredicateTerm parses an optional type after a predicate
// keyword. A bare predicate (no type) is allowed — the Term carries
// only the Pred field and the analyzer infers the position's type
// from the function signature.
//
// vow:cond * -> _, ErrInvalidGrammar | nil
func (p *parser) finishPredicateTerm(pred Predicate) (SumMember, error) {
	term := Term{Pred: pred}
	if p.peek().Kind == tIdent && !isReservedWord(p.peek().Val) {
		typeTok := p.advance()
		term.Type = typeTok.Val
	}
	switch p.peek().Kind {
	case tQuestion, tBang:
		return SumMember{}, fmt.Errorf("prefix predicates do not take a suffix qualifier: %w", ErrInvalidGrammar)
	}
	return SumMember{Term: &term}, nil
}

// acceptTrailingQualifier consumes a `?` token if one is present,
// returning the matching Qualifier. Returns QualifierNone when no
// suffix is present.
func (p *parser) acceptTrailingQualifier() Qualifier {
	if p.peek().Kind == tQuestion {
		p.advance()
		return QualifierOptional
	}
	return QualifierNone
}

// isReservedWord reports whether the identifier-shaped token is a
// keyword the parser must NOT consume as a type. Used by the `nil`
// / `nonzero` lookahead so a payload like `nil | nil` does not
// greedily eat the second `nil` as a type for the first one.
func isReservedWord(val string) bool {
	switch val {
	case "zero", "nil", "nonzero", "nonnil", "true", "false", "_":
		return true
	}
	return false
}

// collapseOptionalTerm folds a sole `T?` alternative into a 1-member
// Optional DirectSum — the canonical AST shape for the optional
// suffix, since Term carries no Optional flag. It rejects the `?`
// on obvious value types (`int?`, `bool?`), which can never be nil,
// mirroring the value-type check ParseTerm performs on the `?`
// suffix.
//
// vow:cond * -> _, ErrInvalidGrammar | nil
func collapseOptionalTerm(m SumMember) (PositionSpec, error) {
	if isObviouslyValueType(m.Term.Type) {
		return PositionSpec{}, fmt.Errorf("term %q: value type %q cannot use ? (Optional): %w", m.Term.Type+"?", m.Term.Type, ErrInvalidGrammar)
	}
	return PositionSpec{Sum: &DirectSum{Members: []SumMember{m}, Qualifier: QualifierOptional}}, nil
}

// ----------------------------------------------------------------------
// AST helpers
// ----------------------------------------------------------------------

// wrapSingleAlternative reshapes a lone parseTermOrTuple result so a
// bare Term collapses to a Single PositionSpec and anything else
// (tuple, literal) stays inside a one-member DirectSum. The latter
// keeps downstream iteration over Members uniform.
func wrapSingleAlternative(m SumMember) PositionSpec {
	if m.Term != nil {
		return PositionSpec{Single: m.Term}
	}
	return PositionSpec{Sum: &DirectSum{Members: []SumMember{m}}}
}

// promoteToSum turns a PositionSpec back into a DirectSum. A Single
// is wrapped as a one-member sum; an existing Sum is returned as-is
// so a grouping-level qualifier can be attached.
func promoteToSum(p PositionSpec) *DirectSum {
	if p.Sum != nil {
		return p.Sum
	}
	return &DirectSum{Members: []SumMember{{Term: p.Single}}}
}

// liftTupleSum detects the top-level shape `(...) | (...)` (or a
// single `(...)`) and rebuilds it as the TupleSum form. The
// downstream analyzer keys off TupleSum to drive the lock-step
// matching, so the structural change must surface at the AST level
// rather than only at the surface syntax. A sum-of-tuples whose
// members have mismatched arities is rejected with a structured
// error so authors get a precise diagnostic instead of a silent
// pass-through to the position-wise interpretation, which would
// then crash later in the analyzer.
func liftTupleSum(positions []PositionSpec) (*DirectSum, bool, error) {
	if len(positions) != 1 {
		return nil, false, nil
	}
	sum := positions[0].Sum
	if sum == nil {
		return nil, false, nil
	}
	if !allTupleMembers(sum) {
		return nil, false, nil
	}
	if err := requireUniformArity(sum); err != nil {
		return nil, false, err
	}
	return sum, true, nil
}

// positionsToTuple converts a parenthesised position list into a
// TupleMember. Every sub-position must collapse to a TupleElement —
// a sub-sum inside a tuple element is rejected because the new
// surface has no `(A | B, C)` form (the equivalent is written as
// `(A, C) | (B, C)`).
//
// vow:cond * -> _, ErrInvalidGrammar | nil
func positionsToTuple(positions []PositionSpec) (*TupleMember, error) {
	elements := make([]TupleElement, 0, len(positions))
	for i, pos := range positions {
		switch {
		case pos.Single != nil:
			elements = append(elements, TupleElement{Term: pos.Single})
		case pos.Sum != nil && len(pos.Sum.Members) == 1 && pos.Sum.Qualifier == QualifierNone:
			m := pos.Sum.Members[0]
			switch {
			case m.Term != nil:
				elements = append(elements, TupleElement{Term: m.Term})
			case m.Literal != nil:
				elements = append(elements, TupleElement{Literal: m.Literal})
			default:
				return nil, fmt.Errorf("tuple element %d cannot be a tuple: %w", i, ErrInvalidGrammar)
			}
		case pos.Sum != nil:
			return nil, fmt.Errorf("tuple element %d cannot be a sum; distribute the sum across the tuple instead: %w", i, ErrInvalidGrammar)
		}
	}
	return &TupleMember{Elements: elements}, nil
}

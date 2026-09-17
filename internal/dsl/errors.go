package dsl

import "errors"

// ErrSyntax marks tokenizer- or structural-syntax failures in DSL input:
// unbalanced delimiters, unterminated string literals, unexpected
// tokens, and empty mandatory constructs. Callers can discriminate
// these from semantic errors with errors.Is(err, ErrSyntax).
// vow:define @Sentinel
var ErrSyntax = errors.New("syntax error")

// ErrInvalidGrammar marks DSL inputs that tokenize cleanly but violate
// the grammar's semantic shape rules — tuple nested in tuple, sum
// nested in tuple, multiple top-level `->`, qualifier stacking,
// arity mismatch on rule application, and qualifier misuse on
// wildcards or prefix predicates.
// vow:define @Sentinel
var ErrInvalidGrammar = errors.New("invalid grammar")

// ErrUnknownReference marks references to predicates, literals, rules,
// or aliases that cannot be resolved in the current scope.
// vow:define @Sentinel
var ErrUnknownReference = errors.New("unknown reference")

// ErrInvalidPreset marks structural validation failures on a loaded
// preset: missing required fields, empty subject or obligation lists,
// or duplicate rule parameters.
// vow:define @Sentinel
var ErrInvalidPreset = errors.New("invalid preset")

// ErrRetiredSurface marks DSL surfaces the parser rejects in
// favor of their replacement, such as the `<sentinel>`
// any-matcher whose role lives on the `vow:cond` signature
// listing concrete sentinels.
// vow:define @Sentinel
var ErrRetiredSurface = errors.New("retired DSL surface")

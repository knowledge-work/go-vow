package dsl

// Effect is the resolved meaning of a rule reference. Every @rule
// occurrence in the surface produces an *Effect after the rule
// resolver runs. The Kind discriminates the union; exactly one of
// the payload fields below is non-nil and is selected by Kind.
//
// The AST declaration of every recognised effect lives here; the
// resolver paths that fill it in live in the downstream analyzer
// passes. The analyzer drives preset `@rule` resolution through
// the textual RuleScope.Expand path, so an *Effect on a parsed
// RuleRef is populated only when a caller (a preset-effect path)
// explicitly resolves it.
type Effect struct {
	Kind EffectKind

	// PayloadExpansion holds the textually-expanded body of a
	// preset-YAML rule (e.g. `@result.OkErr[T, E]`) parsed back into
	// a ReturnAnnotation. Set when Kind == EffectExpandPayload.
	PayloadExpansion *ReturnAnnotation
}

// EffectKind enumerates the recognised effect categories. Future
// effects extend this enum without changing the surface grammar —
// the dispatch lives in this single discriminator.
type EffectKind int

const (
	// EffectExpandPayload covers preset-YAML rules whose body is a
	// fragment of return-annotation syntax. The expanded body is
	// parsed back through the precedence parser and the resulting
	// ReturnAnnotation is stored on Effect.PayloadExpansion.
	EffectExpandPayload EffectKind = iota
)

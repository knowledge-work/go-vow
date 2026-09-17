// Package condLogicalXPkgCallee is the callee-side fixture for
// the cross-package logical-arrow vow:cond trace. Each exported
// declaration carries a logical-arrow rule (`=>` or `<=>`) whose
// operands bind to the function's parameters. The analyzer
// exports a conditionLogicalFact for every entry so the caller-
// side evaluator in an importing package drives the same
// literal-level check the same-package path drives.
//
// The want comment after each declaration pins the rendered
// fact shape so a regression that drops the operand or arrow
// surfaces here rather than as a silent absence in the caller-
// side fixture.
package condLogicalXPkgCallee

// RequireImply declares a forward implication between two pointer
// parameters: when the first is non-nil, the second must be
// non-nil as well. The fact carries this contract across the
// package boundary so an importing call site that pairs a non-nil
// first argument with a nil second argument surfaces a diagnostic.
//
// vow:cond x != nil => y != nil
func RequireImply(x, y *int) {} // want RequireImply:"vow:cond\\(x != nil => y != nil\\)"

// RequireEquiv declares a biconditional between two pointer
// parameters: both must be nil together, or both must be non-nil
// together. The cross-package fact pins the equivalence shape so
// a divergent argument pair at an importing call site surfaces a
// diagnostic.
//
// vow:cond x != nil <=> y != nil
func RequireEquiv(x, y *int) {} // want RequireEquiv:"vow:cond\\(x != nil <=> y != nil\\)"

// RequireTagPayload pairs a string-tagged literal comparison with
// a payload nil check. When the tag equals the live sentinel, the
// payload must be non-nil. The fact carries the literal RHS
// across the package boundary so the importing call-site
// evaluator decides the comparison through the same literal
// extractor the same-package path uses.
//
// vow:cond tag == "live" => payload != nil
func RequireTagPayload(tag string, payload *int) {} // want RequireTagPayload:"vow:cond\\(tag == \"live\" => payload != nil\\)"

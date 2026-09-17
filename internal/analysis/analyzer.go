// Package analysis exposes the top-level go/analysis Analyzer for vow.
//
// The analyzer loads DSL presets (see internal/dsl), discovers the
// subjects each preset selects (e.g. package-level sentinel-error
// vars), and dispatches the obligation checks to type-specific engines.
package analysis

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/inspect"

	"github.com/knowledge-work/go-vow/internal/config"
	"github.com/knowledge-work/go-vow/internal/dsl"
)

// Analyzer is the default analyzer configured with the builtin presets.
// CLIs and integrations that prefer custom presets can construct their
// own analyzer via New.
var Analyzer = New(builtinPresets(), nil)

// NewDefault returns an analyzer configured with the built-in
// presets and an optional driver-supplied config override (passed
// straight through to New). It saves CLI drivers from importing the
// preset constructor that lives unexported in this package.
func NewDefault(override *config.Config) *analysis.Analyzer {
	return New(builtinPresets(), override)
}

// New returns an analyzer configured to enforce the supplied presets.
// The analyzer is read-only with respect to the presets; callers may
// reuse the slice. Each analyzer instance owns a fresh config
// resolver so per-directory `vow.yaml` files discovered during a run
// stay scoped to that run.
//
// override carries a Config that takes precedence over any vow.yaml
// the resolver would otherwise find — set it from a CLI driver that
// loaded an explicit config file (e.g. `--config-file=<path>`). A nil
// override leaves discovery in charge.
func New(presets []*dsl.Preset, override *config.Config) *analysis.Analyzer {
	resolver := config.NewResolver()
	return &analysis.Analyzer{
		Name:      "vow",
		Doc:       "vow lints value-level obligations declared in DSL presets (sentinel-error consumption, nil narrowing, exclusive return, ...).",
		Requires:  []*analysis.Analyzer{inspect.Analyzer, buildssa.Analyzer},
		Run:       runFunc(presets, resolver, override),
		FactTypes: []analysis.Fact{(*signatureNilFact)(nil), (*fieldNilFact)(nil), (*conditionLogicalFact)(nil)},
	}
}

func runFunc(presets []*dsl.Preset, resolver *config.Resolver, override *config.Config) func(*analysis.Pass) (any, error) {
	return func(pass *analysis.Pass) (any, error) {
		state := newPassState(presets, resolver, override)
		cleanup := registerPassState(pass, state)
		defer cleanup()
		// Discover caller-authored filter markers (vow:suppress,
		// vow:use) before any obligation check emits a diagnostic.
		// The function-level / line-level sets must be populated
		// when vowReport later asks "is this must-consume site
		// suppressed?" or "is this must-consume subject asserted as
		// used?"; running discovery up-front keeps that ordering
		// independent of which obligation type runs first.
		discoverSuppress(pass, state)
		discoverUseHints(pass, state)
		// Export the vow:nil signature and field decls as Go
		// analysis Facts so caller-side checks running in
		// importing packages read the per-position contract
		// through pass.ImportObjectFact. Same-package callers
		// still consult the in-memory maps because the fact
		// mechanism roundtrips through the cache only across
		// package boundaries. The signature path goes through
		// state.nilDeclSet, which is eagerly triggered a few
		// lines below; ordering does not matter because
		// nilDeclSet builds idempotently on first access.
		exportSignatureNilFacts(pass, state)
		exportFieldNilFacts(pass, state)
		// Report a vow:nil field marker sitting where the decl
		// collection never reads it — a field of an anonymous struct
		// type, or an embedded field. Such a marker exports no fact and
		// no check consults it, so without this the author holds a
		// contract that reads as enforced and is inert.
		validateFieldNilMarkerReach(pass, state)
		validateReadonlyWrites(pass)
		// Publish a conditionLogicalFact for every function whose
		// vow:cond payload carries a logical-arrow rule so the
		// caller-side evaluator in importing packages drives the
		// same literal-level check the same-package path drives.
		exportConditionLogicalFacts(pass, state)
		// Eagerly trigger the transducer-set build so classifier
		// lookups reuse the cached result. The build itself emits
		// no diagnostics; a `vow:use X` declaration whose
		// signature is non-bool falls through to the callee-side
		// discharge contract without transducer registration.
		state.userTransducerSet(pass)
		// Eagerly trigger the vow:nil signature build so any
		// malformed marker payload surfaces its parse diagnostic
		// at the declaring function, even when no caller of that
		// function reaches the argument check.
		state.nilDeclSet(pass)
		for _, p := range presets {
			subjects := findSubjects(pass, p)
			if len(subjects) == 0 {
				continue
			}
			for _, ob := range p.Obligations {
				switch ob.Type {
				case "must-consume":
					reportMustConsume(pass, p, subjects, ob, state)
					reportSSALeaks(pass, p, subjects, state)
				}
			}
		}
		// vow:cond annotation enforcement is preset-independent:
		// the annotation is intrinsic to the function declaration
		// rather than tied to a particular subject set.
		checkReturnAnnotations(pass, state)
		// Evaluate logical-arrow vow:cond rules (`=>`, `<=>`)
		// against the function's return statements. The evaluator
		// is literal-only at this layer: operands that bind to a
		// return slot whose value is a syntactic nil or a
		// syntactic non-nil shape decide; everything else defers
		// to the caller-side pass and to the layer that lands SSA-driven refinement.
		checkLogicalRuleReturns(pass, state)
		// Apply the same literal-level evaluator at every call
		// site against the callee's logical-arrow rules. The
		// caller-side path binds operands to the call's arguments
		// (or to the method receiver) so a caller that passes a
		// nil literal at a position the callee's rule pins as
		// non-nil surfaces a diagnostic at the call expression.
		validateCallerLogicalConds(pass, state)
		// Report contradictory vow:cond rule pairs that share a
		// left-hand side and require opposite right-hand side
		// truth. The walk inspects each function's rule pool
		// together with the derived contrapositives and
		// transitive chain closures so a contradiction that
		// surfaces only after chain expansion still reaches the
		// diagnostic. The anchor is the function declaration
		// because the offence belongs to the rule set rather
		// than to any single return or call site.
		checkLogicalRuleSetInconsistencies(pass, state)
		// Report vow:cond rules whose operands the vow:nil
		// signature decides into a boolean combination that
		// contradicts the rule's direction at the function
		// boundary. The walk inspects the rule pool together
		// with its derived contrapositives and transitive
		// chain closures so a contradiction that surfaces only
		// after chain expansion still reaches the diagnostic.
		checkLogicalRuleNilDeclConsistency(pass, state)
		// Surface a hint when two or more author-written
		// vow:cond implications narrow the same signature
		// position only to nil and no rule narrows it to
		// non-nil; the recurring pattern is strong enough to
		// invite an explicit vow:nil declaration on the
		// position. The hint reads only author-written rules
		// so derived contrapositives and chain closures do not
		// noise the induction signal.
		reportNilOnlyInductionHints(pass, state)
		// Report tag-discriminator rule pools whose covered
		// literals leave finite-enum candidates uncovered. The
		// per-pass contract lives on checkTagExhaustiveness.
		checkTagExhaustiveness(pass, state)
		// Validate subject-scoped vow:cond and vow:emit markers
		// against the callback signature they delegate to. The
		// vow:cond[subject] path checks each logical-arrow
		// operand's reference against the callback's parameters,
		// receiver, and named returns; the vow:emit[subject] path
		// checks that the subject is function-typed so emission
		// has a callable carrier to propagate through.
		validateSubjectScopedLogicalConds(pass, state)
		validateSubjectScopedEmits(pass, state)
		// Subject-scoped vow:nil contracts route through a
		// callback signature rather than the enclosing function;
		// the walker reports when the resolved subject is not
		// function-typed and when the contract names more
		// positions than the callback signature exposes.
		validateSubjectScopedNilDecls(pass, state)
		// Subject-scoped vow:use assertions — both the function-
		// doc form and the line-scope form anchored inside a body
		// — propagate discharge through a callback's invocation,
		// so the walker reports when the resolved subject is not
		// function-typed.
		validateSubjectScopedUseHints(pass, state)
		// Report a vow:cond line the annotation pipeline could
		// not resolve into a condition. Such a line is dropped so
		// the rest of the file still analyses, which would
		// otherwise leave an unenforced contract reading as
		// enforced.
		validateCondAnnotationResolution(pass, state)
		// Emit a diagnostic for any vow:discharged occurrence that
		// is not anchored to a function doc comment. The marker is
		// function-scoped by design; line-level or non-function
		// occurrences would otherwise be silently discarded.
		reportLineLevelDischarged(pass)
		// Report any vow:use X function whose body lacks a
		// discharge site for X. Each function is validated
		// independently against the (a) direct-conditional and
		// (c) handoff equivalences; the (b) transducer path is
		// not wired.
		validateUseCallees(pass, state)
		// Report any vow:emit X function whose body never returns
		// a value that resolves to X. The vow:emit declaration's
		// caller-side credit (chain authorisation) is only
		// meaningful when the callee actually emits the subject,
		// so the self-validation refuses an empty-emission
		// declaration up-front.
		validateEmitCallees(pass, state)
		// Report every call site whose argument is statically
		// nil at a position the callee's vow:nil signature pins
		// as non-nil (`!`). The judgement is conservative on the
		// false-positive side: only the literal nil and a
		// typed-nil conversion qualify. Cross-package callees
		// ride on the imported signatureNilFact so the per-
		// position contract surfaces regardless of where the
		// callee's declaration sits.
		validateCallerArgNilSafety(pass, state)
		// Report every method-call site whose receiver is
		// statically nil. The judgement mirrors the argument-
		// side check on the receiver axis: a vow:nil signature
		// that pins the receiver as non-nil (`!.()`) emits a
		// strict diagnostic, unannotated callees emit a
		// default-warn diagnostic (nil-receiver method calls
		// are Go's most common runtime panic source), and
		// vow-declared callees that omit the receiver layer or
		// pin it as nillable (`?.()`) stay silent because the
		// author has not claimed a non-nil-receiver constraint.
		validateCallerReceiverNilSafety(pass, state)
		// Report a nil guard standing on a local the callee's
		// vow:nil return declaration already proves non-nil. This
		// mirrors the callee-side dead-guard recogniser across the
		// call boundary: the callee surface retires the defensive
		// check on the parameter side, this one retires it on the
		// call-result side. The pass stays AST-only and block-local
		// so it keeps the same syntactic-pattern stance the other
		// dead-guard surfaces hold. The unified pass covers both
		// vow:nil signature-mirror `!` return narrows and
		// vow:cond logical-rule caller narrows so the two fact
		// sources share one state machine and one emit path;
		// only the fact-collection phase dispatches on source.
		validateCallerDeadGuard(pass, state)
		// Report a selector-driven dereference of a call-result
		// local whose paired vow:cond guard has not yet short-
		// circuited its counter-branch. Same state machine as the
		// dead-guard pass above: the dead-guard surface catches
		// the guard-still-standing shape, this one catches the
		// deref-before-guard shape, and together they cover the
		// two ways a caller can misread the rule.
		validateCondCallerSafetyViolation(pass, state)
		// Report body-leading `if x == nil` guards whose
		// parameter the function's own vow:cond prereq
		// already proves non-nil at entry. The recogniser is
		// vow:cond-driven so it covers sentinel-
		// narrowing prereqs alongside any future prereq-based
		// non-nil declaration that lands without going through
		// the vow:nil signature-mirror surface.
		validatePrereqDeadGuards(pass, state)
		// Report callee-side contradictions the vow:nil signature
		// decl can recognise from the function body alone: a `!`
		// position reassigned or returned as nil, a `!` return
		// slot supplied with a literal nil, or a leading `if x ==
		// nil` guard that the decl already proves unreachable at
		// entry. Forward dataflow that lifts a nillable value into
		// a non-nil slot is not covered.
		validateNilDeclCalleeSelf(pass, state)
		// Report writes of a literal nil into a struct field the
		// field's own vow:nil decl pins as `!`. This is the field-
		// level counterpart of the signature-side reassign check
		// above: the decl states the field never holds nil, so an
		// assignment or a composite-literal element that supplies
		// nil contradicts it on first principles.
		validateFieldNilAssign(pass, state)
		// Report struct literals that leave a `!`-declared field at
		// its zero value and then escape the expression that built
		// them. The write check above catches an explicit nil at the
		// field; this one catches the nil an omission produces, which
		// reaches a reader the declaration entitles to skip the guard.
		validateFieldNilConstruct(pass, state)
		// Report a struct value still at its zero value where it escapes.
		// `Box{}` and `var b Box; return b` compile to the same zero
		// constant, so the syntactic form does not matter here.
		validateZeroStructEscape(pass, state)
		// Report an allocated struct that escapes while a `!`-declared
		// field has no store reaching the escape. The zero-constant pass
		// above covers values folded to a constant; this one decides
		// whether an existing field write runs first.
		validateAllocStructEscape(pass, state)
		// Report a write into a `!`-declared field whose right-hand
		// side the flow resolver classifies as nillable — a `?` field
		// read, a `?` parameter, an alias of either. The literal pass
		// above reads the nil written in the source; this one reads the
		// nil a declaration elsewhere admits, and stays silent where a
		// guard proves the value non-nil at the write.
		validateFieldNilAssignFlow(pass, state)
		// Report nillable / known-nil values that flow into a
		// position the callee's signature decl pins as `!`. The
		// walk runs over SSA so it picks up transitive shapes
		// (alias chains, phi merges, field reads of `?` fields)
		// the AST-only pass above cannot recognise on its own.
		validateNilDeclCalleeFlow(pass, state)
		// Report receiver-side deref panics that the signature
		// decl declares as legal-to-receive-nil. The body of a
		// `?`-declared-receiver method is expected to guard the
		// receiver before the first use; an unguarded deref is
		// the most common nil-receiver panic source.
		validateNilDeclReceiverGuard(pass, state)
		// Report a dereference of a `?`-declared field that no nil
		// guard covers. The declaration admits nil at the field, so
		// following the pointer without proving it non-nil is the
		// panic the declaration warns about. The same field-location
		// guard matcher the caller-side argument check consults
		// discharges the site, so one guard covers both surfaces.
		validateFieldNilDeref(pass, state)
		// Report signature positions whose type can hold nil and
		// whose vow:nil decl is missing, so a scope that opted in
		// through vow.yaml carries a written contract at every
		// nillable parameter and return. The walk reads the Go
		// signature rather than the arity rule, because a
		// well-formed marker can still leave a position undeclared.
		// This is the last of the nil-decl passes because it reports
		// on the absence of a contract rather than on a violation of
		// one, and carries its own diagnostic category to match.
		validateNilDeclCompleteness(pass, state)
		// Emit an info-level diagnostic for every vow:define
		// occurrence whose concept does not have behaviour
		// registered. The Sentinel and Closable concepts ship
		// built-in; other concepts are forward-compat surface so
		// authors can declare them ahead of preset configuration
		// without the analyzer silently dropping the declaration.
		reportConceptDefineDiagnostics(pass)
		// Discover statement-scope vow:emit markers and infer
		// each marker's destination (goroutine launch, callback
		// argument, storage write, or channel send). Authors use
		// the marker to declare a Closable hand-off so the
		// lifecycle pass below treats it as a discharge.
		discoverCallerEmit(pass, state)
		// Track Closable resource lifetimes. The pass walks each
		// function for acquisitions, deferred Close calls, and
		// statement-scope vow:emit hand-offs, then reports a
		// diagnostic for every acquisition that reaches the end
		// of the function without a recognised discharge.
		validateClosableLifetime(pass, state)
		return nil, nil
	}
}

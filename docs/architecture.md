# Architecture

This page describes how the `vow` analyzer implements the
contracts the [DSL reference](dsl-reference.md) declares. It is
aimed at contributors and at integrators who want to understand the
guarantees the analyzer makes (and the ones it intentionally does
not).

## High-level pipeline

The analyzer runs once per package and walks the source files in a
fixed order:

1. **Preset discovery.** `runFunc` iterates the configured presets.
   Each preset names a subject scheme and an obligation type.
2. **Subject selection.** `findSubjects` walks the package and
   returns the set of `types.Object`s that satisfy the subject
   pattern. The current implementation recognises the
   `annotation:` scheme on package-level `var` declarations.
3. **Obligation dispatch.** For each obligation, the matching rule
   engine runs. `must-consume` dispatches to `reportMustConsume`
   followed by `reportSSALeaks`.
4. **Condition enforcement.** `checkReturnAnnotations` runs once
   per package, irrespective of the preset list — every
   `vow:cond` line is parsed into a unified `Condition` AST
   (parameter requirement + return requirement) and the matcher
   dispatches per case. The contract
   is intrinsic to a function rather than tied to a particular
   subject set. The condition AST also represents logical-arrow
   rules (`=>` and `<=>`) so authors spell relational constraints
   with identifier-named references and postfix steps;
   `checkLogicalRuleReturns` evaluates each logical rule at every
   return statement of the declaring function, and
   `validateCallerLogicalConds` evaluates the same rules at every
   call site by binding operands to the call's arguments and
   receiver. The evaluator commits operands through three layers:
   a literal-shape match in `evaluateNilCheck` /
   `evaluateComparison`; an SSA-driven refinement in
   `ssaNarrowedNilCheck` that consults `resolveArgNilness` for
   alias / parameter / field nilness and walks dominating
   `if param != nil` guards through `narrowedByDominatingGuard`;
   and a derived-rules layer (`groupRulesWithContrapositives`)
   that pairs every implication with its contrapositive
   (`contrapositiveOf`) and expands a depth-bounded chain closure
   (`expandTransitives`) that joins implications whose
   intermediate operand matches. An original-first walk keeps the
   closure and the contrapositive silent whenever the source rule
   already committed, so the same offending claim never reports
   twice. A `conditionLogicalFact` carries each function-doc rule
   across the package boundary so an importing call site enforces
   the same contract as the same-package path.
   `validateSubjectScopedLogicalConds` and
   `validateSubjectScopedEmits` cover the `[subject]` scope: a
   logical rule retargeted at a callback parameter validates its
   references against the callback's own signature scope, and an
   emission scoped to a callback validates that the subject is
   function-typed.
   `validateCondAnnotationResolution` guards the step before all
   of these: the parse path drops a line it cannot turn into a
   condition, so a `@alias.Rule` reference whose lookup fails
   would otherwise disappear without a diagnostic. Only lookup
   failures report, which leaves the prose-marker shape (a syntax
   failure) as silent as it was.

The analyzer requires `inspect.Analyzer` (for AST walks) and
`buildssa.Analyzer` (for the SSA fallback) via `Requires`.

## Three-class detection

`classifyReference` decides the fate of every subject reference
from the AST stack — the inspector's stack from the file root
(index 0) down to the identifier itself. The classifier is
ordered:

1. **`isObserveUse`** checks for observation patterns inside a
   conditional context: observer call (`errors.Is(err, X)`), `==` /
   `!=` against the subject, or a `case X:` in a `switch`.
2. **`isChainUse`** checks for authorised propagation: the
   reference must sit inside a `ReturnStmt` whose enclosing
   `FuncDecl` (or `FuncLit` plus enclosing `FuncDecl`) carries a
   trivial-parameter-requirement case (a `vow:cond` line,
   equivalently a `vow:cond * -> ...`) whose return
   requirement lists the subject. Cases with a non-trivial
   parameter requirement stay conservatively gated for chain
   authorisation — their narrowed scope is not threaded through
   the classifier.
3. Anything else is `classLeak` and reported.

Observation wins over chain by design: if a reference is observed,
the chain check is not consulted.

## AST walk vs SSA fallback

Two complementary passes detect leaks at different shapes:

### Per-identifier walk (`reportMustConsume`)

For every subject identifier reference in the package, the walk
classifies the reference via the AST stack. `isAliasBindingRHS`
suppresses the RHS of `:=` defines so a sentinel introduced by an
alias is not double-counted. The walk handles direct references
(`return ErrFoo`) and one-hop aliases (`x := ErrFoo; return x`)
through `resolveSubject`, which delegates to the
stack-independent `resolveSubjectInFunc`.

### SSA-driven flow (`reportSSALeaks`)

For each `return <ident>` site whose AST resolution does not name a
subject, the SSA pass locates the matching `*ssa.Return` by source
position, walks the result value backward through phi nodes and
trivial loads (`UnOp(MUL)`), and reports any reachable
`*ssa.Global` that lies in the subject set.

The SSA pass adds coverage for shapes the AST walk cannot reach:

- **Multi-hop alias chains.** `x := A; y := x; return y` — the
  one-hop AST resolver stops at `x`; SSA walks the chain to `A`.
- **Reassignment after a non-subject init.** `e := f(); e = A; return e`
  — the single `:=` define has a non-Ident RHS, so the AST resolver
  gives up; SSA carries the subject forward.
- **Branch merges.** `var e; if c { e = A } else { e = B }; return e` —
  the AST resolver finds no `:=` define for `e`; the SSA phi
  flattens to `{A, B}`.

The SSA pass intentionally consults `functionAuthorizesPropagation`
before reporting so a `vow:cond` signature that lists the
subject discharges the diagnostic even when SSA detected the flow. Position-based
`*ssa.Return` mapping (`r.Pos() == astRet.Pos()`) works without
`ssa.GlobalDebug`.

## Term matching axes

`exprShape.matchesTermName` compares an annotation Term against a
return expression on these axes, in order:

1. **Syntactic identifier** (`Foo`, `pkg.Foo`).
2. **Fully qualified name** from `types.Info`
   (`module/path.Foo`).
3. **Named type** as `types.Type.String` prints it. Surfaces true
   aliases (`type X = error` makes typeName `error`).
4. **Package-relative type**, which is how an annotation inside
   the package under analysis spells it (`*Foo`, not `*pkg.Foo`).
5. **Package-name-qualified type**, which is how an annotation
   spells a type its file had to import (`*pkg.Foo`). The named
   type renders the import path instead, so an imported type
   reaches the match only here.
6. **Underlying type** after one `Underlying()` step. Reaches the
   structural form (`type Bytes []byte` → underlying `[]byte`).

Earlier axes win first, so short-form annotations continue to
match through axis 1 even after types-aware matching is enabled.

A return expression that names no identifier — `&T{...}`, a
constructor call — carries only a type, so `shapeOf` returns it as
`shapeTyped` and the comparison narrows to the type axes. Tuple
elements additionally consult the declared type of the return
position, which is what lets `nonzero error` cover a helper that
hands back a concrete error type. Matching on a type alone would
admit a nil of that type, so a `nonzero` element runs the static
nil recogniser before it passes.

## Composition (`@rule.OkErr[args]`)

`vow:import <alias> "<path>"` on the package doc registers an alias
bound to a preset's rule namespace. When a `vow:cond` line
contains `@<alias>.<rule>[args]`, the parser resolves the alias to
the preset, looks up the rule by name, and expands the rule's body
with the textual arguments substituted for the parameters. The
expanded payload is fed back through `ParseReturnAnnotation`, so
the expansion must obey the same DSL.

Parse failures during expansion are skipped silently — the "last
valid parse wins" policy means a malformed line never silently
shadows an earlier valid one.

## Pass state

`passState` holds per-pass caches: parsed `vow:cond`
annotations, file-scope rule alias maps from `vow:import`, and a
shared workspace for the zero-value classifier. `passState` is
created once per `Run` and threaded through every rule engine so
the cost of parsing each annotation is paid at most once.

`fileScope(pass, file)` returns the `dsl.RuleScope` for a file's
imported rule namespaces. The chain authorisation path consults
this scope so chain authority expressed through `@rule` resolves
correctly.

## Zero-value classifier

`classifyZero` reports whether an expression is the zero value of
its type using three layers of evidence:

1. `pass.TypesInfo.Types[expr].Value` for constant folding of
   primitives, named-primitive const refs, and similar constant
   expressions.
2. The bare `nil` identifier (Go's zero for interfaces, pointers,
   slices, maps, channels, and function values).
3. Composite literals — both the empty form `T{}` and partial forms
   `T{Field: zero}` whose every element evaluates to zero. The
   recursion inspects nested composites to arbitrary depth.

Composite literal zero classification is restricted to struct and
array types (`*types.Struct`, `*types.Array`). Slice and map
literals like `[]int{}` produce *non-nil empty* values that are
distinct from the type's zero, so the classifier treats them as
non-zero by design.

## Nil-safety pipeline

The nil-safety surface (declared through `vow:nil`; see the
[DSL reference](dsl-reference.md#vow-nil-signature-mirror))
runs as a separate pipeline alongside the must-consume engine.
Discovery, fact export, caller-side checks, and callee-side
self-validation share the same parsed signature so the marker
is the single authoring point for nil-safety contracts.

### Discovery

`nil_decl.go` walks every `FuncDecl` in `pass.Files` and parses
the doc-comment payload through `dsl.ParseNilSignature`,
returning a `map[types.Object]*dsl.NilSignature`. A second pass
(`field_nil.go`) walks every top-level struct type and parses
the per-field marker through `dsl.ParseFieldNilDecl`, returning
a `map[types.Object]*dsl.PositionDecl` keyed on the field's
type-checker Object. Both maps live on `passState` and are
built lazily on first access; same-package callers consult the
maps directly instead of going through the cache.

### Surface forms and canonical print

Nullness tokens carry two interchangeable surfaces: the symbol
form (`!` / `?`) and the keyword form (`nonnil` / `nil`). The
parser accepts either at any slot, so an author can mix forms
within a single payload — `vow:nil (nonnil,?)` is well-formed.
The canonical renderer emits the symbol form because the
signature-mirror payload sits in a dense, position-aligned shape
where every parameter slot already carries a token and the
punctuation reads tersely; the keyword alias is available so an
author can spell the constraint as a word when the symbol reads
less clearly at the authoring site.

The choice of symbol-canonical here is a frequency-driven design
choice and stands in contrast to `vow:cond` (see the [DSL
reference](dsl-reference.md#condition-grammar)), whose canonical
surface is the keyword (`nonnil x -> nonnil result`). The
`vow:cond` payload reads as a sentence with sparse, named
predicates, so the word form dominates that surface; the
`vow:nil` payload reads as a signature mirror with one token per
position, so the symbol dominates this surface. Both markers
admit the other form as an alias to give the author a per-call
escape hatch.

### Fact export and import

Two `analysis.Fact` types ride on the Go analysis Facts
mechanism so caller-side checks running in importing packages
read the contract through `pass.ImportObjectFact`:

- `signatureNilFact` carries the parsed `*dsl.NilSignature`
  (receiver, parameters, returns) at the function's
  type-checker Object. `exportSignatureNilFacts` publishes one
  fact per same-package function whose discovery produced a
  signature; `nilDeclForFunc` is the symmetric reader that
  consults the in-memory map first and falls back to the
  imported fact for cross-package callees.
- `fieldNilFact` carries the parsed `*dsl.PositionDecl` at the
  field's type-checker Object. `exportFieldNilFacts` and
  `fieldNilForField` follow the same shape as the signature
  helpers above.

Both facts encode their payload through `gob`, so the
`dsl.NilSignature` and `dsl.PositionDecl` types stay restricted
to exported fields. The Go analysis Facts mechanism silently
drops unexported objects, so unexported callees and unexported
fields stay recognised only within their declaring package.

### Caller-side checks

`validateCallerArgNilSafety` walks every `CallExpr` in
`pass.Files` and inspects each call's callee through
`nilDeclForFunc`. Parameter positions the signature pins as
`!` drive `nonNilRequirementsForCallee`; the helper builds a
list of `nonNilArgRequirement` entries keyed by argument
index, and the loop reports one diagnostic per statically-nil
argument that lands on one of those positions. The
nil-ability judgement (`argumentIsStaticallyNil`,
`argumentIsDeclaredNillableField`) is conservative — only
proof-by-construction shapes (literal `nil`, typed-nil
conversion, `?`-declared field read) qualify.

`validateCallerReceiverNilSafety` mirrors the argument check
on the receiver axis. `receiverDeclMode` classifies the
callee into three branches: `receiverModeStrict` (a `!.()`
recv decl on the signature), `receiverModeSilent` (a `?.()`
decl, an omitted receiver layer on a `vow:nil` signature, or
a same-package callee that carries `vow:cond` but no
`vow:nil` signature), and `receiverModeUnannotated` (no vow
declaration at all). The strict branch surfaces a diagnostic
that quotes the contract; the unannotated branch surfaces a
default-warn diagnostic because nil-receiver method calls are
Go's most common runtime panic source; the silent branch
emits nothing.

`validateCallerDeadGuard` covers the third caller-side shape: a
nil guard standing on a local the callee's return declaration
already proves non-nil. The pass walks each `*ast.BlockStmt`
statement list once, carrying a `callerNonNilBinding` per local
bound to a `!` return slot. Three steps run per statement and the
order matters — the guard check reads the bindings earlier
statements established, the invalidation step retires anything
this statement rewrites, and the recording step admits the
bindings this statement creates — so `x, err := New()` retires a
stale `x` and re-admits the fresh one while visiting the same
statement.

The pass stays AST-only and block-local for the same reason the
callee-side dead-guard recogniser does. The diagnostic stays
unemitted on a reassignment, an address-of hand-off, or an
increment of the bound local (each puts the value outside the
contract's description), on a function literal in the statement
(a closure rewrites a captured local out of syntactic view), on a
guard body that falls through rather than short-circuiting, on an
`if` carrying an init clause (the clause can rebind the
identifier the condition reads), and on a guard nested in an
inner block (that block's own walk carries no outer binding).
Values whose provenance needs flow analysis fall through
unreported rather than risking a report on a value that did
change.

### Callee-side self-validation

Four independent passes validate the callee's body against
its declaration, so a single function can surface multiple
diagnostics when its body violates several axes.

- `nil_check_callee_self_decl.go` (`validateNilDeclCalleeSelf`)
  groups three AST-level sub-checks on every function whose
  signature carries a `!` parameter, receiver, or return slot:
  the reassign check fires when a `!`-pinned position is
  overwritten with the bare nil literal (or a typed-nil
  conversion); the return-nil check fires when a `return`
  statement supplies the same literal at a `!`-pinned return
  slot; the dead-guard check fires when a body-leading
  `if x == nil { return ... }` short-circuit pre-establishes
  non-nil at entry on a position the signature already proves
  non-nil. The same dead-guard recogniser runs over receiver
  positions through `nonNilParamAndRecvNames`, so a `!.()`-
  pinned receiver behind a body-leading guard reaches the same
  diagnostic. All three sub-checks stay syntactic; transitive
  flow is left to the SSA pass.
- `nil_flow_ssa.go` (`validateNilDeclCalleeFlow`) runs over
  SSA so it picks up shapes the AST pass cannot recognise:
  alias chains ending in a nil literal, phi merges of
  nillable branches, and field reads of `?`-declared fields
  flowing into `!`-required positions.
- The same SSA pass handles the nest-element flow case:
  slice indexes and map lookups whose container field carries
  a nest decl (`![]?`, `![!]?`) lift the element load into a
  nillable proof, and the resulting value flowing into a `!`
  slot surfaces a diagnostic on the use site.
- `nil_check_callee_self_decl_recv.go`
  (`validateNilDeclReceiverGuard`) handles the receiver-guard
  case: a `?.()`-declared receiver dereferenced without a
  leading nil guard surfaces a diagnostic at the first deref
  site. The body of a nillable-receiver method is expected to
  guard before the first use; an unguarded deref is the
  typical nil-receiver panic source. The guard recogniser also
  accepts a check that leads a logical expression
  (`recv == nil || …`, `recv != nil && …`), since Go's
  short-circuit evaluation keeps every operand to its right
  behind it; a receiver check that sits anywhere else in the
  chain does not qualify, because the operands before it are
  evaluated unguarded.

The body-leading dead-guard recogniser lives separately in
`prereq_dead_guard.go`. It reads `vow:cond` prereqs
(`nonnil x`) and surfaces a diagnostic when a body-leading
`if x == nil { return ... }` guard pre-establishes a
condition the prereq already proves at entry — the
sentinel-narrowing surface keeps the diagnostic alongside
the nil-safety pipeline so the same dead-guard recogniser
covers both axes.

### Suppression integration

Every nil-safety diagnostic flows through `vowReportNilSafetyf`,
which routes the report through the same filter chain
`vow:suppress` uses for the must-consume surface. A
function-level suppress drops every nil-safety diagnostic
inside the function range; a line-level suppress drops the
diagnostic at the named line.

## Closable lifecycle pipeline

`closable.go` and `caller_emit.go` track Closable resources from
acquisition to discharge. The pipeline runs in three steps:

1. **Type discovery.** `closableTypeSet` primes a per-pass registry
   of Closable type names. The registry merges two sources: type
   declarations carrying `// vow:define @Closable` (walked by
   `discoverClosableTypeNames` over the package's `TypeSpec` nodes)
   and the `built_in_closables` list from the closable preset
   (`*os.File`, `*net.TCPConn`, `*bufio.Writer`, `io.Closer`).
2. **Caller-side `vow:emit` discovery.** `discoverCallerEmit`
   walks every comment group for statement-scope `// vow:emit X`
   markers. Each marker anchors to a statement line through the
   shared `resolveSuppressLine` resolver. `inferEmitDestination`
   then classifies the site as Goroutine, Callback, Storage, or
   Channel by checking the anchored statement and a parent map
   built on demand; sites that match no recognised shape report
   the analyzer's "destination could not be inferred" warning.
3. **Lifetime check.** `validateClosableLifetime` walks each
   `FuncDecl` for `:=` declarations whose LHS resolves to a
   Closable type, collects the deferred Close receivers
   (`collectDeferredCloseReceivers`) and the statement-scope
   `vow:emit` discharges (`emitDischargesForFunction`), and
   reports an acquisition that reaches the end of the function
   without a recognised discharge. `checkUpcastInvariant` runs in
   the same walk and reports the three upcast sites (return
   statement, call argument, assignment LHS) where a Closable
   value flows into a non-Closable interface.

`isClosableType` resolves the type-axis match across four shapes:
the fully-qualified type string, a single pointer indirection, the
underlying alias / embedded chain, and — for generic type
parameters — the parameter's interface constraint when the
constraint embeds (or names) a registered Closable interface. The
constraint walker (`typeParamConstraintIsClosable`) credits a
parameter declared as `[T io.Closer]` the same way the analyzer
credits a concrete `*os.File`, so the acquisition and upcast
checks fire on generic function bodies without per-instantiation
discovery.

The pass executes after the concept-define info-level diagnostic so
the type registry can rely on the same vow:define discovery walk
that surfaces forward-compat concept notes.

## Lifecycle-pair specialisation

`cond_lifecycle_pair.go` recognises the Closer / paired-resource
shape of a logical-arrow `vow:cond` rule. `isLifecyclePairRule`
consults the signature of the declaring function, the rule's
operands, and the Closable registry (`closableTypeSet`): when one
operand resolves to a Closable signature position and the other
resolves to a distinct nil-bearing reference position (pointer,
interface, map, channel, slice, or function), the rule expresses a
lifecycle pair invariant.

The call-site evaluator in `cond_caller.go` queries the predicate
when it commits a violation and routes the diagnostic through
`describeLifecyclePairCallViolation`, which inserts a `lifecycle
pair` qualifier before the rule rendering. Rules outside the pair
shape — every operand combination that misses the Closer leg, that
walks through a postfix path, that names the same position on both
sides, or whose direction is the structural `->` arrow — report
through the generic logical-arrow diagnostic path unchanged.

The specialisation stays additive on top of the Closable
acquisition tracking documented above: a function may declare a
lifecycle-pair rule and a statement-scope `vow:emit` transfer in
tandem, and the two passes report independently.

## Cross-package considerations

- **Subjects.** The current `findSubjects` is per-package; a sentinel
  declared in one package is recognised as a subject when its
  declaration site is part of the analysis target. Inter-procedural
  subject tracking is not supported.
- **Qualified references.** The four-axis Term comparison handles
  cross-package references through the `fullName` axis. A
  `vow:cond pkg.ErrFoo | nil ` annotation matches a
  `return pkg.ErrFoo` regardless of import aliasing once `types.Info`
  resolves both sides.
- **Generics.** The SSA pass's `findSSAFunction` maps an
  `*ast.FuncDecl` to its `*ssa.Function` by `Syntax()` pointer
  equality. Under the default `buildssa.BuilderMode(0)`, a generic
  origin and its instantiations share the same `Syntax` pointer, so
  leaks inside a generic body are reported at the origin's return.

## Subject scope resolution

Every function-doc marker family accepts an optional `[<subject>]`
qualifier that names a receiver, a regular parameter, or a named
return of the enclosing function. The pipeline keeps the qualifier
shape consistent across families by routing through a single
lexer helper, `stripMarkerScope`, that peels the brackets and
exposes the subject token to the per-family parser.

A shared resolver, `resolveSignatureSubject`, walks the function
declaration in priority order — receiver, then parameters, then
named returns — and reports the first slot whose declared
identifier matches the subject. The resolver is purely
syntactic: it inspects field names rather than types so a
signature without named results never participates and the same
helper covers every marker family without per-family branching.

When a marker carries a scope, the per-family validator skips the
function-level enforcement the same declaration would otherwise
trigger on the enclosing function. The skip avoids misrouting the
contract; higher-order semantics are not covered. The
resolver's `reportSubjectUnresolved` diagnostic surfaces every
authoring typo at parse time so authors never see a silent loss
of contract. The diagnostic category matches the marker family —
`vow[nil-safety]` for `vow:nil`, `vow[sentinel-error]` for the
function-doc forms of the other families, and `vow[closable]` for
caller-side statement-scope `vow:emit`.

## Inconsistency detection layer

After the per-site evaluator runs each logical `vow:cond` rule
at return statements and at call expressions, the analyzer
applies a rule-set inspection layer that reports three classes
of inconsistency the per-site walk would not surface on its
own. Every diagnostic in this layer anchors at the function
declaration because the offence belongs to the rule set or to
the rule set combined with the signature, not to any single
evaluation point. The layer reads the per-function rule set
through `logicalRulesSet` and the per-function signature
through `nilDeclForFunc`, both of which the per-site evaluator
and the signature pass populate before this layer runs.

The three detection paths share a common entries helper that
materialises the rule pool a check inspects. The helper builds
on the contrapositive derivation pass and the transitive
closure expansion, then tags each entry with one of three
kinds — `entryOriginal` for an author-written rule,
`entryContrapositive` for a derived contrapositive, and
`entryTransitive` for a chain closure — so each check filters
the pool to the entries its detection semantic admits.

### Cross-rule contradiction

`checkLogicalRuleSetInconsistencies` walks every function's
rule pool together with the derived contrapositives and
transitive chain closures, then reports every pair whose two
operands the structural `expressionsEqual` and
`negateExpression` helpers identify as a left-hand-side match
with right-hand-side negation. The dedup key combines the
rendered descriptions so the diagnostic never repeats the same
pair regardless of which order the outer loops visit it.

Pairs whose two sides are both derived contrapositives are
dropped because the contrapositive of a rule is logically
equivalent to the rule itself and a contradiction whose two
sides are pure contrapositives reduces to a contradiction on
the source rules (already caught) or to a derived
unsatisfiable-LHS by-product rather than a true rule-set
offence. The filter applies to the `entryContrapositive` pair
shape and leaves every other pair in the walk.

### Signature cross-check

`checkLogicalRuleNilDeclConsistency` evaluates each rule's
operands against the function's `vow:nil` signature. A
`signatureNameContext` bundle, built once per function from the
`*types.Signature` and reused for every operand, names the
receiver, the regular parameters, and the named returns so the
operand resolver matches a reference by identifier name. The
`evaluateOperandFromNilDecl` helper reads `NullnessNonNil`
slots into a fixed truth — `p == nil` decides as false,
`p != nil` decides as true — and leaves nullable and platform
slots undecided so the check stays sound on every shape the
signature does not pin.

The walk reads the original rules and the transitive chain
closures but drops the derived contrapositives because a
contrapositive contradicts the signature exactly when its
source rule contradicts the signature, so emitting both would
surface the same logical offence twice. The collection helper
`collectNilDeclCheckEntries` filters the shared entries pool to
the kinds the cross-check admits.

### Nil-only induction hint

`reportNilOnlyInductionHints` tallies, per signature position,
the count of author-written `=>` rules whose right-hand side
narrows the position to nil (`p == nil`) and the count of
rules that narrow it to non-nil (`p != nil`). A position whose
nil-only count reaches the threshold and whose non-nil count
stays at zero, and whose signature does not already pin a
nullness on that slot, surfaces a hint suggesting the author
declare the position as nullable. The walk skips derived
contrapositives and chain closures because the hint reflects
author intent rather than chain-derived narrowing.

The `referenceNamesSignaturePosition` helper gates the hint to
references that bind to a receiver, a regular parameter, or a
named return so the suggested declaration always points at a
slot the marker can constrain.

## Docstring wording conventions

Production code, testdata fixtures, and the `docs/` set describe
what the code currently is — its capabilities and its
limitations. Comments and docs stay focused on present-tense
facts; they do not narrate project history and they do not
forecast project direction. The conventions below name the
patterns the codebase avoids and the substitutes the codebase
uses so reviewers consult them as a shared checklist.

### Patterns to avoid

- **Past-tense or transition narrative.** `previously`,
  `historically`, `now that`, `was changed`, `has been added`,
  `is now`, `formerly`, `prior`. These attach a project history
  to a code element whose current role is what matters. Use a
  present-tense statement of what the element currently is.
- **Forward-looking narrative.** `in a separate change`, `a
  later change`, `a later wave`, `follow-up wave`, `follow-up
  change`, `next wave`, `future wave`, `forthcoming`,
  `subsequent`, `progressive`, `target state`, `converges on`.
  These point at a project direction the comment cannot
  guarantee. Use a present-tense statement of what is and is
  not covered now (`the analyzer does not recognise this case`,
  `this surface is not enforced`).
- **`legacy` as an adjective.** `the legacy positional grammar`,
  `the legacy textual-expansion path`, `the legacy single-rule
  shape` attach a deprecation tone to a shape that still
  carries its role. Use a descriptor that names the shape
  directly (`the structural positional grammar`, `the textual-
  expansion path`, `the back-compat field`). A bare identifier
  reference (a struct-field name in source such as `Conseq`)
  preserves the identifier and is kept as-is.
- **`retired`, `migrated`, `replaced by` in narrative.** These
  framings describe a transition rather than the present state.
  Use a present-tense description of what the code currently
  does. User-facing error messages keep these spellings because
  the test surface requires the literal text.

### Patterns to prefer

- **Present-tense state describers.** `carries`, `holds`,
  `represents`, `decides`, `pairs`, `routes`, `gates`,
  `currently accepts`, `is not enforced`.
- **Direct capability or limitation statements.** `the analyzer
  parses the suffix but does not enforce its value-level
  content`, `the recogniser does not cover postfix-chained
  references`.
- **Identifier preservation under backticks.** Identifiers that
  exist in the source (`Conseq`, `*ReturnAnnotation`, ``nonzero``)
  stay as-is so readers can grep from documentation to
  implementation.

The conventions extend to commit messages and PR descriptions
that ship alongside the code change: the audit-friendly stance
is to describe what the code currently does and does not do,
not the path the project took to arrive at the change and not
the path the project intends to take next.

These conventions apply to comments and docs in this repository.

## See also

- [DSL reference](dsl-reference.md) for the surface syntax.
- [Annotations](annotations.md) for marker placement rules.
- [Presets](presets.md) for the rule engine dispatch model.

# DSL reference

This page describes the annotation grammar `vow` enforces.
`vow:cond` is the primary annotation surface; `vow:cond`
is the return-requirement-only syntactic sugar that omits the
parameter-requirement half.

The DSL is line-based: an annotation is a single comment line
whose payload follows the marker. `vow:cond` payloads pair
a **parameter requirement** with a **return requirement**
separated by `->`; `vow:cond` payloads carry the return
requirement only.

```
// vow:cond nonnil -> ErrFoo | nil
// vow:cond * -> nonzero Result, ErrFoo | nil
```

Whitespace inside the payload is largely free; commas separate
positions on each side of the arrow.

## Condition grammar

A `vow:cond` rule comes in two shapes that share the same arrow-rule
header. The **structural** rule keeps the positional grammar
where the arrow separates a parameter requirement from a return
requirement; the **logical** rule pairs two expressions with a
forward implication (`=>`) or a biconditional (`<=>`) so authors
write relational constraints in their natural reading order.

### Structural rule

```
condition   := positions '->' positions
positions   := position ( ',' position )*
position    := term ( '|' term )*
term        := paren-positions | predicate-term | type | placeholder | wildcard | literal
predicate-term := predicate type?
predicate   := '!'? predicate-atom
predicate-atom := 'zero' | 'nil'
placeholder := '_'
wildcard    := '*'
```

Position precedence is `comma < pipe < paren`. The left half of
the arrow declares the **parameter requirement** — a positional
list binding to the function's arguments by signature order;
argument names are not part of the surface form. The right half
declares the **return requirement** — the constraint on return
values, with full direct-sum and tuple-sum support. A `vow:cond`
line pairs the two as one case: when a call site provably matches
the parameter requirement, the return requirement applies; when
it does not, the case is silent at that call site.

A `vow:cond * -> X` payload writes the trivial wildcard parameter
requirement that applies at every call site, so the return
requirement `X` applies unconditionally to every return.

### Logical rule

```
logical-rule    := expression arrow expression
arrow           := '=>' | '<=>'
expression      := nil-check | comparison
nil-check       := reference ( '==' | '!=' ) 'nil'
comparison      := reference compare-op value-sum
value-sum       := value-member ( '|' value-member )*
value-member    := 'nil' | integer-literal | string-literal | bool-literal
compare-op      := '==' | '!=' | '<' | '<=' | '>' | '>='
reference       := head postfix-step*
head            := identifier | '$' identifier
postfix-step    := '.' identifier | '[' index-key ']'
index-key       := integer-literal | string-literal | identifier
```

A comparison's right-hand side accepts a single value or a `|`-
separated sum of values. The members admit the bare `nil` token
alongside integer, string, and bool literals; identifier-shaped
members are not part of the value-level sum grammar. Equality and
inequality distribute across the sum — `==` matches when any
member equals the operand, `!=` matches when every member differs
from the operand. Ordered comparisons (`<`, `<=`, `>`, `>=`) take
a single right-hand value; a multi-member sum on an ordered
comparison reports `ordered comparison does not accept a value-
level sum` at parse time.

The left and right operands of a logical rule are each a single
expression. A reference names a parameter, receiver, named return
value, or — via the `$` prefix — a positional return slot (`$1`,
`$2`). Postfix steps walk into a field with `.field`, into a slice
element with an integer literal (`xs[0]`), into a map entry with a
string literal (`m["primary"]`), or into either with an identifier
key that the resolver looks up in the enclosing signature scope
(`m[k]`). Postfix steps chain freely so `cfg.Servers[0].Host`
addresses a nested element directly.

The forward arrow `=>` reads as material implication: when the
left expression holds at a call site the right expression must
also hold. The biconditional `<=>` adds the converse: both
expressions are equivalent at the call site. Both arrows are
distinct from the structural `->`, and the parser rejects mixing
flavours on the same line (`A => B ; -> C`) so the surface stays
unambiguous.

### Multi-rule conditions and left-hand carry

A `vow:cond` line carries one or more rules separated by `;`. When a
rule starts with one of the logical arrows the parser treats it as
a sugar clause and reuses the left-hand side of the preceding rule.
The carry applies only to logical-rule chains, so a sugar clause
after a structural rule or after an atom-rule reference is
rejected at parse time.

```
// vow:cond c.Version >= 2 => c.FeatureX != nil ; => c.FeatureY != nil
```

expands to the same shape as two explicit rules:

```
// vow:cond c.Version >= 2 => c.FeatureX != nil ; c.Version >= 2 => c.FeatureY != nil
```

The biconditional spelling uses the same carry mechanism:

```
// vow:cond e.Tag == "login" <=> e.Login != nil ; <=> e.Logout == nil
```

Both arrows may appear in a single chain — the parser accepts
`A => B ; <=> C` — because the left-hand carry is the only
invariant the sugar form claims. A separate lint may warn when a
chain mixes flavours so authors who do not intend the mix see the
discrepancy, while the parser stays permissive.

The analyzer evaluates each logical rule at every return statement
of the declaring function and at every call site that invokes it.
The judgement is literal-only: an operand decides when its
reference binds to a return slot or argument whose expression is
a syntactic nil, an address-of operator, a composite literal, a
function literal, or a basic literal matching a comparison's
right-hand side. A contradiction surfaces a `vow[sentinel-error]`
diagnostic at the return statement or call expression. A Go
analysis Fact carries each rule across the package boundary so
an importing call site enforces the same contract.

Operands that walk through a postfix path (`cfg.Servers[0]`,
`cfg.Endpoints["primary"]`) reach the decision through two
narrowing tiers. The dominator-guard tier walks the SSA dominator
chain at the call or return site and matches the composed path on
the bound head against each terminating `if <expr> != nil` /
`if <expr> == nil` whose condition reduces to the same FieldAddr /
IndexAddr / Lookup chain. The declarative tier walks the path
through the head's Go type and consults the field-nil registry at
the terminal field — a `[!]!`-style nest body whose value layer
pins map entries as non-nil commits the operand without
consulting the SSA. Operands neither tier reaches fall through
silently so the under-approximation keeps false positives out of
the diagnostic.

#### Closer / paired-resource lifecycle pair

A logical-arrow rule whose operands bind two distinct signature
positions to a Closable type and a nil-bearing reference type
respectively expresses a Closer / paired-resource lifecycle pair
invariant. The grammar is identical to the generic logical-rule
form; the analyzer recognises the pair shape from the operand
types and inserts a `lifecycle pair` qualifier into the call-site
diagnostic so the author distinguishes a pair-lifecycle offence
from a generic logical-arrow contradiction. The recognition uses
the Closable registry maintained by the `closable` preset — see
[`vow:define @Closable`](annotations.md) in the annotations guide
for the declaration syntax.

## Terms

A **term** names one accepted value or type. Five shapes are
recognised:

- **Type name.** A bare or qualified Go identifier:
  `error`, `Result`, `*int`, `pkg.ErrFoo`.
- **Wildcard.** The asterisk `*` is the complete wildcard. It
  matches every value at a position, including the labelled
  subjects the package declares with `vow:define @Sentinel`. A
  wildcard sum member also authorises chain propagation of every
  tracked subject, so authors do not enumerate the open set twice.
- **Placeholder.** The underscore `_` is the label-aware
  fall-through. It admits literals, nil, and non-subject
  identifiers, but excludes labelled subjects so the author must
  list them by name to forward them. `_` is therefore the
  complement of the wildcard's labelled-subject branch: `*` admits
  everyone, `_` admits everyone except the explicitly-labelled
  subset.
- **Sentinel reference.** A package-level var marked with
  `vow:define @Sentinel`, referenced by its identifier
  (`ErrNotFound`) or by its qualified form
  (`store.ErrNotFound`). Chain authorisation is
  expressed by listing the concrete sentinel references the
  function propagates.
- **Literal.** A constant value: `nil`, `true`, `false`, integer
  literals (`0`, `-1`, `0xff`), or double-quoted string literals
  (`"foo"`).

Term matching compares the annotation Term against the return
expression on four axes, in order:

1. **Syntactic identifier** as written at the call site
   (`pkg.ErrFoo`).
2. **Fully qualified name** resolved through `types.Info` — useful
   when the same short name exists in multiple packages.
3. **Named type** as `types.Type.String` prints it. Works for true
   aliases (`type Alias = error` makes the typeName `error`).
4. **Underlying type** after one `Underlying()` step. Reaches the
   structural form behind a defined type
   (`type Bytes []byte` → underlying `[]byte`).

Axis 1 wins first; later axes only fire when earlier ones miss, so
the new types-aware axes never override an authored short-name
match.

## Qualifiers

Qualifiers attach to a term as a single-character suffix.

| Suffix | Name | Meaning |
|--------|------|---------|
| (none) | `QualifierNone` | Lenient default; presets may warn. |
| `?`    | `QualifierOptional` | The value is allowed to be the nil / zero state. Only meaningful for nillable kinds (pointer, interface, slice, map, chan, func, error). |

The parser rejects obviously ill-formed combinations: `?` applied
to predeclared value types (`int?`), stacked qualifiers (`T??`),
and qualifiers without a base type. A non-nil / non-zero
constraint is spelt as the `nonnil` / `nonzero` predicate prefix,
not as a suffix qualifier.

Qualifiers on **const references** (`vow:cond * -> nonzero QValueA`)
are rejected outright: a const's value is fixed at compile time,
so any nilability or non-zero contract on top is semantically
empty.

## Direct sums

A **direct sum** is a brace-enclosed set of alternatives accepted at
one position:

```go
vow:cond * -> ErrFoo | ErrBar | nil
```

A qualifier on the sum as a whole applies to the matched value:

```go
vow:cond * -> (ErrFoo | ErrBar)?
```

Per-member qualifiers (`nonzero Foo | Bar`) attach only to that
alternative — `nonzero Foo` rejects a const-zero `Foo` even while `Bar`
remains lenient.

Members can be:

- **Terms**, as described above.
- **Literal values**: `nil`, `true`, `false`, integer literals
  (`0`, `-1`, `0xff`), or double-quoted string literals (`"foo"`).
  Literal values match return expressions through `go/constant` so
  `0` and `0x00` compare equal.
- **Tuples**, when the sum is used as a tuple-sum (see below).

## Position-wise and tuple-sum forms

A signature has one of two top-level shapes.

### Position-wise

Each comma-separated element describes one return position, left to
right:

```go
vow:cond * -> nonzero Result, (ErrFoo | ErrBar)?
```

The position count must equal the function's return arity. Each
position is either a single term (`nonzero Result`) or a direct sum.

### Tuple-sum

A single direct sum whose every member is a tuple. Each tuple
covers all return positions in lock-step:

```go
vow:cond * -> (nonzero Result, nil) | (nil, nonzero error)
```

Use this when the contract is best expressed as "exactly one of
these multi-return shapes". All tuples in a tuple-sum must have the
same arity, validated by the parser.

A direct sum without tuple members is treated as the position-wise
form: `vow:cond * -> ErrFoo | nil` is the single-position direct
sum at position 0.

## Composition

A rule template lives in a preset's `rules:` map. It carries a list
of parameters and a body that is a fragment of return-annotation
syntax:

```yaml
rules:
  OkErr:
    params: [T, E]
    body: "(nonzero T, nil) | (nil, nonzero E)"
```

A package imports a rule namespace via `vow:import`:

```go
// vow:import result "preset/std/result"
package store
```

Functions then reference the rule with the alias:

```go
// vow:cond @result.OkErr[Result, error]
```

Substitution happens at the string level — the expanded payload is
fed back through the parser, so the expanded form must obey the
same syntax. Rule parameters appear as bare identifiers in the
body; each is textually replaced with the corresponding argument
at the call site.

## Case-style multi-condition

Multiple `vow:cond` lines on the same function describe
**independent cases**. At every return, every case whose
parameter requirement matches enforces its return requirement. A
return that matches no case's parameter requirement is unchecked
— the conservative narrowing from the
single-condition surface carries over so authors do not see
false positives from unanalysed control flow.

```go
// vow:cond nonnil -> 1 | 2 | 3
// vow:cond nil  -> 0
func F(x *Key) int { ... }
```

Authors typically write **disjoint parameter requirements** so
each return sits inside exactly one case. Overlapping parameter
requirements are legal: every applicable case enforces its
return requirement at the overlapping returns, and the author
must satisfy all of them.

Chain authorisation accumulates across every trivial-parameter-
requirement case in the same way: subjects listed in any
`vow:cond` return requirement (or in a `vow:cond * -> ...`)
are authorised for propagation, with the union taken across
cases.

## Signature mirror (`vow:nil`) {#vow-nil-signature-mirror}

The `vow:nil` marker is a signature-mirror declaration that
pins the nullness of each position on a function signature or
a struct field. Unlike `vow:cond`, which expresses a
parameter-requirement / return-requirement pair, `vow:nil`
mirrors the Go signature inline and uses per-position tokens to
declare each position's nullness in isolation.

### Function-signature grammar

```
nilsig       := [ recv-decl '.' ] '(' [ params ] ')' [ returns ]
recv-decl    := positiondecl
params       := positiondecl ( ',' positiondecl )*
returns      := positiondecl ( ',' positiondecl )*

positiondecl := layers nest?
              | nest
              | <empty>           // platform slot
layers       := symbol+ | keyword
symbol       := '!' | '?'
keyword      := 'nonnil' | 'nil'
nullness     := symbol | keyword
nest         := slice-nest | map-nest
slice-nest   := '[' ']' positiondecl?
map-nest     := '[' positiondecl ']' positiondecl?
```

A function with the payload `vow:nil (!,?)` pins parameter 1
as non-nil (`!`) and parameter 2 as nillable (`?`). A method
with `vow:nil ?.()` pins the receiver as nillable and leaves
the parameters at platform. A return list trails the parameter
list separated by one or more spaces / tabs: `vow:nil (!) !,?`
pins one non-nil parameter and two return positions (`!` then
`?`); the parser trims the gap before parsing the trailing
decl list. An empty slot inside a comma-separated list
represents the platform state — the analyzer does not track
that position. `vow:nil (!,)` pins only the first parameter
and leaves the second at platform.

### Nullness tokens

| Token | Keyword alias | Name | Meaning |
|--------|---------------|------|---------|
| `!`    | `nonnil`      | non-nil  | The position must not be nil at the call site, and the body must not overwrite it with nil. |
| `?`    | `nil`         | nillable | The position is explicitly allowed to be nil; the body is expected to guard before deref. |
| (none) | (none)        | platform | No token recorded; the analyzer does not track this position. |

Each nullness state has a symbol form (`!` / `?`) and a keyword
alias (`nonnil` / `nil`). The parser accepts either form on any
position; mixing surfaces within a single payload — for example
`vow:nil (nonnil,?)` — is supported because each slot is parsed
in isolation. The canonical printout keeps the symbol form
because the symbol is the high-frequency surface on the
position-decl mirror, where every slot carries a token and the
punctuation stays terse; the keyword alias is available so an
author can spell the constraint as a word when the symbol reads
less clearly. The keyword form is accept-only and is not emitted
by the analyzer's renderer.

A platform slot ("no token") differs from omitting the marker
entirely: the marker still names the position in the comma-
separated list, but contributes no constraint. Omitting the
marker on the whole function leaves every position implicit
under the platform rules.

### Layers

A layer is one place a value can hold nil. A position names one Go
type, and a type can hold nil at more than one layer: `*error` holds
nil at the pointer, and again at the interface the pointer addresses.
A decl writes one token per layer, outermost first, and that sequence
of tokens is its run.

| Type | Layers that can hold nil |
|--------|-------------------------|
| `int`, `[4]*int` | none |
| `*int`, `[]*int`, `map[string]int`, `error` | one |
| `**int`, `*error` | two |

The count reads a type from the outside in. The type itself counts as a
layer when its own kind can hold nil — pointer, interface, slice, map,
channel, func — and the count then descends into a pointer's element
and asks the same question again. Anything else ends it, the ending
type counted or not on the same rule: `*error` counts the pointer and
the interface it addresses, while `[]*int` counts the slice and leaves
its element layer to the nest body rather than to a second token. An
array holds nil nowhere, so it counts none whatever it contains.

`!?` on a `**int` position pins the outer pointer non-nil and admits
nil at the pointer it addresses.

A run covers the layers from the outermost inward, so a run shorter
than the layer count leaves the layers below it unnamed. The layer
check below reports a run of either wrong length, within the limits
that section gives.

The symbols in a run must be adjacent, and a keyword alias spells
exactly one layer.

Positions and layers are independent axes. A position says which
declaration site a token belongs to — receiver, parameter, return,
field; a layer says which part of that site's type the token
describes. The nest grammar below reaches the layers a container
holds — its element, its key, its type arguments — and each of those is
a position decl carrying a run of its own.

### Nest grammar

The optional nest carries the nullness of nested container
elements. The brackets discriminate the two shapes:

- `[]` declares a slice nest. An optional position decl after
  the brackets pins the element layer (`![]?` — non-nil slice
  of nillable elements).
- `[<keydecl>]` declares a map nest. The keydecl pins the
  map's key layer, the optional decl after the closing bracket
  pins the value layer (`![!]?` — non-nil map of non-nil keys
  to nillable values).

Nests compose recursively, so `![][!]?` reads as a non-nil
slice whose element is a map at platform (no token between
`[]` and `[!]`) with non-nil keys and nillable values.

### Field-inline grammar

A field-position marker reuses the same `positiondecl` grammar
without parentheses or arrow. The payload is a single decl
applied to the field's type:

```
field-payload := positiondecl     // mandatory; empty payload is rejected
```

The field marker sits in the field's doc-comment block or its
trailing line-comment:

```go
type Customer struct {
    // vow:nil !
    ID *string

    // vow:nil ![]?
    Tags []*string
}
```

An empty payload is ill-formed at the field surface (the
platform state is spelled by omitting the marker entirely), so
the parser rejects it with a diagnostic at the field position.

### Arity checks

A marker whose slot count differs from the Go signature —
in either direction — surfaces a diagnostic at the function
declaration so the author aligns the marker with the signature.
Naming more positions is an authoring typo; naming fewer leaves
the reader unsure which trailing slots the omitted decls
intended. The same rule applies to the return list. Marker
tokens (`!` / `?`) inside each slot remain optional per position.

Both axes share one exception: an empty expression covers 0 or 1
positions, because the separator is what carves a slot out and a
lone empty (platform) slot has no other spelling. The rule reads
empty = 0-or-1, `,` = 2, `,,` = 3.

On the parameter axis this is `()`. On the return axis it is an
absent return list, which is what lets `vow:nil (!)` sit on a
function whose single return is a non-pointer — a position no
return token could describe truthfully.

### Layer check

A decl whose token run does not match the layers its type carries
surfaces a diagnostic at the declaration. A token past the last
nil-bearing layer constrains nothing; a run that stops short leaves
the layers below it untracked while reading as the whole contract,
which is what `!` on a `*[]byte` gets — the pointer is pinned and the
slice it addresses is not.

The two directions rest on different halves of the layer count. Naming
more layers than exist needs the count's upper end, so an unsubstituted
type parameter is judged only where its constraint settles the question:
a token on a `~int | ~int64` position surfaces a diagnostic because no
type in that set can be nil, while `any`, `comparable`, and a
method-set constraint stay silent. Naming fewer needs the count's lower
end alone, which is always known because the count stops at the first
type parameter: `**T` holds nil at two layers whatever T binds, so one
token there is short.

A run is judged wherever one is written: at the position itself, and
at each layer its nest body reaches. A position that carries no token
of its own claims nothing about its own type and is not judged for it,
the same way a signature carrying no marker is not; asking for a decl
where none was written is the completeness check's business and rides
on its opt-in.

At a generic nest the count comes from the value a field of the
container holds at that type-argument position; where several fields
reach the same parameter, the deepest one sets the count. A
`type PointerBox[T any] struct{ V *T }` instantiated at `Value` admits
one token, for the `*Value` a read of `V` yields.

A field-inline decl is not judged: a `!` on a field is also read by the
zero-value escape checks, which act on it whatever the field's type.

### Completeness check

Arity asks whether the slot count mirrors the signature;
completeness asks whether each nillable position carries a token.
The two are independent: `vow:nil ()` on a one-parameter signature
satisfies arity through the exception above while leaving that
parameter's nullness unwritten.

The completeness demand is off unless the scope sets
`nil_decl.require_declarations` in its `vow.yaml`, and it exempts
the positions whose nilness is not the author's to state —
`error`, `context.Context`, the empty interface, and unsubstituted
type parameters — along with generated files. See the [annotations
guide](annotations.md#declaration-completeness) for the authoring
view.

### Duplicate marker

A function (or field) that carries more than one `vow:nil`
marker surfaces a duplicate-marker diagnostic at the
declaration. The first parsed signature wins so the rest of the
analysis still has a well-defined contract to consult; the
author resolves the redundancy by removing the extra marker.

## Parse-error policy

Annotations that fail to parse — through expansion or through DSL
syntax — are skipped silently. Successfully-parsed lines all
contribute as independent cases; a malformed line does not affect
its siblings.

Diagnostics emitted by the rule engine itself (arity mismatch,
const-with-constraint, return-vs-signature mismatch) anchor to
the function declaration so the author sees one consolidated
message per offending case rather than a flurry per return
statement.

## `vow:define @Closable` and lifecycle markers

The `vow:define @<Concept>` form attaches a `type` declaration to a
named concept whose semantics live in a preset. The Closable
concept ships built-in and tracks every value of the declared type
from acquisition to discharge.

```
ClosableDecl := "// vow:define @Closable" newline TypeDecl
```

Discharges are surfaced through two existing markers:

- `defer expr.Close()` — the syntactic-level discharge inside the
  same function. The receiver identifier of the deferred call must
  match the acquisition's variable name.
- `// vow:emit <subjects>` — a statement-scope marker whose payload
  is a comma-separated subject list. The marker anchors to the
  same line as a goroutine launch, a call with a callable
  argument, a selector or index assignment, or a channel send; the
  analyzer infers the destination from the anchored statement and
  treats the marker as the hand-off declaration.

## `[subject]` scope qualifier

Every function-doc marker accepts an optional bracketed subject
chain between the marker name and its payload:

```
<marker> ( '[' <subject> ']' )+ <payload>
```

`<marker>` is one of `vow:nil`, `vow:cond`, `vow:use`, or
`vow:emit`; each `<subject>` is a Go identifier or a `$N`
positional reference. The first segment resolves against the
enclosing function's receiver, parameters, and named returns;
each non-head segment resolves against the callback signature
carried by the previous segment's slot, so the chain steps inward
through nested higher-order callbacks. The bracketed form sits
between the marker name and the payload so the existing payload
grammar parses unchanged once the qualifier is stripped.

A single-segment qualifier — `vow:cond[b] x != nil` — retargets
the marker at the parameter `b` of the enclosing function. A
multi-segment qualifier — `vow:cond[outer][inner] z != nil` —
walks through `outer`'s callback signature into the `inner` slot,
and the body references resolve against the innermost callback
signature. Two- and three-level chains are the typical depths in
practice; the parser does not bound the chain depth.

Body references inside a chained marker resolve against the
innermost callback signature. A bare identifier names a receiver,
a parameter, or a named return of that callback. A `$N`
positional reference addresses the callback's N-th return slot
(1-based). The same `$N` form also reaches a positional slot in
the chain qualifier itself — `vow:cond[factory][$1]` lands on the
first return slot of `factory`'s callback signature when that
return is itself callback-typed.

A chain segment that does not name any slot at its depth surfaces
a parse-time diagnostic that quotes the chain in the `[X][Y]...`
surface form and names the failing segment by index. An
intermediate segment that resolves to a non-callback slot
surfaces a separate diagnostic because the chain cannot step
inward through a non-function-typed slot. Subject-scoped
declarations introduce a higher-order contract that the
executable validator does not enforce here — the current pipeline
parses the qualifier, resolves the chain, and skips the
function-level validation that the same declaration would
otherwise trigger on the enclosing function so the contract is
not misrouted.

## Rule-set inconsistency detection

Beyond the per-site evaluator that runs each logical rule at a
return statement or call expression, the analyzer inspects the
rule set as a whole and reports three classes of inconsistency
the per-site walk would not surface on its own. Every diagnostic
in this layer anchors at the function declaration because the
offence belongs to the rule set or to the rule set combined with
the signature, not to any single evaluation point.

### Cross-rule contradiction

A function whose `vow:cond` payload carries two rules that share
a left-hand side and require opposite right-hand side truth
emits a `contradictory vow:cond rules` diagnostic. The check
applies pairwise across the author-written rules, the chain
closures the transitive expansion produces, and the derived
contrapositives, so a contradiction the source rules already
imply through a chain still reaches the diagnostic.

A pair whose two sides are both derived contrapositives is
dropped because the contrapositive of a rule is logically
equivalent to the rule itself, and a contradiction whose two
sides are pure contrapositives reduces to a contradiction on
the source rules (already caught) or to a derived
unsatisfiable-left-hand-side by-product rather than a true
rule-set offence. Pairs that involve an original rule or a
transitive chain closure stay in the walk because each surfaces
a genuine new derivation.

```
// vow:cond a != nil => b != nil ; a != nil => b == nil
func contradictoryDirect(a, b *int) { ... }
// reports: contradictory vow:cond rules on contradictoryDirect:
// vow:cond a != nil => b != nil and vow:cond a != nil => b == nil
```

The biconditional arrow contradicts the forward arrow under the
same pairwise predicate; the check does not gate on the
direction because both arrows demand right-hand side truth align
with left-hand side truth under a matching left-hand side.

### Signature cross-check

A function whose `vow:cond` rule operands the `vow:nil`
signature decides into a boolean combination that contradicts
the rule's direction emits a `vow:cond rule on F contradicts
signature` diagnostic. The signature fixes the truth of every
nil-check whose reference names a position pinned as `!`: a
`p == nil` operand resolves to false at the function boundary
and a `p != nil` operand resolves to true. When both operands of
a rule resolve and the combination fails the direction's truth
condition, the cross-check fires.

Nullable (`?`) and platform positions stay undecided because
the signature permits either nil shape, and comparison operands
(equality or ordered) against scalar literals stay undecided
because `vow:nil` tracks nil presence rather than scalar value.
The two-decisive-operand requirement keeps the diagnostic free
of false positives from one-sided narrowing.

The walk inspects the author-written rules and the transitive
chain closures; derived contrapositives stay out of the walk
because a contrapositive carries the same logical content as
its source under a fixed-truth signature context. A
contradiction the contrapositive would surface always rides on
the source rule the walker already evaluated.

```
// vow:nil (!,!)
// vow:cond a != nil => b == nil
func crossCheckRhs(a, b *int) { ... }
// reports: vow:cond rule on crossCheckRhs contradicts signature:
// vow:cond a != nil => b == nil: a != nil holds at this function
// boundary but b == nil does not
```

### Nil-only induction hint

A function whose author-written rules narrow the same signature
position only to nil across two or more forward implications,
and never to non-nil, emits a hint inviting the author to
declare the position as nullable through `vow:nil`. The hint
reads only the right-hand side of `=>` rules and counts, per
position, the rules that pin it to nil (`p == nil`) and the
rules that pin it to non-nil (`p != nil`). When the nil-only
count reaches the threshold and the non-nil count stays at zero
on a position the signature does not already declare, the
recurring pattern is strong enough to invite an explicit
declaration.

The walk reads only author-written rules. Derived
contrapositives and transitive chain closures stay out of the
count because the hint reflects author intent rather than
chain-derived narrowing: an implication the author wrote on the
function is a stronger signal of design intent than a derivation
a chain expansion produced. A position the signature already
declares with any nullness is skipped because the contract is
already pinned explicitly and the hint would be redundant.

```
// vow:cond a != nil => b == nil ; c != nil => b == nil
func nilOnly(a, b, c *int) { ... }
// reports: b of nilOnly is narrowed only to nil by 2 vow:cond
// rules; consider declaring vow:nil for b
```

## See also

- [Annotations](annotations.md) — where each marker is placed.
- [Architecture](architecture.md) — how Term matching and
  classification are implemented.
- [Presets](presets.md) — how rule libraries are authored.

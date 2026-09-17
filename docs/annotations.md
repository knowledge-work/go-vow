# Annotations

Every contract `vow` enforces is conveyed through doc-comment
markers. This page lists each `vow:` marker, where it must be
placed, and what it changes about the analyzer's behaviour.

Markers must appear on their own comment line (or as the first
token of a multi-line comment block parsed from `Doc.Text()`). The
parser anchors on the line prefix, so a marker buried in prose is
not recognised.

## `vow:define @Sentinel`

Marks a package-level `var` (typically initialized with
`errors.New`) as a tracked subject. The `must-consume` obligation
checks every reference to the variable against its classification
rules and reports leaks.

```go
// vow:define @Sentinel
var ErrNotFound = errors.New("not found")
```

The marker payload (`@Sentinel`) names the concept the var belongs
to. Sentinel and Closable are the concepts with behaviour wired
into the analyzer; the analyzer accepts the declarations with
additional concepts under the same `vow:define @<Concept>` form. A
declaration whose concept has no behaviour registered surfaces an
info-level diagnostic so authors learn that preset configuration
for that concept is pending.

Constraints:

- Only package-level vars are recognised. Function-local declarations
  are not selected by the subject scheme.
- The marker is annotation-scheme specific. A subject pattern like
  `match: "annotation:vow:define @Sentinel"` in the preset YAML
  drives the selection.
- The variable's initializer is unused for selection — any
  expression is acceptable.

## `vow:define @Closable`

Marks a `type` declaration as a resource whose every value must
reach a discharge before it leaves the function that acquired it.
The analyzer tracks each Closable acquisition and reports the
acquisition site when no recognised discharge covers the value.

```go
// vow:define @Closable
type Conn struct { ... }

func (c *Conn) Close() error { ... }
```

Discharges:

- **defer Close**: `defer c.Close()` in the same function clears
  the obligation for the receiver identifier.
- **`vow:emit` goroutine hand-off**: a statement-scope `// vow:emit c`
  marker on a `go` launch line transfers the close obligation to
  the goroutine.
- **`vow:emit` callback hand-off**: a statement-scope marker on a
  call that passes a callable argument transfers the obligation to
  the callee.
- **`vow:emit` storage hand-off**: a statement-scope marker on a
  selector or index assignment transfers the obligation to the
  owner of the shared state.
- **`vow:emit` channel hand-off**: a statement-scope marker on a
  channel send transfers the obligation to the receiver.

Built-in Closable types ship through the `closable` preset's
`built_in_closables` list (`*os.File`, `*net.TCPConn`,
`*bufio.Writer`, `io.Closer`) so standard-library values flow
through the same lifecycle check without a user-side annotation.

Upcast invariant:

The analyzer reports an upcast whenever a Closable value flows
into a non-Closable interface position — a return statement, a
call argument, or an assignment LHS — so the close obligation
cannot vanish behind a wider type. Declare the destination type as
a Closable interface (carrying its own `vow:define @Closable`
marker) or hand the value off explicitly with `vow:emit`.

The recogniser is intentionally aggressive: an ambiguous site
reports so the author opts in to the suppression with
`vow:suppress` rather than the analyzer silently dropping the
lifecycle. Generic-type parameter constraints land in a separate
change; until then, polymorphic Closable handling falls back to
the suppression valve.

## `vow:cond`

Declares a case-style contract on a function — a parameter
requirement on the arguments, an arrow `->`, and a return
requirement on the return values. The analyzer enforces the
return requirement at every return whose path provably matches
the parameter requirement; returns outside the matching scope
are unchecked. See [DSL reference](dsl-reference.md) for the
grammar and the supported parameter-requirement predicates.

```go
// vow:cond nonnil -> ErrFoo | nil
func Lookup(x *Key) error { ... }
```

Multiple `vow:cond` lines on the same function describe
independent cases. Every case whose parameter requirement
matches at a particular return enforces its return requirement
at that return. Authors typically write disjoint parameter
requirements so each return sits inside exactly one case.

### Logical-arrow rules

A `vow:cond` line also accepts a logical form where the two
operands are addressable expressions paired by a forward arrow
`=>` (material implication) or a biconditional `<=>`. Each
expression names an identifier from the enclosing signature —
a parameter, a receiver, or a named return — optionally walked
through postfix steps (`.field`, `[0]`, `["key"]`). The
implication form constrains the right operand when the left
holds; the biconditional adds the converse. The expanded grammar
covers identifier references, slice and map element access,
struct field access, equality and ordered comparisons (`==`,
`!=`, `<`, `<=`, `>`, `>=`), and the `;` left-hand-side carry,
so authors write relational constraints without spelling out the
predicate twice.

```go
// vow:cond c.Version >= 2 => c.FeatureX != nil ; => c.FeatureY != nil
func Configure(c *Config) error { ... }

// vow:cond e.Tag == "login" <=> e.Login != nil ; <=> e.Logout == nil
func Process(e *Event) error { ... }
```

A comparison's right-hand side accepts a `|`-separated sum of
values so a single rule covers several literal cases at once. The
members admit integer, string, and bool literals alongside the
bare `nil` token; identifier-shaped members are not part of the
value-level sum grammar. The equality form distributes as OR
(`op == 0 | 1` matches when the operand equals any member); the
inequality form distributes as AND by De Morgan (`op != 0 | 1`
matches when the operand differs from every member). Ordered
comparisons take a single right-hand value and reject the sum
shape at parse time.

```go
// vow:cond op == 0 | 1 => out != nil
// vow:cond op == 0 => $1 != 1 | 2
```

The analyzer evaluates a logical rule at every return statement of
the declaring function and at every call site that invokes it.
A combination that contradicts the arrow surfaces a
`vow[sentinel-error]` diagnostic at the return statement (callee
side) or at the call expression (caller side). Logical-arrow
rules also travel across the package boundary through a Go
analysis Fact, so an importing package's call sites enforce the
same contract as the same-package path.

The evaluator commits an operand through three layers, tried in
order until one decides:

1. **Literal shape.** A reference that binds to a return slot or
   argument whose expression is a syntactic nil, an address-of
   operator, a composite literal, a function literal, or a basic
   literal matching the comparison's right-hand side decides on
   sight.
2. **SSA refinement.** An identifier bound to a parameter, a local
   alias of an address-of declaration, a field with a `vow:nil`
   declaration, or a slice or map element constrained by a
   `vow:nil` nest commits through the forward walk the nil-safety
   pass already drives. The same layer narrows a parameter
   through an enclosing `if param != nil` (or `if param == nil`)
   guard, so a block-scoped argument decides without an explicit
   literal.
3. **Derived rules.** Every forward implication (`=>`) carries a
   contrapositive (`!B => !A`) and joins the per-function chain
   closure that links every implication whose intermediate operand
   matches the next rule's left-hand side, up to a fixed depth.
   The closure surfaces a violation that no single rule reaches on
   its own.

A site processes each author-written rule alongside its
derivations under an original-first walk: the contrapositive and
the chain closure stay silent whenever the original already
commits, so the same offending claim never reports twice. A
chain-driven violation announces the source rules through a
`transitive closure of vow:cond X => Y and vow:cond Y => Z`
prefix so the reader recognises which rules combined to surface
the diagnostic.

Operands no layer reaches — postfix-chained references inside a
narrowing context, function calls, identifiers whose value depends
on flow shapes outside the evaluator's coverage — stay undecided.

### Closer / paired-resource lifecycle pair

A logical-arrow rule whose operands bind to two distinct signature
positions — one whose type satisfies the Closable registry (the
`closable` preset's built-in entries, or a user-declared type
carrying `vow:define @Closable`), the other whose type is a
nil-bearing reference shape (pointer, interface, map, channel,
slice, or function) — expresses a Closer / paired-resource
lifecycle pair invariant.

```go
// vow:cond closer != nil => resource != nil
func Attach(closer io.Closer, resource *Resource) { ... }
```

A call-site violation surfaces a `lifecycle pair` qualifier in the
diagnostic so the author distinguishes a pair-lifecycle offence
from a generic logical-arrow contradiction:

```
vow[sentinel-error]: call violates lifecycle pair vow:cond closer != nil => resource != nil: closer != nil holds at this call but resource != nil does not
```

The qualifier rides on top of the generic call-site evaluator —
rules whose operand types do not form a Closable / reference-like
pair, whose operands bind to the same position, or whose direction
is the structural `->` arrow fall back to the generic diagnostic —
and the Closable acquisition tracking that `vow:define @Closable`
covers stays on its own axis. A function may declare a lifecycle-
pair rule and a statement-scope `vow:emit` transfer in tandem, and
the two passes report independently.

## `vow:cond`

Syntactic sugar for the trivial-parameter-requirement
`vow:cond` form: `vow:cond X` is equivalent to
`vow:cond * -> X`, where the `*` wildcard means "applies
to every call site". The payload is the return-requirement half
of the DSL signature; see [DSL reference](dsl-reference.md).

```go
// vow:cond * -> nonzero Result, ErrFoo | nil
func Lookup() (Result, error) { ... }
```

Behaviour:

- Listing a subject in any direct-sum position implicitly authorizes
  chain propagation for that subject (the function may return it
  without being flagged as a leak).
- Multiple annotation lines (any mix of `vow:cond` and
  `vow:cond`) accumulate as case-style cases — applicable
  cases all enforce their return requirements and trivial-
  parameter-requirement cases all contribute to chain
  authorisation.
- Arity mismatch between the signature and the function's declared
  return values is reported once at the function declaration per
  offending case, and the per-return checks for that case are then
  skipped to avoid cascading diagnostics.

## Chain authorisation via `vow:cond`

Chain authorisation — letting a function forward a tracked sentinel
to its caller without being reported as a leak — is conveyed
exclusively through a `vow:cond` signature that lists the
concrete sentinel in some direct-sum position:

```go
// vow:cond * -> ErrNotFound | nil
func Lookup(id string) error { ... }
```

Listing the concrete sentinels is the recommended form: the
contract communicates exactly which values may flow out and the
caller (or a future reader) can audit the propagation surface from
the annotation alone.

Boundaries that genuinely cannot enumerate their forwarded set
should narrow the function's signature (split the open-set call
into a typed wrapper that holds the variants the boundary cares
about, or expose the transport error through a fresh sentinel
the package declares itself).

## `vow:emit`

Declares that the function may emit one or more tracked subjects in
its return values. The marker is the output-side counterpart of
`vow:use`: every subject the function lists is recognised as a
chain-authorisation source on the caller side, so callers may
propagate the call's result without listing the subject in their
own signature, and the callee body is validated to actually return
at least one of the declared subjects somewhere.

```go
// vow:emit ErrNotFound
func Lookup(id string) error { ... }
```

The payload is a comma-separated list of emit specs. Each spec
takes one of three shapes that coexist on the same surface:

- **Name-based subject**: `ErrFoo` declares the function may
  return the named subject from one or more of its returns. The
  shape carries no return-slot binding.
- **Positional discrimination**: `$N ErrFoo` declares the function
  returns the named subject at the 1-based return slot `N`, so the
  caller binds the subject to a specific position even when the
  signature exposes multiple return values of the same type.
- **Passthrough mapping**: `name -> $N` declares the function
  reads the named parameter and emits it through the 1-based
  return slot. The shape carries no subject identifier because the
  parameter binding establishes the propagation surface.

The dispatcher distinguishes the three shapes by token presence: a
`->` arrow routes to the passthrough form; a `$N` prefix without
the arrow routes to the positional form; everything else falls
into the name-based form.

```go
// vow:emit ErrFoo
// vow:emit $1 ErrFoo
// vow:emit err -> $1
// vow:emit err1 -> $1, err2 -> $2
```

Behaviour:

- Caller-side: a call to a `vow:emit X` callee whose result flows
  into the enclosing function's return is credited as a chain-
  authorisation source for X. The caller does not need its own
  signature to list X — propagation through the call is
  authorised by the callee's declaration.
- Callee-side self-validation: a `vow:emit X` function whose body
  never returns X surfaces a diagnostic at the function
  declaration so an empty-emission contract is not silently
  accepted.
- The recognition is per-subject: `vow:emit ErrFoo, ErrBar` declares
  ErrFoo *and* ErrBar; returning a subject not on the list still
  leaks unless covered by another chain-authorisation surface.

Caller-side hand-off:

A statement-scope `// vow:emit X` marker also declares that the
acquisition named X is handed off at the statement's site. The
caller-side surface discharges the Closable lifecycle check (see
`vow:define @Closable`) and lets the analyzer infer the
destination context from the AST. The recogniser anchors the
marker to its line per the `vow:suppress` resolution: trailing
comments claim the statement on the same source line, and leading
comments claim the next statement-bearing line.

Recognised destinations (priority order):

1. **Goroutine** — the marker sits on a `go` launch statement, so
   the close obligation transfers to the goroutine body.
2. **Callback** — the marker sits on a call expression that passes
   a callable argument, so the callee owns the close.
3. **Storage** — the marker sits on an assignment whose LHS is a
   selector or index expression, so the owner of the shared state
   takes the close.
4. **Channel** — the marker sits on a channel send, so the receiver
   takes the close. The destination resolves but dedicated fixture
   coverage is not covered.
5. **Wrapped return** — the marker sits on a return whose value
   flows through a function registered through the
   passthrough-emit obligation of a loaded preset, so the wrap
   call's callee carries the propagation surface. The sample
   preset under `example/werror.yaml` shows the obligation shape
   adopters copy and adapt for their own wrap library.

A site that matches none of the recognised shapes surfaces a
warning so the author either repositions the marker or annotates
the suppression with `vow:suppress` explicitly.

## `vow:nil` and the nil-safety surface

The `vow:nil` marker is a signature-mirror declaration: the
payload spells the receiver, parameter, and return positions
inline with the Go signature and uses per-position tokens to pin
each position's nullness. The analyzer drives every nil-safety
check — caller-side argument and receiver checks, callee-side
self-validation, cross-package fact-driven trace — off the same
parsed signature, so the marker is the single authoring point
for nil-safety contracts.

```go
// vow:nil (!,?)
func Process(p *Request, opts *Options) {}
```

The payload above pins parameter 1 as non-nil and parameter 2 as
nillable; the function has no receiver layer and no return
positions in the contract. The full grammar lives in the [DSL
reference](dsl-reference.md#vow-nil-signature-mirror); the
authoring summary is:

- `!` (or `nonnil`) — non-nil required. The position must not be
  nil at the call site, and the body must not overwrite it with
  nil.
- `?` (or `nil`) — nillable. The position is explicitly allowed
  to be nil; the callee's body is expected to guard before
  dereference.
- (none) — platform; the analyzer does not track this
  position. Useful for partial pins (`vow:nil (!,)` pins only
  the first parameter).

Each nullness state has a symbol form and a keyword alias; the
parser accepts either at every slot. The canonical printout
stays in the symbol form because that surface dominates on the
position-decl mirror, where every slot carries a token and the
punctuation stays terse.
- Receiver prefix (`!.()` / `?.()`) — applies to the method's
  receiver. Omitting the prefix leaves the receiver
  unconstrained.
- Return list — comma-separated decls trailing the parameter
  list (`vow:nil () !,?`).

A token declares one *layer* of the position's type — one place a
value there can hold nil. Most types have exactly one, so most slots
carry exactly one token; a type that can hold nil more than once takes
one token per layer, outermost first, and `!?` on a `**Request` pins
the outer pointer non-nil while admitting nil at the one it addresses.
A run that names more layers than the type carries surfaces a
diagnostic, and so does one that names fewer. The [layer
rules](dsl-reference.md#layers) spell out how the count is read.

The slot count in the payload must mirror the Go signature
exactly, on both the parameter and the return axes. Marker
tokens within each slot stay optional, but a comma-separated
slot must exist for every parameter and every return position;
missing or extra slots surface an arity diagnostic at the
declaration. One exception relaxes both axes: an empty expression
covers 0 or 1 positions, since a separator is what carves a slot
out. `()` covers a 0- or 1-param signature; an absent return list
covers a 0- or 1-return signature.

The return half is what lets a parameter contract sit on a
function whose single return is a non-pointer:

```go
// vow:nil (!)
func renderName(p *Person) string { ... }
```

A token written there anyway stands at a layer the type does not have,
and surfaces a diagnostic at the declaration: `?` would claim a
`string` can be nil, and `!` would state what the type already
guarantees.

### Field-inline declarations

`vow:nil` also annotates struct fields directly. The marker sits
in the field's doc-comment or trailing-comment slot and carries
a single position decl:

```go
type Customer struct {
    // vow:nil !
    ID *string

    // vow:nil ?
    Email *string

    // Phone has no marker, so the field stays at platform.
    Phone *string
}
```

A `!` field declaration is a two-sided contract. On the
construction side, whoever builds the struct owes the field a
non-nil member. On the reading side, that obligation is what
entitles a reader to treat the field as non-nil without a guard
of its own — so a `!`-declared field flowing into a
`!`-required parameter slot stays silent, while a `?`-declared
field in the same position surfaces a caller-side diagnostic at
the call site.

Two checks hold the construction side up. The first reads an
explicit nil: a literal nil supplied at a `!` field contradicts
the declaration and surfaces a diagnostic at the nil expression.
Both write shapes are recognised — an assignment through a
selector (`c.ID = nil`, on a pointer or a value receiver alike)
and a composite-literal element, keyed (`Customer{ID: nil}`) or
positional. A typed-nil conversion (`(*string)(nil)`) counts as a
literal nil. Right-hand sides whose nil-ness needs dataflow to
decide — a call result, a read of another nillable field — stay
unreported at this surface.

A value whose nil-ness comes from a declaration elsewhere is read by
a second layer of the same check: a `?`-declared field, a
`?`-declared parameter, or an alias of either, written at a `!`
field, is reported even though no nil appears in the source. A guard
that proves the value non-nil where the write runs discharges it,
under the same matcher the read side uses — so one guard covers both
passing the value and storing it.

```go
// vow:nil !  — on Box.Ref
// vow:nil ?  — on Box.Maybe

p.Ref = src.Maybe        // reported

if src.Maybe != nil {
    p.Ref = src.Maybe    // silent
}
```

A write is reported only where a guard could have discharged it. The
declaration asks the author to guard the value before using it, so a
diagnostic that survives the guard would refuse the very remedy the
annotation prescribes; where no guard can be matched against the
write, the write goes unreported instead. That silence has a
consequence worth naming: an identifier source is matched by value
identity, and the place to ask a guard about comes from the store the
assignment compiles to, so a bare identifier supplied as a
composite-literal element is not reported. `p.Ref = q` is; `Box{Ref:
q}` is not. A field source carries its own location and is reported
in either form.

A call result is read when the callee declares it. A callee whose
`vow:nil` signature pins its single return slot as `?` hands back a
value the contract says may be nil, and that value arriving at a
`!`-required position — an argument, a return slot, a field write —
is reported. Binding the result and guarding it is the remedy, since
there is nothing to guard about a call written inline:

```go
// vow:nil () ?
func find() *int { ... }

consume(find())           // reported
p.Ref = find()            // reported

r := find()
if r != nil {
    consume(r)            // silent
}
```

A callee with no signature is at platform, and the absence of a
declaration is not a claim that the result may be nil — reading it as
one would report every use of every undeclared function's result. A
callee returning more than one value is not read either: reaching a
use site through a spread needs a per-slot mapping the checks do not
perform, so pinning a nullness would attach the wrong slot's
contract.

The second reads an omission, which produces the same nil less
visibly: a struct literal that names no member for a `!` field
leaves it at its zero value. The omission is reported once the
literal escapes the expression that built it — as a return
result, a call argument, the receiver a method call is invoked
on, the value of a channel send, the initialiser of a
package-level var, or the right-hand side of an assignment to
anything but a local variable.

```go
return Box{}    // reported: Ref is left nil
consume(Box{})  // reported
h.Inner = Box{} // reported

b := Box{} // silent: Ref can still be filled before b escapes
b.Ref = x
return b
```

A value bound to a local stays silent while a write can still
reach the field: a store guaranteed to run before the value
escapes discharges the obligation, however the value was built —
a literal, `var b Box`, or `new(Box)`. A write below the top
level counts too, at the nested field or at the whole struct
holding it. Past the escape there is nothing left to reach, so
filling a field afterwards is not accepted; hand the value over
initialised instead. An array is judged element by element, and
an element the code never addresses owes every `!` field.

```go
var h Holder
h.Inner.Ref = x
return h                     // silent
return Holder{Inner: Box{}}  // reported: the inner Ref is left nil
return []Box{{Ref: x}, {}}   // reported: the second element owes Ref
```

An allocation handed to a call, or captured by a function literal
that writes through the capture, is left to whoever received it.
An element reached through an index this check cannot resolve
leaves its array unjudged, since no one member can be said to
have been filled.

Writes and omissions at `?`-declared and platform-state fields
stay silent throughout, because neither declaration claims the
field is non-nil.

A `?` declaration also places an obligation on the field's own
readers: dereferencing the field without a nil guard surfaces a
diagnostic, the same way a `?`-declared receiver owes a guard
before its first dereference. Both the explicit operator
(`*h.Maybe`) and a selection that has to step through the pointer
count — reaching a field of the pointee, or a method with a value
receiver. Naming a method with a pointer receiver does not: the
pointer is passed along untouched, so a nil there is the method
body's business rather than the call site's.

```go
// vow:nil ?  — on Holder.Maybe

return h.Maybe.N        // reported
return h.Maybe.ByValue() // reported — the receiver is built from the pointee
return h.Maybe.ByPointer() // silent — the pointer is passed as it stands

if h.Maybe != nil {
    return h.Maybe.N    // silent
}
```

One diagnostic is reported per field per receiving value, so a
field read repeatedly under one missing guard surfaces once. The
guard that discharges a dereference is the same one that
discharges a call argument, so a single guard covers both uses.

The two shapes above are the whole of what this check recognises.
Other ways of reaching through a nillable field are outside it,
including ones that panic just as readily: indexing or ranging a
pointer-to-array field (`h.Cells[0]`), calling a nil func field,
and dispatching a method on a nil interface field. Each of those
is a different question about which uses of a kind are hazardous,
rather than a wider reading of the two shapes here.

The marker is read on the fields of a declared struct type. Two
positions fall outside that and are reported rather than ignored:
a field of an anonymous struct type, and an embedded field. Neither
can carry a contract — the first belongs to a type nothing names,
the second has no field name to key a declaration against — so the
diagnostic points at the marker instead of leaving it inert.

```go
type Outer struct {
    Inner struct {
        // vow:nil !  — reported: name the struct type to declare here
        Ref *Foo
    }

    // vow:nil !  — reported: declare on Foo's own fields
    *Foo
}
```

A marker on a field of a generic struct type applies through every
instantiation of that type. The declaration is written once, on the
generic type, and a field access such as `GenBox[int]{}.Maybe`
reads it — the container's type arguments do not enter into which
declaration a field carries.

### Caller-side checks

Three passes inspect every call site:

- **Argument check.** For every parameter the callee's signature
  pins as `!`, a statically-nil argument surfaces a diagnostic.
  The judgement is conservative on the false-positive side: only
  the bare `nil` literal, a typed-nil conversion (`(*T)(nil)`),
  and a selector reading a `?`-declared field qualify. Other
  shapes (variable reads, function returns) fall through
  unreported and are left to the callee-side flow pass.
- **Receiver check.** For every method call whose receiver is
  statically nil, the callee's receiver decl drives one of three
  branches: a `!.()` decl surfaces a strict diagnostic that
  quotes the contract; a `?.()` decl or an omitted receiver
  layer on a `vow:nil` signature stays silent (the contract
  admits a nil receiver, or the author has not claimed a
  constraint); an undeclared callee surfaces a default-warn
  diagnostic, because nil-receiver method calls are Go's most
  common runtime panic source. A same-package callee that
  carries a `vow:cond` annotation without a `vow:nil`
  signature also stays silent — the author has acknowledged
  vow's surface but has not claimed a receiver constraint.
  Cross-package callees with no exported signature fact fall
  into the default-warn branch because the imported fact
  mechanism carries no "vow declared but no recv decl" signal.
- **Dead-guard check.** A nil guard standing on a local bound to
  a return slot the callee pins as `!` can never run, so the
  guard surfaces a diagnostic. This mirrors the callee-side
  dead-guard recogniser across the call boundary: that surface
  retires the defensive check on the parameter side, this one
  retires it on the call-result side.

  ```go
  // vow:nil () !
  func newBox() *Box { return &Box{} }

  func use() {
      b := newBox()
      if b == nil { // reported: the call cannot return nil
          return
      }
  }
  ```

  The recogniser is AST-only and block-local. It stays silent on a
  reassignment, an address-of hand-off, or an increment of the
  bound local; on a function literal in the statement; on a guard
  body that falls through instead of short-circuiting; on an `if`
  carrying an init clause; and on a guard nested in an inner
  block. Each shape either puts the value outside the contract's
  description or moves the write out of syntactic view.

  The same recogniser also covers the `vow:cond`-driven caller
  narrow. When a callee ties one return slot's nilness to another
  via a forward implication or biconditional (`err == nil <=> $1
  != nil`), an `if err != nil { return }` guard on the paired
  local proves the counter-branch dead, so the continuation runs
  under `err == nil` and the rule's forward direction carries the
  target slot to non-nil. A subsequent nil check on that slot is
  reported the same way the `vow:nil` case is.

  ```go
  // vow:cond err == nil <=> $1 != nil
  func newBox() (b *Box, err error) { return &Box{}, nil }

  func use() {
      b, err := newBox()
      if err != nil {
          return
      }
      if b == nil { // reported: the rule proves b non-nil past the err-guard
          return
      }
  }
  ```

  The leave-alone shapes match the `vow:nil` surface — reassignment,
  address-of, function literals, init-clause guards, and non-short-
  circuit guard bodies all keep the recogniser silent. The rule
  itself must be a bare-reference nil check on both sides (`<name>
  == nil <arrow> <ref> != nil`); path-bearing operands and other
  comparison shapes fall outside the caller-narrow surface.

  A companion recogniser reads the same state machine from the
  other side: a selector-driven dereference of a call-result local
  whose paired guard has not yet short-circuited its counter-branch
  is a safety violation, because the rule leaves that slot possibly-
  nil until the guard runs. Together the two surfaces catch the
  two ways a caller can misread the rule — keeping a defensive
  check the rule has already answered, and skipping the check the
  rule requires.

  ```go
  // vow:cond err == nil <=> $1 != nil
  func newBox() (b *Box, err error) { return &Box{}, nil }

  func use() {
      b, _ := newBox()
      _ = b.Field() // reported: the paired guard on `_` never runs,
                     // so the rule leaves b possibly-nil at this deref
  }
  ```

  The safety recogniser is block-local like the dead-guard one and
  does not descend into nested blocks from the outer scan; a deref
  hidden behind an if-guard the outer scan cannot track stays
  silent at the outer scope. Function literals, reassignment of
  the bound local, and address-of hand-offs all clear the pending
  binding, matching the dead-guard's leave-alone stance.

### Callee-side self-validation

Four passes validate the callee's own body against its
declaration. Each pass reads the parsed signature and reports
contradictions independently, so a single function can surface
multiple diagnostics when its body violates several axes.

1. **AST literal-nil writes.** Two sub-checks share the AST
   pass. The reassign check fires when a `!`-declared
   parameter (or receiver) is overwritten with the bare nil
   literal — or a typed-nil conversion — inside the body; the
   return check fires when a `return` statement supplies the
   same literal at a `!`-declared return slot. Both shapes are
   purely syntactic; transitive flow is left to the SSA pass.
2. **SSA forward flow.** A nillable value (a `?`-declared field
   read, an alias chain ending in a nil literal, a phi merge of
   nillable branches) flowing into a `!`-required argument or
   return position surfaces a diagnostic at the use site. The
   SSA walk picks up shapes the AST pass cannot recognise.

   A nil guard discharges the diagnostic: once the value is
   proved non-nil where the use site runs, the declaration's
   admission of nil no longer decides. The proof accepts a
   dominating `if v != nil` branch, an `if v == nil` early
   return, the else side of an `if v == nil`, and a merge block
   whose every incoming edge establishes non-nil on its own.
   The guard applies to the value the use site actually passes,
   so guarding an alias narrows that alias. A variadic position is
   the exception: the call passes one folded slice rather than the
   individual arguments, so no value there can be matched against
   a guard and the diagnostic stands.

   A guard on a `?`-declared field discharges a later read of the
   same field, matched by the value the field is read through and
   the field itself — so a guard on `h.Maybe` says nothing about
   `g.Maybe` or about `h.Other`. Because a field is memory rather
   than a value, the proof ends where something can write it: an
   assignment to that field, a replacement of the whole struct, or
   a call handed the struct, any of which between the guard and the
   read puts the diagnostic back. A read inside a loop whose later
   iteration writes the field is declined rather than reasoned
   about.

   ```go
   // vow:nil ?  — on Holder.Maybe

   if h.Maybe != nil {
       consume(h.Maybe) // silent
   }

   if h.Maybe != nil {
       mutate(h)        // may write the field through the pointer
       consume(h.Maybe) // reported again
   }
   ```

   A write that reaches the field through a pointer a callee
   already held, or through a package-level variable, is not
   visible to this check; recognising it needs alias analysis.

   ```go
   // vow:nil (?)
   func use(p *int) {
       if p == nil {
           return
       }
       consume(p) // silent: the guard proves p non-nil here
   }
   ```
3. **Nest layer flow.** A read through a container whose decl
   carries a nest body takes its nilness from the layer the read
   lands on. A slice index or map lookup on a field declared
   `![]?` or `![!]?` lifts the element load into a nillable
   proof. A read of a generic container's field whose type
   reaches one of the container's type parameters reads the token
   run the enclosing signature's decl carries at that
   type-argument position — the read itself from the run's first
   token, each dereference of it from the token after, so a field
   declared `*T` sits one layer outside the type argument. Either
   way the resulting value flowing into a `!` slot surfaces a
   diagnostic on the use site.
4. **Receiver dead-guard.** A `?.()`-declared receiver
   dereferenced without a leading nil guard surfaces a
   diagnostic at the first deref site. The body of a
   nillable-receiver method is expected to guard the receiver
   before the first use; an unguarded deref is the typical
   nil-receiver panic source. A guard written together with
   the checks it protects (`if recv == nil || recv.dep == nil`,
   `if recv != nil && recv.dep != nil`) counts, because Go
   evaluates the operands to its right only once the receiver
   check has passed.

A symmetric callee-side check fires when a position the
signature pins as `!` — parameter or receiver — sits behind a
body-leading `if x == nil` guard whose then-branch short-
circuits: the guard is unreachable because the contract pre-
establishes non-nil at entry, so the diagnostic asks the author
to revisit the decl. The same dead-guard recogniser applies to
`vow:cond` prereqs that pin a position as `nonnil`.

### Cross-package trace

The analyzer exports two Go analysis Facts so caller-side checks
running in an importing package see the contract regardless of
where the callee's declaration sits:

- `signatureNilFact` carries the parsed signature (receiver,
  parameters, returns) at the function's type-checker Object.
  The caller-side argument and receiver checks consult the fact
  through a same-package-first / imported-fact-fallback
  dispatch, so the cross-package call surfaces the same
  diagnostic a same-package call would have produced.
- `fieldNilFact` carries the parsed field decl at the field's
  type-checker Object. The argument check resolves a selector
  expression's terminal field through the same dispatch.

Both facts are eligible only on exported package-level objects
(the Go analysis Facts mechanism silently drops unexported
ones), so unexported callees and unexported fields stay
recognised only within their declaring package.

### Axis separation from `vow:cond`

The `nonnil` predicate on a `vow:cond` prereq
(`vow:cond nonnil err -> ErrFoo | nil`) is a
sentinel-narrowing surface, not a nil-safety surface. It pins an
argument position as non-nil to authorise an arrow-rule case in
the return-requirement matcher; the analyzer keeps the body-
leading dead-guard recogniser for the prereq so the function's
own contract still surfaces dead-guard diagnostics, but the
prereq does not drive caller-side argument or receiver nil
reporting. Authors that want caller-side nil-safety checks
declare the constraint through `vow:nil` instead.

### Declaration completeness

By default a position without a marker is simply untracked. A scope
that wants the contract written down everywhere opts in through its
`vow.yaml`:

```yaml
nil_decl:
  require_declarations: true
```

With the key set, a parameter or return whose type can hold nil and
whose slot carries no `!` / `?` reports at the declaration:

```
store.go:14:1: vow[nil-decl]: vow:nil leaves parameter id, return 1 without a nullness decl
```

One diagnostic names every uncovered position of the signature. The
rule reads the Go signature rather than the arity gate, so `vow:nil
()` on a one-parameter signature — well-formed under the
empty-expression exception — still reports the parameter.

Four exemptions keep the demand where an author has something to say:

- **Convention-bearing types.** `error` (nil means success),
  `context.Context`, and the empty interface carry a Go-wide nilness
  convention that a per-position decl would only restate.
- **Type parameters.** An unsubstituted `T` may be instantiated with
  `int`, so its constraint — not the author — decides whether nil is
  representable.
- **Generated files.** A source carrying the `Code generated ... DO
  NOT EDIT.` header is skipped: the contract belongs on whatever the
  generator reads.
- **The receiver and nest layers.** The rule asks for the outermost
  nullness at each parameter and return. Receiver decls (`!.()`) and
  element layers (`![]?`) stay optional.

Completeness diagnostics carry their own `nil-decl` category, separate
from `nil-safety`, so a CI integration can treat "the contract here is
unwritten" as advisory while nil-safety violations stay blocking —
`go/analysis` has no severity axis, so the category is what carries
the distinction. Pairing the opt-in with `--changed-files` narrows the
demand to the files a change touches, which is how an existing
codebase adopts the rule without rewriting every signature first.

### Suppression

Every nil-safety diagnostic flows through `vow:suppress`, so an
author who reads the leak and chooses to defer the fix silences
it through the same valve used for the must-consume surface. The
completeness diagnostics above route through the same valve.

## `vow:discharged`

A callee-authored, function-only marker that asks the analyzer to
treat the function as a black box on the caller side. The name is
deliberately literal: it is the *callee* declaring "I take
responsibility — suppress every caller-side check that would
otherwise inspect this call". The pair on the caller-side surface
is `vow:suppress`, which silences the **current** function's must-
consume diagnostics; `vow:discharged` silences the analyzer at
every *upstream* call site that propagates through this function.

```go
// vow:discharged
func TrustedLeaf() error { ... }
```

Behaviour:

- Caller-side: any reference to the marked function (passing a
  subject in, returning the result back out) short-circuits the
  caller's must-consume walk and the caller's return-position
  validation. The caller propagates without listing the marked
  function's subjects on its own signature.
- Callee-side: the marker takes no payload (a `vow:discharged:
  <reason>` form is rejected). The contract is structurally
  asymmetric with `vow:suppress`, which **requires** a reason
  because it is a caller-authored debt marker. Here the marker is
  the callee's declaration; the function declaration itself is
  the responsibility surface.
- Position: only function-doc comments are honoured. A
  `vow:discharged` placed on a line comment surfaces a
  parser diagnostic so authors do not silently lose the
  declaration.

Axis-2 (callee vs. caller authorship) example:

```go
// Callee-authored, suppresses *callers':
//
// vow:discharged
func DangerousBoundary() error { ... }

// Caller-authored, suppresses *its own* must-consume diagnostic:
func usesBoundary() error {
    // vow:suppress: routed through the package error sink
    return SomeOtherCall()
}
```

## `vow:suppress`

A caller-authored marker that drops a must-consume diagnostic on
the line or function the author writes it on. The payload is a
mandatory reason — the marker is a debt signal, not an assertion,
so the analyzer enforces the documentation surface.

```go
func dropsLeak() error {
    // vow:suppress: discharged in the upstream typed wrapper
    return ErrFoo
}
```

Behaviour:

- The marker affects must-consume diagnostics only; other diagnostic
  classes (return-position mismatch, parser errors, concept-system
  info diagnostics) flow through untouched.
- Two scopes: a function-doc occurrence silences every must-consume
  diagnostic inside the function; a line comment (trailing or
  leading the statement, mirroring `//nolint`) silences the
  statement on that line. A trailing comment wins over a leading
  comment on the next line when both could anchor.
- Bare `vow:suppress` and `vow:suppress:` (empty reason) are
  diagnosed at parser time; the analyzer refuses to silence a leak
  without a reason because the surface is for debt tracking, not
  assertion.

## `vow:use`

A caller-authored discharge assertion. The author claims "the
named subject is used here" and the analyzer drops the must-
consume diagnostic for that subject at the asserted scope. The
marker is the dual of `vow:suppress`: both silence must-consume
diagnostics, but `vow:suppress` is a debt marker (requires a
reason) and `vow:use` is an assertion (no reason needed, because
the claim itself is the documentation).

```go
func observe() {
    // vow:use ErrFoo
    handle(ErrFoo)
}
```

Behaviour:

- Subject-keyed: a `vow:use ErrFoo` on a statement that leaks
  both ErrFoo and ErrBar silences only ErrFoo; the ErrBar leak
  still surfaces.
- Two scopes mirror `vow:suppress`: a function-doc occurrence
  applies across the body; a line comment anchors to the trailing
  or leading statement.
- Payload syntax: a comma-separated subject list. Whitespace and
  empty fragments fold; an authored-but-empty payload (bare
  `vow:use`) surfaces a parser diagnostic so a malformed claim
  does not silently silence a leak.
- Callee self-validation: a function-doc `vow:use X` declares the
  function consumes X somewhere in its body. The analyzer surfaces
  a diagnostic at the function declaration when the body has no
  discharge site for X, so the declaration cannot drift from the
  body's actual behaviour. A discharge site is an identifier
  reference in a conditional position, an `errors.Is(_, X)` /
  `errors.As(_, &X)` call, a handoff call to another `vow:use X`
  function, or a statement-scope `vow:use X` marker inside the
  body.
- Transducer recognition: a function-doc `vow:use X` on a
  function whose signature returns a single bool promotes the
  function to transducer status. A call site that evaluates the
  predicate inside a conditional context (`if` / `switch` cond or
  init) discharges the must-consume obligation for the declared
  subjects; a call elsewhere falls through to the callee-side
  discharge contract without crediting the caller. Multi-subject
  markers (`vow:use ErrFoo, ErrBar`) register the function as a
  transducer for every listed subject so a single call site
  discharges whichever subject the call's argument names.

## `vow:import`

Brings a preset's rule namespace into the current package's scope.
The marker must appear on a package doc comment (the comment group
immediately preceding a `package` clause).

```go
// vow:import result "preset/std/result"
package store
```

The scope is the package, not the file carrying the marker. A
package is conventionally given a single doc comment — `godoclint`
and similar checks enforce it — so one file's marker has to cover
its siblings.

Collecting a package's `vow:import` lines in one file works well —
`doc.go` is where a package doc comment lives anyway, and a reader
then finds the package's aliases in one place rather than grepping
for the binding of each `@alias` they meet.

Layout:

- The first argument is an alias (`result`); annotations in the
  package reference rules as `@result.OkErr[args]`.
- The second argument is the preset path (relative to the project
  or absolute, depending on how the preset is loaded).

Behaviour:

- Multiple `vow:import` lines may coexist, in one file or across
  the package; each contributes a separate alias.
- An alias bound more than once keeps its first binding. Repeating
  an identical binding is quiet; binding one alias to two different
  presets is reported at the losing line.
- The imported preset must declare a `rules:` map; preset files
  that only contribute obligations cannot be used as a rule
  namespace.

A `@alias.Rule` reference the lookup cannot resolve — the alias
never reached the package, or the preset declares no rule under
that name — surfaces a diagnostic at the annotated declaration.
The annotation pipeline drops a line it cannot turn into a
condition, so without the diagnostic a misplaced `vow:import`
would leave the reference reading as a contract while nothing
checked it. The blank line between the marker and the `package`
clause is the common way to lose the alias: it splits the marker
into a comment group the package clause does not own.

## Authoring tip: keep markers off line starts in prose

The analyzer matches markers by trimming a leading `//` from each
comment line and checking whether the trimmed body **starts with**
one of the recognised marker strings. That makes the recognition
cheap and consistent across surfaces, but it also means a doc
comment line whose first non-`//` token happens to be a marker
name is interpreted as an actual declaration on the surrounding
declaration:

```go
// Wrong: the analyzer treats the second line as a declaration of
// the surrounding function as a vow:emit declaration because the
// line begins with vow:emit.
//
// helper documents the
// vow:emit marker's payload format.
func helper() { ... }
```

Two safe options keep the prose readable without misleading the
analyzer:

- Re-flow the sentence so the marker appears mid-line:

  ```go
  // helper documents the payload format of the vow:emit marker.
  ```

- Use indentation or punctuation to break the prefix match:

  ```go
  // helper documents the
  //   vow:emit marker (indented; the analyzer skips).
  ```

The same constraint applies to vow's own source comments — see
the in-repo guides under `internal/analysis/emit_shorthand.go`
for an example of the re-flow pattern.

## Authoring tip: no space after a comma in a slot list

Write `(,,?)`, not `(, , ?)`: the commas are then the count of the
positions left out, where the spaced form leaves a reader deciding
whether the first slot was omitted on purpose or forgotten. The
payload's whitespace is free either way, and the analyzer's own
printout carries the unspaced form.

## `[subject]` scope qualifier

Every function-doc marker — `vow:nil`, `vow:cond`, `vow:use`,
`vow:emit`, and `vow:cond` — accepts an optional `[<subject>]`
qualifier that retargets the declaration at a named signature slot.
The subject names a receiver, a regular parameter, or a named
return of the enclosing function so the same declaration grammar
describes a higher-order contract carried by a callback parameter
or by a returned function value.

```go
// vow:nil[cb] (!)
// vow:use[handler] ErrFoo
// vow:emit[cb] Close
// vow:cond[cb] * -> ErrFoo | nil
```

The resolver verifies the subject against the enclosing signature
at parse time. A subject that does not name any slot surfaces a
diagnostic at the function declaration so the author sees the
typo instead of losing the contract silently. The diagnostic
category matches the marker family: `vow[nil-safety]` for
`vow:nil` and `vow[sentinel-error]` for the rest, with caller-side
statement-scope `vow:emit` reporting under `vow[closable]`.

A subject spelt as `$N` (a 1-based integer prefixed by a dollar
sign) reaches the N-th return value of the enclosing function.
The positional form keeps an unnamed callback reachable when the
function exposes the callback as a return value and lets the
author pick a single slot when the signature carries multiple
returns of the same identifier.

```go
// vow:use[$1] ErrFoo   // discharges through the first return value
func f() (func(error), error) { /* ... */ }
```

The resolver scans the return list one logical slot per declared
name, so `$1` against `func f() (a, b T, c U)` reaches the slot
declared by `a`. A positional index beyond the return list
surfaces the standard subject-unresolved diagnostic; a positional
slot whose declared type is not function-typed surfaces the
callback-type diagnostic.

### Chained `[X][Y]...` scope

`vow:cond`, `vow:use`, `vow:emit`, and `vow:nil` accept a chained
`[X][Y]...` qualifier that steps through nested higher-order
callbacks. The first segment resolves against the enclosing
function's signature; each non-head segment resolves against the
callback signature carried by the previous segment's slot.

```go
// InnerFn is the innermost callback shape.
type InnerFn func(z error) error

// OuterFn carries an InnerFn parameter named inner.
type OuterFn func(inner InnerFn) error

// vow:cond[outer][inner] z == nil <=> $1 == nil
// vow:use[outer][inner] ErrFoo
// vow:emit[outer][inner] Close
// vow:nil[outer][inner] (!)
func walk(outer OuterFn) error { /* ... */ }
```

Body references inside the chained marker resolve against the
innermost callback signature. The bare identifier `z` names a
parameter of `InnerFn`; `$1` names its first return slot. A `$N`
positional reference also reaches a slot in the chain qualifier
itself, so `vow:cond[factory][$1]` lands on `factory`'s first
return value when that return is callback-typed.

A chain segment that does not name any slot at its depth surfaces
a diagnostic that quotes the full chain in the `[X][Y]...`
surface form and names the failing segment by index. An
intermediate segment that resolves to a non-callback slot
surfaces a separate diagnostic because the chain cannot step
inward through a non-function-typed slot. Two- and three-level
chains are the typical depths in practice; the parser does not
bound the chain depth.

The analyzer validates the callback signature scope for the two
markers whose executable semantics target the callback directly.
A `vow:cond[subject]` logical-arrow rule resolves each operand's
reference against the callback's own signature — the callback's
parameters, receiver, and named returns — and surfaces a
diagnostic when the reference does not match a slot. A
`vow:emit[subject]` declaration validates that the subject is
function-typed so the emission has a callable carrier to
propagate through; a non-callback subject surfaces a diagnostic
at the function declaration. The remaining marker families
(`vow:nil`, `vow:use`, statement-scope `vow:emit`) accept the
surface and resolve the subject; the wire that threads
their higher-order contracts through to the validator lands in
the layer.

## See also

- [DSL reference](dsl-reference.md) for the signature payload syntax.
- [Presets](presets.md) for how rule libraries are authored and
  referenced.
- [Architecture](architecture.md) for the classification order that
  decides when a marker discharges a diagnostic.

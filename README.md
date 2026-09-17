# vow — Value Obligation Witness

`vow` is a Go linter that expresses and verifies *value-level
obligations* — properties that must hold for specific values as they
flow through code — through a declarative DSL and reusable presets.
The name reads as `vow` *witnessing* the obligation: every contract
is named, located, and proved (or reported) by the analyzer.

Typical obligations vow expresses:

- **Nil safety** — every parameter, receiver, and return position is statically classified as non-nil, nillable, or platform, so callers cannot pass `nil` into a non-nil slot and bodies cannot overwrite a non-nil binding (`vow:nil`).
- **Relational invariants between values** — a function declares that one value's state determines another's: a parameter requirement pairs with a return requirement, or two expressions chain through an implication or biconditional. The analyzer enforces the invariant at every call site and every matching return (`vow:cond`).
- **Must-consume values** — a designated value must be checked through a recognised consumer (`errors.Is` / `errors.As`, an equality compare, a switch case) before it leaves the function that produced it; propagation across a boundary requires explicit authorisation (`vow:define @Sentinel`, `vow:emit`, `vow:use`).
- **Lifecycle invariants** — a designated resource must reach a discharge — `defer Close`, goroutine hand-off, callback hand-off, storage write, or channel send — before its acquisition scope ends (`vow:define @Closable`, `vow:emit`).

The concept system extends the same enforcement machinery to
project-specific invariants (`vow:define`), and dedicated controls
let a function declare itself as an all-verification barrier or
suppress a single site with a reason (`vow:discharged`,
`vow:suppress`). See the *Marker family* table for placement and
payload details and the *Concept system* section for the
extensibility surface.

Rather than hard-coding each rule, `vow` reads YAML *presets* that
describe the obligation in a uniform DSL and dispatches them to
generic rule engines built on
[`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis).

## Installation

Requires Go 1.27 or later.

```sh
go install github.com/knowledge-work/go-vow/cmd/vow@latest
```

## Quick start

Declare a package-level sentinel and a function that propagates it
under both an emit and a nil-safety contract:

```go
package store

import "errors"

type Record struct{ /* ... */ }

func load(id *string) (*Record, error) { /* ... */ }

// vow:define @Sentinel
var ErrNotFound = errors.New("not found")

// vow:nil (!) !, ?
// vow:emit ErrNotFound
func Lookup(id *string) (*Record, error) {
    if *id == "" {
        return nil, ErrNotFound
    }
    return load(id)
}
```

The `vow:emit ErrNotFound` marker authorises `Lookup` to hand the
named sentinel back; without it, `return nil, ErrNotFound` is
reported as a leak. The check reads references to the sentinel —
every place `ErrNotFound` is itself named. A reference in a
conditional position discharges the obligation, whether the branch
matches it with `errors.Is` / `errors.As`, compares it directly, or
lists it as a `switch` case; a reference anywhere else is reported,
including an `errors.Is` call whose result is returned instead of
branched on. A caller that only holds the `error` value `Lookup`
returned never names the sentinel, so the obligation is not carried
across the call.

The `vow:nil (!) !, ?` signature mirror pins `id` and the first
return as non-nil and the second return as nillable. A caller
passing `nil` to `id` is reported at the call site; a body that
overwrites `id` with `nil` is reported at the assignment.

Run the linter:

```sh
vow ./...
```

## Marker family

| Marker | Purpose |
|--------|---------|
| `vow:nil` | Signature-mirror nil-safety contract. Per-position tokens `!` (non-nil), `?` (nillable), or unmarked (platform) pin the receiver, parameters, and returns inline with the Go signature. The keyword form `nonnil` / `nil` is accepted as input at every slot. |
| `vow:cond` | Relational constraint. The structural form pairs a parameter requirement with a return requirement through `->`; the logical form pairs two expressions through `=>` (implication) or `<=>` (biconditional). The payload supports value-level sum types, the `[subject]` scope qualifier, and lifecycle-pair invariants. |
| `vow:use` | Caller-authored discharge assertion: the marker claims the named subject is consumed at this scope and drops the must-consume diagnostic locally. Placed on a bool-returning function declaration, the same marker also promotes the function to transducer status so callers in conditional context discharge the obligation. |
| `vow:emit` | Declares the callee's output side. The payload has three shapes: name-based, positional (`$N`), and passthrough (`name -> $N`). A statement-scope `vow:emit` additionally discharges a Closable lifecycle hand-off and dispatches to five recognised destinations: goroutine, callback, storage, channel, or wrapped-return. |
| `vow:discharged` | Callee-authored marker on a function declaration. The annotated function counts as an all-verification barrier so callers may treat the call as discharging every obligation the function carries. |
| `vow:suppress` | Suppression marker. The payload must carry a reason string so the suppression survives review. |
| `vow:define` | Concept declaration. `@Sentinel` and `@Closable` are the built-in concepts; user-declared concepts attach behaviour to a `var` or `type` through preset configuration. |
| `vow:import` | Imports a preset rule namespace into the package: `vow:import result "preset/std/result"` makes the namespace available for inline reference (`@result.OkErr[Result, error]`). |

The [Annotations reference](docs/annotations.md) documents each
marker's full payload grammar, placement rules, and the analyzer
behaviour it drives.

## Three-class detection

Every subject reference resolves into one of three classes:

- **Observe** — the reference sits in a conditional position and is
  consumed by a recognised discharger there (`errors.Is` /
  `errors.As`, an equality compare against the sentinel, or a
  `switch` case branch). The same discharger outside a conditional
  — its result returned or bound to a variable — does not observe.
- **Chain** — the reference is propagated through a recognised
  authorisation surface (the enclosing signature lists the subject
  through `vow:emit`, or the call expression resolves to a
  `vow:emit`-declared callee).
- **Leak** — every other reference. The analyzer reports the leak
  at the offending expression so the author either observes the
  subject, authorises the chain, or suppresses the diagnostic with
  a reason.

## Concept system

`vow:define @<Concept>` declares a tracked subject. The analyzer
ships two built-in concepts:

- `@Sentinel` — a package-level `var` (typically `errors.New`)
  becomes a must-consume subject. The `sentinel-error` preset wires
  the observation surfaces.
- `@Closable` — a `type` (or a built-in standard-library type
  listed under the `closable` preset) becomes a lifecycle subject.
  Every acquisition must reach a discharge before its scope
  closes.

A `vow:define @<Concept>` declaration whose concept has no
behaviour registered surfaces an info-level diagnostic so the
author learns that preset configuration is pending. Adopters add
new concepts by extending a preset YAML and registering the
obligation pattern the concept follows.

## Presets

| Preset | Purpose |
|--------|---------|
| `preset/sentinel-error.yaml` | Activates the must-consume obligation on every `vow:define @Sentinel` declaration. |
| `preset/closable.yaml` | Activates the lifecycle-discharge obligation on every `vow:define @Closable` type and on a curated list of built-in Closable Go types (`*os.File`, `*net.TCPConn`, `*bufio.Writer`, `io.Closer`). |
| `preset/std/result.yaml` | A reusable rule library for the `(Result, error)` return shape. Imported into a package with `vow:import result "preset/std/result"`. |
| `example/werror.yaml` | A sample preset showing the wrapped-return shape `vow:emit` uses to recognise a wrap library's pass-through call as a chain-authorisation source. Adopters copy and adapt the shape for their own wrap helpers. |

See the [Presets guide](docs/presets.md) for the YAML schema and
authoring walkthrough.

## Documentation

- [Getting started](docs/getting-started.md) — install, annotate,
  run.
- [DSL reference](docs/dsl-reference.md) — terms, qualifiers, sums,
  tuples, logical-arrow rules, signature-mirror grammar.
- [Annotations](docs/annotations.md) — every `vow:` marker and
  where it applies.
- [Presets](docs/presets.md) — the bundled presets, the
  `std/result` rule library, and how to author your own.
- [CLI](docs/cli.md) — `vow` command, flags, exit codes.
- [Architecture](docs/architecture.md) — how detection, matching,
  and cross-package facts are implemented internally.

## Reading the fixtures

The analyzer's test fixtures under `internal/analysis/testdata/src/`
are also worked examples: each is a compiling Go package carrying
real annotations, and every `// want` comment states the exact
diagnostic the annotated code produces. To see how a marker behaves
in a shape the prose does not cover, read the fixture rather than
guess — `go test ./...` fails as soon as a `// want` stops matching
what the analyzer reports, so the examples cannot drift from the
implementation.

One package per marker family and scenario, named after the two:

| Prefix | Covers |
|--------|--------|
| `nilDecl*` | `vow:nil` — signature mirror, field declarations, receivers, flow, cross-package facts. |
| `cond*` | `vow:cond` — grammar, logical arrows, tag exhaustiveness, caller-side narrowing, lifecycle pairs. |
| `sentinels`, `vowUse`, `vowEmit`, `useHint`, `discharged`, `suppress`, `concept` | `@Sentinel` must-consume, and the markers that discharge or suppress it. |
| `lifecycle_*`, `closable*` | `@Closable` — the five discharge destinations. |
| `higher_order*`, `narrowScope*`, `qualified_*`, `importPkgScope` | Function values, scope qualifiers, and preset imports. |

A fixture whose name ends in `XPkgCallee` / `XPkgCaller` is one half
of a cross-package pair; read both to see which side exports the
fact and which side consumes it.

## Layout

| Path | Purpose |
|------|---------|
| `cmd/vow/` | CLI entry point. |
| `internal/analysis/` | Analyzer implementation built on `go/analysis`. |
| `internal/dsl/` | YAML preset parser and annotation grammar. |
| `preset/` | Bundled presets. |
| `preset/std/` | Reusable rule libraries. |
| `example/` | Sample presets demonstrating extension patterns. |
| `docs/` | User-facing documentation. |
| `internal/analysis/testdata/src/` | Fixtures consumed by `analysistest`, one package per marker family and scenario. |

## Building

```sh
go build ./...
go test ./...
go vet ./...
```

## License

MIT. See [LICENSE](./LICENSE).

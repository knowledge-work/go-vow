# Getting started

This guide walks through installing `vow`, declaring a sentinel
error, running the linter, and resolving each class of diagnostic
the analyzer can emit.

## Install

Requires Go 1.27 or later.

```sh
go install github.com/knowledge-work/go-vow/cmd/vow@latest
```

The command installs a binary called `vow` in `$GOBIN` (default
`$GOPATH/bin`). The linter is a `go/analysis` single-checker, so it
accepts the same package patterns as `go vet`.

## A complete example

```go
package store

import "errors"

// vow:define @Sentinel
var ErrNotFound = errors.New("not found")

// Lookup returns ErrNotFound when id is empty. The annotation
// authorizes that propagation; without it, vow would report a
// leak at the return.
//
// vow:cond * -> ErrNotFound | nil
func Lookup(id string) (string, error) {
    if id == "" {
        return "", ErrNotFound
    }
    return id, nil
}

// HandleLookup observes the sentinel via errors.Is — discharging the
// must-consume obligation in place. No annotation needed.
func HandleLookup(id string) (string, bool) {
    value, err := Lookup(id)
    if errors.Is(err, ErrNotFound) {
        return "", false
    }
    return value, true
}
```

Run the linter:

```sh
vow ./...
```

With the snippet above the linter is silent. Removing the
`vow:cond` annotation reports a leak at `return "", ErrNotFound`,
because the function then has neither an observation nor an
authorized propagation.

## Three diagnostic classes

Every subject reference (here, every mention of `ErrNotFound`) is
classified as one of:

- **observe** — discharged via `errors.Is`, `errors.As`, equality
  comparison, or a `switch` case inside a conditional. Silent.
- **chain** — sits inside a `return` statement whose enclosing
  function authorizes propagation through a `vow:cond` signature
  that lists the subject. Silent.
- **leak** — everything else. Reported.

The classifier prefers observation over chain: if an `errors.Is`
discharge applies, the chain check is not consulted. See
[Architecture](architecture.md) for the full classification order.

## Annotation-driven obligations

The `sentinel-error` preset's contract is conveyed entirely through
annotations:

- `// vow:define @Sentinel` on a package-level `var` marks the value
  as a tracked subject of the Sentinel concept.
- `// vow:cond <param-req> -> <return-req>` on a `func`
  declares a case-style contract: the parameter requirement binds
  to the function's arguments by signature order, the return
  requirement describes the return positions. `vow:cond X` is
  the sugar for the trivial wildcard parameter requirement
  (`vow:cond * -> X`). Both forms authorise chain
  propagation when the return requirement lists the concrete
  sentinel. The signature uses the DSL described in
  [DSL reference](dsl-reference.md).
- `// vow:use X` on a `bool`-returning `func` also promotes it
  to a transducer over subject X: a call inside an `if` /
  `switch` condition discharges the must-consume obligation for
  X when X is passed as an argument, just like the preset's
  built-in `errors.Is` / `errors.As`. Multiple subjects ride on
  one marker as a comma-separated payload (`vow:use A, B`).
- `// vow:import <alias> "<preset path>"` on a package doc brings a
  rule namespace into scope so annotations can write
  `@<alias>.<rule>[args]`.

See [Annotations](annotations.md) for the full marker list with
placement rules.

## Configuring observers

The preset YAML declares which functions count as observers:

```yaml
obligations:
  - type: must-consume
    detail:
      consumers:
        - errors.Is
        - errors.As
```

Both functions are syntactic — `vow` matches `pkg.Func` calls
directly, so aliased imports are not resolved. See
[Presets](presets.md) for how to add custom observers.

## Next steps

- Read the [DSL reference](dsl-reference.md) to write richer
  `vow:cond` signatures.
- Read [Presets](presets.md) to author or extend a preset.
- Read [Architecture](architecture.md) for the data-flow guarantees
  the analyzer makes (and the ones it does not).

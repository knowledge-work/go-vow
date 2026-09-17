# Presets

A **preset** is a YAML file that declares one or more obligations
on a set of subjects, or one or more reusable rule templates, or
both. The `vow` analyzer dispatches each obligation to the
matching rule engine.

## File structure

```yaml
name: <preset name>
version: "<semver>"
description: |
  Human-readable description.
subjects:
  - match: "annotation:vow:<marker>"
obligations:
  - type: <obligation type>
    detail:
      <type-specific key>: <value>
severity: warning
rules:
  <RuleName>:
    params: [<P1>, <P2>, ...]
    body: "<DSL fragment>"
```

A preset must contribute something. The validator accepts three
combinations:

- **Obligation contract.** Both `subjects:` and `obligations:` are
  populated.
- **Rule-only.** No `subjects:` or `obligations:`, but `rules:` is
  populated. Used as an import target via `vow:import`.
- **Mixed.** Subjects + obligations + rules.

Empty `subjects:` paired with non-empty `obligations:` (or vice
versa) is a validation error.

## `sentinel-error`

The default preset, loaded by the bundled analyzer. It marks
package-level sentinel errors as subjects and enforces the
`must-consume` obligation against them.

```yaml
name: sentinel-error
version: "0.1"
subjects:
  - match: "annotation:vow:define @Sentinel"
obligations:
  - type: must-consume
    detail:
      consumers:
        - errors.Is
        - errors.As
severity: warning
```

Key points:

- The subject pattern `annotation:vow:define @Sentinel` picks out every
  package-level `var` whose doc carries the `// vow:define @Sentinel`
  marker.
- The `must-consume` obligation classifies every subject reference
  via the AST stack and reports leaks. Observers are matched
  syntactically as `pkg.Func` calls inside a conditional.
- The list of consumers is configurable. Adding `myerrors.Is` would
  accept `myerrors.Is(err, ErrFoo)` as an observation. Aliased
  imports are not resolved, so the consumer must spell the same
  package identifier as the call site.

## Sample: passthrough-emit preset (`example/werror.yaml`)

```yaml
name: werror
subjects:
  - match: "annotation:vow:define @Sentinel"
obligations:
  - type: passthrough-emit
    detail:
      functions:
        - pattern: "example.com/wraplib.Wrap"
          arg-position: 0
          return-slot: 1
        - pattern: "example.com/wraplib.Errorf"
          arg-position: 0
          return-slot: 1
```

- The sample preset ships under `example/` rather than the
  built-in set so adopters opt into it deliberately. Copy the
  file, change the function patterns to match the wrap library
  the codebase uses, and register the preset alongside the
  built-in set when constructing the analyzer.
- The subject scheme reuses the `vow:define @Sentinel`
  registration so the obligation applies to the same sentinel
  set the `sentinel-error` preset tracks.
- The `passthrough-emit` obligation registers the wrap signatures
  the analyzer recognises as the fifth `vow:emit` caller-side
  destination. A statement-scope `// vow:emit X` marker anchored
  to a return whose value flows through one of the registered
  calls resolves as the wrapped-return destination instead of
  reaching the missing-destination diagnostic.
- The `functions` list takes a fully-qualified function name
  (`<import-path>.Name`), the 0-based argument position the wrap
  reads its subject from, and the 1-based return slot the wrapped
  value lands in. The sample uses the RFC 2606 reserved
  `example.com/wraplib` path so the file reads as a template that
  names no specific third-party library.

## `std/result` rule library

A rule-only preset that exposes the most common multi-return shapes
and one explicit chain-authorisation escape hatch:

```yaml
name: std/result
rules:
  OkErr:
    params: [T, E]
    body: "(nonzero T, nil) | (nil, nonzero E)"
  Either:
    params: [L, R]
    body: "(nonzero L | nonzero R)"
```

Usage:

```go
// vow:import result "preset/std/result"
package store

// vow:cond @result.OkErr[Result, error]
func Lookup() (Result, error) { ... }
```

`@result.OkErr[Result, error]` expands to
`(nonzero Result, nil) | (nil, nonzero error)` before being fed back through
the parser.

The arguments name types, so each position matches on the type
axes: the expression's own type, the same type rendered relative
to the package under analysis or qualified by package name, and
the declared type of the return position. That last axis is what
admits an error helper whose result type is concrete while the
position declares `error`.

## Authoring a custom preset

1. **Pick a subject scheme.** The only scheme currently recognised
   is `annotation:` — the argument is the doc-comment marker the
   variable must carry.

2. **Pick an obligation type.** The bundled analyzer recognises
   `must-consume`. The `detail` bag is type-specific:

   ```yaml
   obligations:
     - type: must-consume
       detail:
         consumers:
           - mylib.IsNotFound
           - errors.Is
   ```

3. **(Optional) Author rule templates.** Each rule declares
   `params` and a `body`. Parameters appear as bare identifiers in
   the body and are textually substituted at expansion time. Avoid
   shadowing the parameter name with another DSL keyword.

4. **Load the preset.** Built-in presets are embedded in
   `internal/analysis/builtin.go`. Custom presets can be added by
   constructing an `analysis.Analyzer` with the desired preset list
   via `analysis.New(presets)`.

## Validation gates

The preset validator enforces a small invariant set:

- Preset `name` must be non-empty.
- Subjects with empty `match` are rejected.
- Obligations with empty `type` are rejected.
- Rule names must be non-empty.
- Rule bodies must be non-empty.
- Rule parameter lists may not contain duplicates.

Errors arrive as `dsl.Load` / `dsl.Parse` return values; the
analyzer surfaces them on startup so authoring mistakes fail fast
instead of producing partial enforcement.

## See also

- [DSL reference](dsl-reference.md) for the syntax used in rule
  bodies and `vow:cond` signatures.
- [Annotations](annotations.md) for the markers presets reference.
- [Architecture](architecture.md) for how obligations dispatch to
  rule engines.

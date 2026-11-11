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
- The list of consumers is configurable: a copy of this preset
  loaded under the same name, with `myerrors.Is` added to
  `consumers`, accepts `myerrors.Is(err, ErrFoo)` as an observation
  (see [Loading a preset](#loading-a-preset)). Aliased imports are
  not resolved, so the consumer must spell the same package
  identifier as the call site.

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
  the codebase uses, and list the copy under `presets` in
  `vow.yaml` (see [Loading a preset](#loading-a-preset)).
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

1. **Pick the concept.** The subject is
   `annotation:vow:define @<Concept>`, which selects every
   package-level `var` declared with `// vow:define @<Concept>`.
   Use a new name for your own concept, or `@Sentinel` for the vars
   the `sentinel-error` preset tracks (for `must-consume`, replace
   that preset instead).

2. **Pick an obligation type.** `must-consume` lists the functions
   that count as an observation, as below; `passthrough-emit` lists
   wrap signatures, as in the sample above.

   ```yaml
   obligations:
     - type: must-consume
       detail:
         consumers:
           - mylib.IsNotFound
           - errors.Is
   ```

3. **Load the preset** as the next section describes.

A loaded preset may not declare `rules:` or configure the Closable
check; see [Validation gates](#validation-gates).

## Loading a preset

List the preset files under `presets` in `vow.yaml`:

```yaml
presets:
  - tools/vow/notfound.yaml
  - tools/vow/werror.yaml
```

- A relative path resolves against the directory of the `vow.yaml`
  that lists it, or against the working directory under
  `--config-yaml`.
- A preset whose `name` is `sentinel-error` replaces the built-in
  preset of that name; any other name adds a preset. To change what
  counts as observing a `@Sentinel` var, load a copy of
  `preset/sentinel-error.yaml` with the consumers added. An added
  preset would run beside the built-in one, which would go on
  reporting the new consumer's checks as leaks, and would report
  every other leak a second time.
- The key follows the same scoping as the rest of `vow.yaml`: the
  nearest file applies to a package, so a `vow.yaml` lower in the
  tree that omits `presets` loads none of the presets above it.
- `--vet-mode` passes the content of each file to `go vet`, so an
  edit to a preset invalidates the cached result.

## Validation gates

The preset validator enforces a small invariant set:

- Preset `name` must be non-empty.
- Unknown keys are rejected.
- `severity`, when present, must be `warning`, the only severity vow
  reports.
- Subjects must use the `annotation:` scheme.
- Obligations must be of type `must-consume` or `passthrough-emit`
  (`must-close` belongs to the built-in `closable` preset), with a
  `detail` holding the keys that type reads: `consumers` for
  `must-consume`, and `functions` entries with `pattern`,
  `arg-position`, and `return-slot` for `passthrough-emit`. A
  function name may not contain spaces.
- Rule names must be non-empty.
- Rule bodies must be non-empty.
- Rule parameter lists may not contain duplicates.

A loaded preset must in addition:

- select its subjects with `vow:define @<Concept>`;
- declare no `rules:`, since `vow:import` reads rule templates only
  from the libraries built into `vow`, such as `preset/std/result`;
- leave the Closable check alone: no `must-close` obligation, no
  `built_in_closables`, and no preset named `closable`. The check is
  built in, and `closable.built_in_closables` in `vow.yaml` sets its
  types;
- have a name no other file under the same `presets` key declares.

A preset file that is missing or fails a gate stops a run that names
it through `--config-file` or `--config-yaml` with exit code `2`.
Named by a `vow.yaml` that a package discovers, it is reported as a
`vow[config]` diagnostic, and that package runs with the built-in
defaults, as it does when its `vow.yaml` is malformed.

## See also

- [DSL reference](dsl-reference.md) for the syntax used in rule
  bodies and `vow:cond` signatures.
- [Annotations](annotations.md) for the markers presets reference.
- [Architecture](architecture.md) for how obligations dispatch to
  rule engines.

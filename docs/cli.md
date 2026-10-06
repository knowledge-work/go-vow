# CLI

Run from the command line, `vow` takes only the flags on this page,
and any other flag stops the run with exit code 2. It drives the
analyzer itself rather than through `singlechecker.Main`, so
`singlechecker` flags such as `-c` and `-fix` are not among them. When
`go vet` runs `vow` as a vettool, `vow` takes the flags `-flags`
reports instead.

## Usage

```sh
vow [flags] [packages...]
```

Package patterns follow the standard Go tooling conventions —
relative paths, `./...`, import paths, and explicit file lists are
all accepted. An argument `--` ends vow's flags and every argument
after it is a package pattern; use it to name packages after
`--changed-files`, which otherwise reads every following argument up
to the next flag as a changed file.

```sh
vow ./...                 # every package in the module
vow ./internal/...        # one subtree
vow github.com/foo/bar    # one external import path (must be on GOPATH)
vow main.go util.go       # specific files in the current package
```

## Common flags

These flags share their names with other `go/analysis` tools.

| Flag | Purpose |
|------|---------|
| `-V=full` | Print a build ID (a hash of the `vow` binary) and exit. `go vet` asks for it in this form to tell when the tool has changed; a bare `-V` is rejected. For the vow release, run `go version -m $(which vow)`. |
| `-flags` | Print as JSON the flags `vow` accepts as a `go vet` vettool, which differ from the flags on this page. |
| `-json` | Print the report, analyzer failures included, on stdout as one JSON document instead of as text on stderr. See [Output format](#output-format). |
| `-test` | Include test files in the analysis. Test files are excluded by default; the flag takes no value. |

## Driver flags

These flags control which packages are loaded, how they are
analyzed, and where the configuration comes from.

| Flag | Purpose |
|------|---------|
| `--changed-files <file>...` | Restrict the run to the listed paths. An empty expansion — `git diff` returning nothing, for instance — falls back to the package-level run. |
| `--with-callers` | Extend the run with the caller-side checks. Requires `--changed-files`, and replaces any package arguments with the changed packages plus their direct callers. Using it alone exits with code 2. |
| `--config-file <path>` | Read configuration from one `vow.yaml` file for the whole run, instead of discovering one per directory. |
| `--config-yaml <content>` | Read the same configuration from an inline string. Passing both config flags is an error. |
| `--vet-mode` | Run the analysis through `go vet` instead of loading the packages in one process, so each package's facts come from the Go build cache. See [Vet mode](#vet-mode). |

### Vet mode

`--vet-mode` drives `go vet` with vow as the vettool. The go command then
analyzes one package at a time, stores each package's facts in the build
cache, and reuses them while the package's sources and dependencies are
unchanged — where the default driver loads the dependency closure and
analyzes it again on every run. Both paths run the same analyzer over
the same packages and report the same diagnostics, as `position:
message` lines on stderr or, under `-json`, as a JSON document on
stdout; what changes is how much is loaded to produce the report.

Two consequences are worth knowing before turning it on:

- **Configuration must be passed on the command line.** The per-directory
  `vow.yaml` walk does not run in this mode. The go command keys a cached
  result on the flags the vettool received, and a file the tool opens
  itself is not part of that key, so a discovered config would leave an
  edited policy answering with the previous run's verdict. Use
  `--config-file` or `--config-yaml`; vow reads the file and passes its
  content, so an edit invalidates the cached result.
- **The first run pays for the dependencies.** The go command compiles
  export data for the closure on a cold cache. A CI job that runs this
  mode wants a Go build cache in front of it.
- **Only the run's own packages report.** `go vet` reports for every
  package it analyzes, including the dependencies it analyzes so their
  facts reach the packages under test. vow keeps the entries of the
  packages the run targets — under `--with-callers`, the changed
  packages and their callers — and drops the rest.
- **`--changed-files` narrowing is applied by vow, not by the analysis.**
  The changed-file set cannot travel to the analysis without becoming
  part of what the go command caches it on, which would leave every
  package re-analyzed whenever the set changes. Each diagnostic instead
  carries the inputs the narrowing needs — whether it sits inside a call
  and where that call's callee is declared — and vow applies the same
  rule the default path applies inside the analyzer.

## Configuration

Behaviour that is neither a preset nor an annotation is set in a
`vow.yaml` file. Without a config flag, each package uses the
nearest such file in an ancestor directory, which lets a repository
turn a rule on one subtree at a time. A section a file omits keeps
the built-in default for that section.

| Key | Purpose |
|-----|---------|
| `closable.built_in_closables` | Replace the standard-library and convention types treated as `@Closable` (`*os.File`, `*net.TCPConn`, `*bufio.Writer`, `io.Closer`). An empty list replaces it with nothing; types carrying `vow:define @Closable` participate either way. See [Annotations](annotations.md). |
| `nil_decl.require_declarations` | Turn on the declaration-completeness rule, which is off by default. See [Annotations](annotations.md). |
| `caller_resolver.exclude_package_suffixes` | Drop packages whose import path ends in one of these suffixes from the scope `--with-callers` computes. Empty by default. |
| `analysis_scope.first_party_prefixes` | Parse and analyze only the target packages and the dependencies whose import path falls under one of these prefixes; every other dependency contributes type information from export data and is neither parsed nor analyzed. Empty (the default) keeps the whole-closure load. |

The resolver runs once per invocation rather than once per package,
so `caller_resolver` — and `analysis_scope`, whose load likewise
happens once per invocation — is read from `--config-file` /
`--config-yaml` only; a per-directory `vow.yaml` does not reach
them.

### First-party scope

vow facts originate exclusively from `vow:` markers, so a
dependency that carries no markers can contribute nothing to a run
beyond its type information — parsing and analyzing it is pure
overhead. `analysis_scope.first_party_prefixes` lets the adopter
assert where markers may appear:

```yaml
analysis_scope:
  first_party_prefixes:
    - github.com/your-org/your-repo
```

A prefix matches on import-path boundaries (`example.com/foo`
covers `example.com/foo` and `example.com/foo/...`, never
`example.com/foobar`). List every module that may carry markers —
including sibling modules reached through `replace` directives — or
their facts silently drop out of the run.

The scoped load reads dependency types from export data, which the
`go` command compiles on demand: the first run against a cold build
cache pays that compilation, and subsequent runs reuse it. On a
large module the mode cuts peak memory to roughly a third and,
warm, the wall time to roughly half.

## Output format

Default (text) mode prints one line per diagnostic:

```
path/to/file.go:LINE:COL: vow[<preset>]: <message>
```

Example:

```
store.go:14:9: vow[sentinel-error]: sentinel error ErrNotFound leaked: needs observation or explicit propagation
```

With `-json`, `vow` prints one JSON document on stdout in the shape
`go vet -json` uses: an object keyed by package ID and then by
analyzer name, whose value is either the list of diagnostics or an
object with an `error` field when the analyzer failed on that
package. A package with nothing to report is left out, so a clean run
prints `{}`.

The exit code is the one the run would have without `-json`. Read it
before the document: under `--vet-mode`, a failure the go command
reports itself, such as a package that does not type-check, goes to
stderr rather than into the document, which can then be `{}`. When
`vow` stops on an error before analyzing anything (a bad flag, for
instance), or cannot run the analysis or, under `--vet-mode`, read the
report `go vet` returns, it prints no document and says why on stderr.
A run whose `--changed-files` resolve to no lintable package is not
such an error: it prints `{}` and exits 0.

```json
{
	"example.com/store": {
		"vow": [
			{
				"category": "must-consume:ErrNotFound",
				"posn": "/repo/store/store.go:14:9",
				"end": "/repo/store/store.go:14:9",
				"message": "vow[sentinel-error]: sentinel error ErrNotFound leaked: needs observation or explicit propagation"
			}
		]
	}
}
```

The keys are package IDs as each path's loader names them. Under
`-test`, the default path analyzes a package that has test files both
on its own and compiled together with them, keying the second
`example.com/store [example.com/store.test]`, so a diagnostic in a
non-test file is listed under both keys. `--vet-mode` analyzes only the
package compiled with its test files, keyed `example.com/store`, and
lists each diagnostic once.

## Exit codes

The framework maps the diagnostic state to a process exit code:

| Code | Meaning |
|------|---------|
| `0`  | No diagnostics fired. |
| `1`  | One or more diagnostics fired. |
| `3`  | The analyzer failed to run (e.g. a preset failed to load). |

Build failures and parse errors in the analyzed packages propagate
through the framework and produce a non-zero exit; the precise code
depends on which stage failed.

## Integration

- **`go vet`.** `vow` can be invoked via `go vet -vettool=$(which vow)`
  inside a project, which gives access to the analyzer through the
  same package loading pipeline `go vet` uses.
- **CI.** Run `vow ./...` after `go build` so the analyzer sees a
  green-compiling tree. Diagnostics never block the build (they only
  set the exit code), so pair the invocation with the upstream
  tool's failure semantics.
- **Custom presets.** The default `vow` binary embeds the
  `sentinel-error` preset. To enforce custom obligations, build a
  binary that constructs `analysis.New(presets)` over your own
  preset list and wraps it with `singlechecker.Main`. That binary
  takes the `singlechecker` flags, so the driver flags on this page,
  such as `--changed-files` and `--config-file`, are not available.

## See also

- [Getting started](getting-started.md) for the typical install /
  annotate / run loop.
- [Presets](presets.md) for authoring custom presets that the CLI
  can enforce when statically embedded.

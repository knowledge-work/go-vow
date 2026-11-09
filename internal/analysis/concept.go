package analysis

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/knowledge-work/go-vow/internal/dsl"
	"github.com/knowledge-work/go-vow/internal/seq"
)

// conceptDefineMarker is the doc-comment marker that declares a
// variable as a member of a named concept set. The marker payload
// is a single rule-ref-shaped concept name (`@Sentinel`,
// `@Resource`, ...) whose semantics are configured by the preset
// registry. Authors declare every tracked subject through this one
// marker so the analyzer's discovery walk has a single surface to
// scan regardless of which concept the subject belongs to.
const conceptDefineMarker = "vow:define"

// builtinConceptClosable is the concept whose lifecycle pass is wired
// into the analyzer rather than dispatched from a preset, so it has
// behaviour whichever presets are loaded.
const builtinConceptClosable = "Closable"

// reportConceptDefineDiagnostics walks every package-level var
// declaration and reports an info-level diagnostic for each
// `vow:define @<concept>` whose concept has no behaviour under
// presets: neither Closable nor selected by a preset that dispatches
// must-consume. Such a concept is accepted so authors can declare it
// ahead of the preset that gives it behaviour.
//
// The diagnostic anchors at the GenDecl so a `var ( ... )` block
// shared by multiple specs emits one diagnostic per declared
// concept rather than one per spec; it is intentionally worded as
// a pending-configuration note rather than a hard error because
// the declaration is structurally valid, only the behaviour is
// awaiting registration.
func reportConceptDefineDiagnostics(pass *analysis.Pass, presets []*dsl.Preset) {
	registered := registeredConcepts(presets)
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	filter := []ast.Node{(*ast.GenDecl)(nil)}
	insp.Preorder(filter, func(n ast.Node) {
		gd := n.(*ast.GenDecl)
		if gd.Tok != token.VAR && gd.Tok != token.TYPE {
			return
		}
		// Walk the GenDecl's group doc once so a `var ( ... )`
		// or `type ( ... )` block with multiple specs does not
		// re-fire the same diagnostic per spec; then walk each
		// spec's own doc so single-spec declarations with their
		// own comment are covered too. A short visited set keeps
		// the two passes from double-reporting a concept that
		// lives on a doc shared by both shapes.
		reported := map[string]struct{}{}
		emitFromDoc(pass, gd, gd.Doc, registered, reported)
		for _, spec := range gd.Specs {
			emitFromDoc(pass, gd, specDoc(spec), registered, reported)
		}
	})
}

// specDoc returns the doc-comment group attached to a spec inside a
// GenDecl, or nil when the spec carries no doc. Both ValueSpec
// (var/const) and TypeSpec (type) variants are handled so the
// concept walker uniformly inspects per-spec docs without branching
// on the GenDecl token at every call site.
func specDoc(spec ast.Spec) *ast.CommentGroup {
	switch s := spec.(type) {
	case *ast.ValueSpec:
		return s.Doc
	case *ast.TypeSpec:
		return s.Doc
	}
	return nil
}

// registeredConcepts returns the concepts that have behaviour under
// presets: Closable, and each concept a preset dispatching
// must-consume selects through `vow:define @<Concept>`.
func registeredConcepts(presets []*dsl.Preset) map[string]struct{} {
	registered := seq.ChainOf(presets...).
		Filter(func(p *dsl.Preset) bool {
			return seq.ChainOf(p.Obligations...).Any(func(o dsl.Obligation) bool { return o.Type == "must-consume" })
		}).
		FlatMap(func(p *dsl.Preset) seq.Chain[dsl.Subject] { return seq.ChainOf(p.Subjects...) }).
		FilterMap(dsl.Subject.Concept).
		ToSet(func(concept string) string { return concept })
	registered[builtinConceptClosable] = struct{}{}
	return registered
}

// emitFromDoc scans doc for `vow:define` lines and emits an info-
// level diagnostic anchored at gd's position for every concept it
// finds outside registered. The reported set carries the concepts
// already surfaced for this GenDecl so the helper is safe to invoke
// once per doc-comment source (the group doc and each spec doc).
func emitFromDoc(pass *analysis.Pass, gd *ast.GenDecl, doc *ast.CommentGroup, registered, reported map[string]struct{}) {
	if doc == nil {
		return
	}
	for _, line := range strings.Split(doc.Text(), "\n") {
		concept, ok := parseConceptDefineLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		if _, ok := registered[concept]; ok {
			continue
		}
		if _, seen := reported[concept]; seen {
			continue
		}
		reported[concept] = struct{}{}
		vowReportf(pass, gd.Pos(), "vow[concept]: vow:define @%s: concept declared but no behavior registered (preset configuration pending)", concept)
	}
}

// parseConceptDefineLine inspects a trimmed comment-body line and
// reports the concept name it declares, when present. A well-formed
// declaration carries exactly `@<Name>` as the payload (no further
// trailing text); the analyzer rejects payloads that lack the `@`
// prefix or carry trailing tokens by returning ok=false so future
// `vow:define <Concept> <option>=...` syntax can be added without
// retrofitting this matcher.
func parseConceptDefineLine(line string) (string, bool) {
	rest, ok := stripMarkerPrefix(line, conceptDefineMarker)
	if !ok {
		return "", false
	}
	payload := strings.TrimSpace(rest)
	if !strings.HasPrefix(payload, "@") {
		return "", false
	}
	name := strings.TrimPrefix(payload, "@")
	if name == "" {
		return "", false
	}
	// Reject trailing tokens — the current surface declares one concept
	// per line.
	if idx := strings.IndexAny(name, " \t"); idx >= 0 {
		return "", false
	}
	return name, true
}

package analysis

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// conceptDefineMarker is the doc-comment marker that declares a
// variable as a member of a named concept set. The marker payload
// is a single rule-ref-shaped concept name (`@Sentinel`,
// `@Resource`, ...) whose semantics are configured by the preset
// registry. Authors declare every tracked subject through this one
// marker so the analyzer's discovery walk has a single surface to
// scan regardless of which concept the subject belongs to.
const conceptDefineMarker = "vow:define"

// Built-in concepts whose behaviour is wired into the analyzer.
// Other concepts that authors may declare are forward-compat
// surface: the parser recognises them and records the declaration
// so user code can already be written against future preset
// configurations, but the analyzer emits an info-level diagnostic
// stating that no behaviour is registered.
const (
	builtinConceptSentinel = "Sentinel"
	builtinConceptClosable = "Closable"
)

// reportConceptDefineDiagnostics walks every package-level var
// declaration and reports an info-level diagnostic for each
// `vow:define @<concept>` whose concept is not currently wired
// into the analyzer. Sentinel and Closable are the built-in
// concepts; other concepts are accepted as a forward-compat
// surface so authors can declare them and the analyzer accepts
// the declaration without enforcing behaviour.
//
// The diagnostic anchors at the GenDecl so a `var ( ... )` block
// shared by multiple specs emits one diagnostic per declared
// concept rather than one per spec; it is intentionally worded as
// a pending-configuration note rather than a hard error because
// the declaration is structurally valid, only the behaviour is
// awaiting registration.
func reportConceptDefineDiagnostics(pass *analysis.Pass) {
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
		emitFromDoc(pass, gd, gd.Doc, reported)
		for _, spec := range gd.Specs {
			emitFromDoc(pass, gd, specDoc(spec), reported)
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

// emitFromDoc scans doc for `vow:define` lines and emits an info-
// level diagnostic anchored at gd's position for every non-builtin
// concept it finds. The reported set carries the concepts already
// surfaced for this GenDecl so the helper is safe to invoke once
// per doc-comment source (the group doc and each spec doc).
func emitFromDoc(pass *analysis.Pass, gd *ast.GenDecl, doc *ast.CommentGroup, reported map[string]struct{}) {
	if doc == nil {
		return
	}
	for _, line := range strings.Split(doc.Text(), "\n") {
		concept, ok := parseConceptDefineLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		if concept == builtinConceptSentinel || concept == builtinConceptClosable {
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
	if !strings.HasPrefix(line, conceptDefineMarker+" ") {
		return "", false
	}
	payload := strings.TrimSpace(line[len(conceptDefineMarker)+1:])
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

package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateNilDeclCompleteness reports every function, method, and
// interface method whose signature carries a nillable position that
// no `vow:nil` nullness decl covers. The check answers "is the
// contract at this position written down?", which the arity gate
// deliberately does not: an author may spell `vow:nil ()` on a
// one-parameter signature, and the empty-expression exception admits
// that as a well-formed marker carrying no decl. So the walk reads
// the Go signature directly rather than routing through the arity
// rule.
//
// The check is off unless the scope opts in through vow.yaml's
// `nil_decl.require_declarations`, because a codebase that adopts
// vow mid-life carries thousands of undeclared positions and a
// default-on rule would bury its nil-safety findings.
//
// Diagnostics anchor at the declaration and name every uncovered
// position in one message, matching the way the arity diagnostics
// report against the signature as a whole rather than per slot.
func validateNilDeclCompleteness(pass *analysis.Pass, state *passState) {
	if !state.nilDeclCompletenessRequired(pass) {
		return
	}
	decls := state.nilDeclSet(pass)
	for _, file := range pass.Files {
		if fileIsGenerated(file) {
			continue
		}
		for _, decl := range file.Decls {
			switch node := decl.(type) {
			case *ast.FuncDecl:
				checkNilDeclCompleteness(pass, decls, pass.TypesInfo.Defs[node.Name], node.Doc, node.Pos())
			case *ast.GenDecl:
				checkInterfaceNilDeclCompleteness(pass, decls, node)
			}
		}
	}
}

// nilDeclCompletenessRequired reports whether this package's scope
// opted into the completeness check. The driver-supplied override wins
// so `--config-file` / `--config-yaml` can turn the rule on for a
// whole run; otherwise the nearest-ancestor vow.yaml decides, which is
// what lets a repository enable the rule one directory at a time. A
// missing config, an absent key, or a discovery error all leave the
// check off. Resolved once per pass and cached.
func (s *passState) nilDeclCompletenessRequired(pass *analysis.Pass) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nilDeclRequiredBuilt {
		return s.nilDeclRequired
	}
	s.nilDeclRequired = s.resolveNilDeclRequirement(pass)
	s.nilDeclRequiredBuilt = true
	return s.nilDeclRequired
}

// resolveNilDeclRequirement reads the opt-in flag from whichever
// configuration source applies to pass. Callers hold s.mu.
func (s *passState) resolveNilDeclRequirement(pass *analysis.Pass) bool {
	if s.configOverride != nil {
		return s.configOverride.NilDecl.RequireDeclarations
	}
	if s.configResolver == nil {
		return false
	}
	dir := passPackageDirectory(pass)
	if dir == "" {
		return false
	}
	cfg, err := s.configResolver.Resolve(dir)
	if err != nil || cfg == nil {
		return false
	}
	return cfg.NilDecl.RequireDeclarations
}

// checkInterfaceNilDeclCompleteness applies the completeness check to
// every named method of every interface node declares. Embedded
// elements carry no method name and are skipped: their positions
// belong to the embedded interface's own declaration site.
func checkInterfaceNilDeclCompleteness(pass *analysis.Pass, decls map[types.Object]*dsl.NilSignature, node *ast.GenDecl) {
	if node.Tok != token.TYPE {
		return
	}
	for _, spec := range node.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		iface, ok := ts.Type.(*ast.InterfaceType)
		if !ok || iface.Methods == nil {
			continue
		}
		for _, field := range iface.Methods.List {
			if len(field.Names) != 1 {
				continue
			}
			checkNilDeclCompleteness(pass, decls, pass.TypesInfo.Defs[field.Names[0]], field.Doc, field.Pos())
		}
	}
}

// checkNilDeclCompleteness emits at most one diagnostic naming the
// positions of obj's signature that carry no nullness decl. A doc
// comment holding a `vow:nil` marker the function-level decl set does
// not carry means the marker is subject-scoped or failed to parse:
// either way the enclosing signature is not what it describes, and
// the parse failure already surfaces its own diagnostic, so the
// position walk stays out.
func checkNilDeclCompleteness(pass *analysis.Pass, decls map[types.Object]*dsl.NilSignature, obj types.Object, doc *ast.CommentGroup, anchor token.Pos) {
	if obj == nil {
		return
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return
	}
	decl, declared := decls[obj]
	if !declared && docCarriesNilMarker(doc) {
		return
	}
	undeclared := undeclaredPositions(sig, decl)
	if len(undeclared) == 0 {
		return
	}
	vowReportNilDeclf(
		pass,
		anchor,
		"vow[nil-decl]: %s leaves %s without a nullness decl",
		nilDeclMarker,
		strings.Join(undeclared, ", "),
	)
}

// undeclaredPositions returns the human-readable labels of sig's
// nillable parameter and return positions that decl leaves at
// platform, in signature order. A nil decl means the declaration is
// absent entirely, which leaves every such position uncovered.
//
// The receiver stays out of scope: it is a prefix layer (`!.()`)
// rather than a slot in either list, and a value receiver cannot be
// nil at all. Nest layers stay out too — the completeness rule asks
// for the outermost nullness at each position, and the element layers
// of a slice of maps of pointers would multiply one signature into
// many demands.
func undeclaredPositions(sig *types.Signature, decl *dsl.NilSignature) []string {
	var paramSlots, returnSlots []*dsl.PositionDecl
	if decl != nil {
		paramSlots, returnSlots = decl.Params, decl.Returns
	}
	var out []string
	params, results := sig.Params(), sig.Results()
	for i := 0; i < params.Len(); i++ {
		if positionIsCovered(params.At(i).Type(), declSlotAt(paramSlots, i)) {
			continue
		}
		out = append(out, describePosition(params.At(i), "parameter", i))
	}
	for i := 0; i < results.Len(); i++ {
		if positionIsCovered(results.At(i).Type(), declSlotAt(returnSlots, i)) {
			continue
		}
		out = append(out, describePosition(results.At(i), "return", i))
	}
	return out
}

// positionIsCovered reports whether a position of type t needs no
// further decl, either because the type asks for none or because the
// slot already pins a nullness.
func positionIsCovered(t types.Type, slot *dsl.PositionDecl) bool {
	if !positionNeedsDecl(t) {
		return true
	}
	return slot != nil && slot.Nullness != dsl.NullnessNone
}

// declSlotAt returns the slot at index i, or nil when the list is
// shorter than the signature. A short list is what the empty-
// expression arity exception admits, so a missing slot is the ordinary
// way a one-position signature reaches this check undeclared.
func declSlotAt(slots []*dsl.PositionDecl, i int) *dsl.PositionDecl {
	if i >= len(slots) {
		return nil
	}
	return slots[i]
}

// positionNeedsDecl reports whether a position of type t asks the
// author for an explicit nullness decl: the type must be able to hold
// nil, and its nilness must be the author's to state.
//
// An unsubstituted type parameter is exempt, and the exemption has to
// be explicit: typeIsNillable reads a type parameter through its
// constraint's interface and answers true even for `T int`, which no
// nil-bearing type satisfies. That answer is the safe one where true
// widens an analysis, but here true would demand a decl at a position
// no instantiation can make nil.
func positionNeedsDecl(t types.Type) bool {
	if _, ok := t.(*types.TypeParam); ok {
		return false
	}
	if !typeIsNillable(t) {
		return false
	}
	return !typeCarriesNilnessConvention(t)
}

// typeCarriesNilnessConvention reports whether a Go-wide convention
// already settles t's nilness, leaving a per-position decl with
// nothing to add: `error` (nil is the success case),
// `context.Context` (callers pass a live context, never nil), and the
// empty interface (no shape to constrain, so neither token reads as
// a contract).
func typeCarriesNilnessConvention(t types.Type) bool {
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		if obj.Name() == "error" && obj.Pkg() == nil {
			return true
		}
		if obj.Name() == "Context" && obj.Pkg() != nil && obj.Pkg().Path() == "context" {
			return true
		}
	}
	iface, ok := t.Underlying().(*types.Interface)
	return ok && iface.Empty()
}

// describePosition names a position by its declared identifier when it
// has one, falling back to the axis word and the 1-based index for the
// unnamed and blank-identifier forms, so the label always identifies
// the slot the author must fill.
func describePosition(v *types.Var, axis string, i int) string {
	if name := v.Name(); name != "" && name != "_" {
		return axis + " " + name
	}
	return fmt.Sprintf("%s %d", axis, i+1)
}

// docCarriesNilMarker reports whether doc holds any `vow:nil` marker
// line, including the subject-scoped and malformed forms that never
// reach the function-level decl set.
func docCarriesNilMarker(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	return slices.ContainsFunc(doc.List, func(comment *ast.Comment) bool {
		line := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
		_, _, ok := stripMarkerScopeChain(line, nilDeclMarker)
		return ok
	})
}

// generatedHeader matches the canonical Go generated-code header.
// go.dev/s/generatedcode specifies a `//` line whose entire content is
// that sentence, trailing period included, so the match is anchored at
// both ends: a looser substring test would exempt a whole file whose
// prose merely mentions both halves, and the exemption is silent —
// nothing reports, so nobody notices the file left the rule's reach.
var generatedHeader = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// fileIsGenerated reports whether file carries the canonical Go
// generated-code header ahead of its package clause. Generated sources
// are exempt from the completeness rule: the contract belongs on the
// template or the schema the generator reads, and an author cannot
// answer the diagnostic in a file they must not edit.
func fileIsGenerated(file *ast.File) bool {
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			return false
		}
		if slices.ContainsFunc(group.List, func(comment *ast.Comment) bool {
			return generatedHeader.MatchString(comment.Text)
		}) {
			return true
		}
	}
	return false
}

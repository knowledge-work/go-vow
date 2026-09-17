package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// reasonFieldNilDecl is the human-readable rationale a nil-safety
// diagnostic uses when a field's declared `?` rides into a non-nil
// required position at the call site. The diagnostic prefixes this
// rationale with `field <name> ` so the field name and the rationale
// read together as one phrase. Centralised next to reasonNilDecl so
// the two rationale strings stay aligned at one source.
const reasonFieldNilDecl = "declared nillable via vow:nil"

// userFieldNilDecls walks every struct type declared in pass.Files
// and returns a map from each field's type-checker Object to the
// parsed nullness decl it carries. Fields whose doc holds no
// well-formed marker do not appear in the map; a malformed payload
// surfaces a diagnostic at the field's position so the parse error
// never silently disables the contract.
//
// Only the fields of declared struct types are walked here. A field of
// an anonymous struct type, and an embedded field whose name is absent
// in the AST, fall outside the collection; a marker at either position
// is reported as untracked rather than dropped, so an inert contract
// does not read as an enforced one.
func userFieldNilDecls(pass *analysis.Pass) map[types.Object]*dsl.PositionDecl {
	result := map[types.Object]*dsl.PositionDecl{}
	for st := range collectedStructTypes(pass) {
		collectStructFieldNilDecls(pass, st, result)
	}
	return result
}

// collectedStructTypes returns the struct types whose fields the decl
// collection reads: the direct type of a type declaration. It is the
// single answer to "would the collection reach a field here", consulted
// both by the collection itself and by the check that reports a marker
// sitting outside it, so the two cannot disagree about which positions
// are tracked.
func collectedStructTypes(pass *analysis.Pass) map[*ast.StructType]bool {
	out := map[*ast.StructType]bool{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || st.Fields == nil {
					continue
				}
				out[st] = true
			}
		}
	}
	return out
}

// collectStructFieldNilDecls walks every field on st and registers
// the parsed decl against each named field's Object. Grouped names
// (`A, B *string`) share one doc-comment and register the same decl
// against each Object so the caller-side lookup is identity-aware
// regardless of which name the author selects on a value expression.
func collectStructFieldNilDecls(pass *analysis.Pass, st *ast.StructType, into map[types.Object]*dsl.PositionDecl) {
	for _, field := range st.Fields.List {
		decl, ok := collectFieldNilDecl(pass, field)
		if !ok {
			continue
		}
		for _, name := range field.Names {
			obj := pass.TypesInfo.Defs[name]
			if obj == nil {
				continue
			}
			into[obj] = decl
		}
	}
}

// collectFieldNilDecl walks field's doc + line comments for the
// first well-formed marker and returns the parsed decl. Multiple
// marker lines on the same field are ill-formed — the first parsed
// decl wins and a duplicate-marker diagnostic anchors at the field
// so the author resolves the redundancy.
func collectFieldNilDecl(pass *analysis.Pass, field *ast.Field) (*dsl.PositionDecl, bool) {
	var (
		decl *dsl.PositionDecl
		seen bool
	)
	for _, comment := range fieldCommentLines(field) {
		line := trimCommentMarker(comment)
		rest, ok := stripMarkerPrefix(line, nilDeclMarker)
		if !ok {
			continue
		}
		payload := strings.TrimSpace(rest)
		parsed, err := dsl.ParseFieldNilDecl(payload)
		if err != nil {
			vowReportNilSafetyf(pass, field.Pos(), "vow[nil-safety]: %s: %v", nilDeclMarker, err)
			continue
		}
		if seen {
			vowReportNilSafetyf(pass, field.Pos(), "vow[nil-safety]: field carries more than one %s marker; keeping the first", nilDeclMarker)
			continue
		}
		decl = parsed
		seen = true
	}
	return decl, seen
}

// trimCommentMarker strips a comment line's leading slashes and
// surrounding space, yielding the text a marker prefix is matched
// against. Both the decl collection and the reach check read a comment
// through this helper so they agree on what counts as the line.
func trimCommentMarker(comment string) string {
	return strings.TrimSpace(strings.TrimPrefix(comment, "//"))
}

// stripMarkerPrefix returns the payload that follows marker on line,
// accepting any whitespace separator (space, tab) and also the bare
// marker with no payload at all. ok=false marks "line does not carry
// this marker" so the caller can move on without inspecting the
// remainder.
func stripMarkerPrefix(line, marker string) (string, bool) {
	if line == marker {
		return "", true
	}
	rest := strings.TrimPrefix(line, marker)
	if rest == line {
		return "", false
	}
	if rest == "" {
		return "", true
	}
	if rest[0] != ' ' && rest[0] != '\t' {
		return "", false
	}
	return rest, true
}

// fieldCommentLines returns the raw comment text lines that decorate
// field. The grammar reads both the leading doc-comment block and
// the trailing line-comment so authors can pick whichever placement
// reads better next to the field declaration.
func fieldCommentLines(field *ast.Field) []string {
	var lines []string
	if field.Doc != nil {
		for _, c := range field.Doc.List {
			lines = append(lines, c.Text)
		}
	}
	if field.Comment != nil {
		for _, c := range field.Comment.List {
			lines = append(lines, c.Text)
		}
	}
	return lines
}

// fieldNilSet returns the pass-wide map from a struct-field Object
// to its parsed vow:nil decl. Caller-side checks consult the map
// when an argument is a field-access expression; the field's
// declared nullness then flows into the caller-side judgement
// without re-parsing the doc-comment. Built lazily on first access
// and reused for the remainder of the pass.
//
// userFieldNilDecls calls vowReportNilSafetyf for malformed
// markers, and vowReport's filter chain takes the same passState
// mutex. Running the build outside the locked section keeps the
// non-reentrant sync.Mutex deadlock-free; the worst case is two
// concurrent callers each computing the same set, which is harmless
// because the result is deterministic.
func (s *passState) fieldNilSet(pass *analysis.Pass) map[types.Object]*dsl.PositionDecl {
	s.mu.Lock()
	built := s.fieldNilBuilt
	cached := s.fieldNilFields
	s.mu.Unlock()
	if built {
		return cached
	}
	out := userFieldNilDecls(pass)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.fieldNilBuilt {
		s.fieldNilFields = out
		s.fieldNilBuilt = true
	}
	return s.fieldNilFields
}

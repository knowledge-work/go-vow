package analysis

import (
	"go/ast"
	"go/token"
	"strings"
)

// importMarker is the doc-comment marker that introduces an alias
// for a preset's rule namespace, e.g.
//
//	// vow:import result "preset/std/result"
//
// The line is consumed by the analyzer to build a RuleScope; it has
// no effect on the Go program itself.
const importMarker = "vow:import"

// importEntry pairs a parsed `vow:import` declaration with the source
// position of its comment, so callers can attribute diagnostics back
// to the offending line.
type importEntry struct {
	alias string
	path  string
	pos   token.Pos
}

// parseImportAnnotations walks a doc comment and returns the
// `vow:import` declarations it finds, preserving source order.
// Malformed lines (missing alias, unquoted path, extra tokens) are
// skipped silently; surfacing those as diagnostics is not
// implemented.
func parseImportAnnotations(doc *ast.CommentGroup) []importEntry {
	if doc == nil {
		return nil
	}
	var out []importEntry
	for _, c := range doc.List {
		line := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		if line != importMarker && !strings.HasPrefix(line, importMarker+" ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, importMarker))
		alias, path, ok := splitImportPayload(payload)
		if !ok {
			continue
		}
		out = append(out, importEntry{alias: alias, path: path, pos: c.Pos()})
	}
	return out
}

// splitImportPayload extracts (alias, path) from `<alias> "<path>"`.
// The alias must be a non-empty identifier-shaped token; the path
// must be wrapped in straight double quotes. Trailing content after
// the closing quote (typically an analysistest want-marker) is
// ignored so fixtures can place want-comments on the same line.
// Returns ok=false on any deviation.
func splitImportPayload(payload string) (string, string, bool) {
	parts := strings.SplitN(payload, " ", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	alias := strings.TrimSpace(parts[0])
	rest := strings.TrimSpace(parts[1])
	if alias == "" || len(rest) < 2 || rest[0] != '"' {
		return "", "", false
	}
	end := strings.IndexByte(rest[1:], '"')
	if end < 0 {
		return "", "", false
	}
	return alias, rest[1 : 1+end], true
}

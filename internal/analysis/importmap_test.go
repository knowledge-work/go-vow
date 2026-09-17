package analysis

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
)

func TestParseImportAnnotations(t *testing.T) {
	src := `// Package fixture demonstrates vow:import annotations.
//
// vow:import result "preset/std/result"
// vow:import std    "preset/std/either"
package fixture
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, parser.ParseComments)
	assert.MustNoError(t, "ParseFile", err)
	got := parseImportAnnotations(file.Doc)
	assert.MustLen(t, "parseImportAnnotations", got, 2)
	assert.Equal(t, "got[0].alias", got[0].alias, "result")
	assert.Equal(t, "got[0].path", got[0].path, "preset/std/result")
	assert.Equal(t, "got[1].alias", got[1].alias, "std")
	assert.Equal(t, "got[1].path", got[1].path, "preset/std/either")
}

func TestParseImportAnnotations_ignoresMalformed(t *testing.T) {
	src := `// vow:import only-one-token
// vow:import missing-quotes preset/std/result
// vow:import ok "preset/std/result"
package fixture
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, parser.ParseComments)
	assert.MustNoError(t, "ParseFile", err)
	got := parseImportAnnotations(file.Doc)
	assert.MustLen(t, "parseImportAnnotations; the malformed lines drop", got, 1)
	assert.Equal(t, "got[0].alias", got[0].alias, "ok")
	assert.Equal(t, "got[0].path", got[0].path, "preset/std/result")
}

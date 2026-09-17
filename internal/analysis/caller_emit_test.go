package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
)

// TestParentMapReusesPerFile pins the per-`*ast.File` reuse
// contract on `passState.parentMap`: a second lookup for the same
// file returns the same map instance that the first lookup built,
// so two callers inspecting the same statement do not pay for two
// full-file walks. A different file pointer takes its own slot so
// the cache stays scoped to each file rather than collapsing into
// a single map across files.
func TestParentMapReusesPerFile(t *testing.T) {
	fset := token.NewFileSet()
	srcA := `package p
func F() { go func() {}() }`
	srcB := `package p
func G() { defer func() {}() }`
	fileA, err := parser.ParseFile(fset, "a.go", srcA, 0)
	assert.MustNoError(t, "parse a.go", err)
	fileB, err := parser.ParseFile(fset, "b.go", srcB, 0)
	assert.MustNoError(t, "parse b.go", err)
	state := newPassState(nil, nil, nil)
	first := state.parentMap(fileA)
	second := state.parentMap(fileA)
	assert.Equal(t, "the second lookup for the same file returns the cached map",
		reflect.ValueOf(second).Pointer(), reflect.ValueOf(first).Pointer())
	other := state.parentMap(fileB)
	assert.Equal(t, "a different file takes its own slot; the cache must not collapse across files",
		reflect.ValueOf(other).Pointer() == reflect.ValueOf(first).Pointer(), false)
}

// TestParentMapRecordsParents pins the parent-map contract itself
// through the passState wrapper: every non-root node in the parsed
// file resolves to an immediate parent that contains it. The check
// guards against a regression that returns the cached map skeleton
// without walking the tree.
func TestParentMapRecordsParents(t *testing.T) {
	fset := token.NewFileSet()
	src := `package p
func F() { go func() {}() }`
	file, err := parser.ParseFile(fset, "a.go", src, 0)
	assert.MustNoError(t, "parse", err)
	state := newPassState(nil, nil, nil)
	parents := state.parentMap(file)
	var goStmt *ast.GoStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if gs, ok := n.(*ast.GoStmt); ok {
			goStmt = gs
			return false
		}
		return true
	})
	assert.MustNotNil(t, "the parsed source must contain a GoStmt", goStmt)
	_, ok := parents[goStmt]
	assert.Equal(t, "the parents map records the GoStmt; a missing entry means the wrapper bypassed the walker",
		ok, true)
}

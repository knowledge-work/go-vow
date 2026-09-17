package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// writeTempFile materializes content under t.TempDir and returns its
// path, so parser- and scanner-level tests run against real files
// the same way the resolver does.
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	assert.MustNoError(t, "WriteFile "+name, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestDeclaredIdents(t *testing.T) {
	path := writeTempFile(t, "decls.go", `package sample

import "example.com/dep"

const answer = 42

var Exported, unexported int

type Widget struct {
	Field   int
	nested  struct{ Inner bool }
	Embedded
	*PtrEmbedded
	dep.Qualified
	GenericEmbed[int]
}

type Alias = Widget

type List[T any] struct{ Items []T }

type Doer interface {
	Do(x int) error
}

func New() *Widget { return nil }

func Options() struct{ Debug bool } { return struct{ Debug bool }{} }

var Box struct{ Lid int }

func (w *Widget) Reset() {}

func helper() {
	type localOnly struct{ LocalField int }
	_ = localOnly{}
}

var _ = answer
`)
	got, err := declaredIdents([]string{path})
	assert.MustNoError(t, "declaredIdents", err)
	// Notably absent: the type parameter T, the function-local
	// localOnly/LocalField, and the blank identifier.
	want := []string{
		"Alias", "Box", "Debug", "Do", "Doer", "Embedded", "Exported",
		"Field", "GenericEmbed", "Inner", "Items", "Lid", "List", "New",
		"Options", "PtrEmbedded", "Qualified", "Reset", "Widget",
		"answer", "helper", "nested", "unexported",
	}
	var gotSorted []string
	for name := range got {
		gotSorted = append(gotSorted, name)
	}
	sort.Strings(gotSorted)
	assert.DeepEqual(t, "declaredIdents", gotSorted, want)
}

func TestDeclaredIdentsMultipleFiles(t *testing.T) {
	a := writeTempFile(t, "a.go", "package sample\n\nfunc FromA() {}\n")
	b := writeTempFile(t, "b.go", "package sample\n\nfunc FromB() {}\n")
	got, err := declaredIdents([]string{a, b})
	assert.MustNoError(t, "declaredIdents", err)
	for _, name := range []string{"FromA", "FromB"} {
		_, ok := got[name]
		assert.Equal(t, "declaredIdents has "+name, ok, true)
	}
}

func TestDeclaredIdentsParseError(t *testing.T) {
	path := writeTempFile(t, "broken.go", "package sample\n\nfunc {\n")
	_, err := declaredIdents([]string{path})
	assert.Error(t, "declaredIdents on an unparseable file", err)
}

func TestFileMentionsAny(t *testing.T) {
	idents := map[string]struct{}{"Widget": {}, "識別子": {}}
	type mentionCase struct {
		content string
		want    bool
	}

	tabletest.Run(t, map[string]mentionCase{
		"selector reference":                            {content: "package p\nvar w = callee.Widget{}\n", want: true},
		"comment-only mention still matches":            {content: "package p\n// Widget is mentioned here only.\n", want: true},
		"substring of longer identifier does not match": {content: "package p\nvar w = callee.WidgetFactory{}\nvar x = MyWidget{}\n", want: false},
		"underscore-joined identifiers do not match":    {content: "package p\nvar w = Widget_x\nvar x = _Widget\n", want: false},
		"digit-joined identifier does not match":        {content: "package p\nvar w = Widget2\n", want: false},
		"identifier at EOF without trailing newline":    {content: "package p\nvar w = callee.Widget", want: true},
		"multi-byte identifier matches":                 {content: "package p\nvar _ = callee.識別子\n", want: true},
		"multi-byte run does not split":                 {content: "package p\nvar _ = callee.識別子だよ\n", want: false},
		"no mention":                                    {content: "package p\nvar w = callee.Other()\n", want: false},
	}, func(t *testing.T, c mentionCase) {
		path := writeTempFile(t, "sample.go", c.content)
		got, err := fileMentionsAny(path, idents)
		assert.MustNoError(t, "fileMentionsAny", err)
		assert.Equal(t, "fileMentionsAny", got, c.want)
	})
}

// fixtureDir resolves the textmatch fixture module relative to this
// test file.
func fixtureDir(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	assert.MustEqual(t, "runtime.Caller(0) resolved this test file's path", ok, true)
	return filepath.Join(filepath.Dir(testFile), "testdata", "textmatch")
}

// TestResolveScopePackages_TextMatchScope pins the resolver's
// file-granular caller semantics on the textmatch fixture module:
// a package referencing an identifier declared in the changed file
// stays (including one that only mentions a struct field name),
// while an importer that only references the owning package's other
// files is dropped.
func TestResolveScopePackages_TextMatchScope(t *testing.T) {
	fixture := fixtureDir(t)
	t.Chdir(fixture)
	changed := filepath.Join(fixture, "callee", "callee.go")
	scope, err := ResolveScopePackages([]string{changed}, nil)
	assert.MustNoError(t, "ResolveScopePackages", err)
	assert.MustNotNil(t, "scope; the fixture's changed file must resolve to a non-nil scope", scope)
	assert.DeepEqual(t, "Changed", scope.Changed, []string{"example.com/textmatch/callee"})
	// caller_other must be dropped: it references the owning package's
	// other files only.
	assert.DeepEqual(t, "Callers", scope.Callers, []string{
		"example.com/textmatch/caller_field",
		"example.com/textmatch/caller_new",
	})
}

// TestResolveScopePackages_IgnoresNonGoChangedFiles pins the
// pre-parse filter: paths Phase 1 does not recognise as Go sources
// (build files, docs, deleted paths) drop out silently instead of
// failing the whole resolution, matching how the changed-package
// mapping has always treated them.
func TestResolveScopePackages_IgnoresNonGoChangedFiles(t *testing.T) {
	fixture := fixtureDir(t)
	t.Chdir(fixture)
	changed := []string{
		filepath.Join(fixture, "callee", "callee.go"),
		filepath.Join(fixture, "go.mod"),
		filepath.Join(fixture, "no-such-file.md"),
	}
	scope, err := ResolveScopePackages(changed, nil)
	assert.MustNoError(t, "ResolveScopePackages with non-Go changed files", err)
	assert.MustNotNil(t, "scope; the Go changed file must still resolve", scope)
	assert.DeepEqual(t, "Callers", scope.Callers, []string{
		"example.com/textmatch/caller_field",
		"example.com/textmatch/caller_new",
	})
}

// TestResolveScopePackages_NoDeclarations pins the early return for
// a changed file that declares nothing referenceable: the owning
// package is still the changed set, and the caller set is empty.
func TestResolveScopePackages_NoDeclarations(t *testing.T) {
	fixture := fixtureDir(t)
	t.Chdir(fixture)
	changed := filepath.Join(fixture, "callee", "doc.go")
	scope, err := ResolveScopePackages([]string{changed}, nil)
	assert.MustNoError(t, "ResolveScopePackages", err)
	assert.MustNotNil(t, "scope; a declaration-free changed file still resolves its owning package", scope)
	assert.DeepEqual(t, "Changed", scope.Changed, []string{"example.com/textmatch/callee"})
	// No identifiers to reference, so nothing can be a caller.
	assert.Len(t, "Callers", scope.Callers, 0)
}

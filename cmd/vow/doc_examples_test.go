package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestDocExamples runs the Go example of README.md and
// docs/getting-started.md through the analyzer with the built-in
// presets. Each example must be silent as written. A silent run looks
// the same whether the analyzer reads a marker or ignores it, so each
// case that edits an example as its prose describes must report the
// diagnostic message the prose promises.
func TestDocExamples(t *testing.T) {
	type exampleCase struct {
		doc  string // the markdown file, relative to the repository root
		drop string // a line deleted from the example before the run
		add  string // code appended to the example before the run
		want string // text the diagnostics must contain; "" requires a silent run
	}
	tabletest.Run(t, map[string]exampleCase{
		"README example is silent as written": {doc: "README.md"},
		"README example leaks without its vow:emit": {
			doc:  "README.md",
			drop: "// vow:emit ErrNotFound\n",
			want: "sentinel error ErrNotFound leaked",
		},
		"README example reports a nil id at the call site": {
			doc:  "README.md",
			add:  "\nfunc lookupNil() { Lookup(nil) }\n",
			want: "argument 1 (id) is nil",
		},
		"getting-started example is silent as written": {doc: "docs/getting-started.md"},
		"getting-started example leaks without its vow:cond": {
			doc:  "docs/getting-started.md",
			drop: "// vow:cond * -> _, ErrNotFound | nil\n",
			want: "sentinel error ErrNotFound leaked",
		},
	}, func(t *testing.T, c exampleCase) {
		example := goExample(t, filepath.Join("..", "..", c.doc))
		if c.drop != "" {
			assert.MustContains(t, c.doc+" example", example, c.drop)
			example = strings.Replace(example, c.drop, "", 1)
		}
		dir := t.TempDir()
		assert.MustNoError(t, "write go.mod", os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/store\n\ngo 1.25\n"), 0o644))
		assert.MustNoError(t, "write store.go", os.WriteFile(filepath.Join(dir, "store.go"), []byte(example+c.add), 0o644))
		got := runAnalyzer(t, dir, false)
		if c.want == "" {
			assert.Equal(t, "diagnostics", got, "")
			return
		}
		assert.Contains(t, "diagnostics", got, c.want)
	})
}

// goFenceOpener matches the opening line of a ``` or ~~~ fence indented at
// most three spaces, whose language is go or golang in any case, alone or
// followed by whitespace and any attributes. It misses a fence after a
// list or blockquote marker, or indented four or more spaces in a list.
var goFenceOpener = regexp.MustCompile("(?im)^ {0,3}(`{3,}|~{3,})[ \t]*(go|golang)([ \t].*)?$")

// goExample returns the body of the only Go fence in the markdown file
// at path, so an example split across fences fails the test instead of
// leaving its later fences unchecked.
func goExample(t *testing.T, path string) string {
	t.Helper()
	doc, err := os.ReadFile(path)
	assert.MustNoError(t, "read "+path, err)
	assert.MustLen(t, path+" Go fences", goFenceOpener.FindAllString(string(doc), -1), 1)
	_, rest, found := strings.Cut(string(doc), "```go\n")
	assert.MustEqual(t, path+" opens a ```go fence", found, true)
	example, _, found := strings.Cut(rest, "\n```")
	assert.MustEqual(t, path+" closes its ```go fence", found, true)
	return example + "\n"
}

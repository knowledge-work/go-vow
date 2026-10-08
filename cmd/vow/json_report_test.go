package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/cli"
	"github.com/knowledge-work/go-vow/internal/driver"
)

// TestDriverReportMatchesTheVetShape pins the default path's report to
// the `go vet -json` shape: errors from every job, diagnostics from root
// jobs only, and an end position that falls back to the start.
func TestDriverReportMatchesTheVetShape(t *testing.T) {
	fset := token.NewFileSet()
	file := fset.AddFile("/src/a.go", -1, 100)
	file.SetLines([]int{0, 10, 20})
	vow := &analysis.Analyzer{Name: "vow"}
	result := &driver.Result{Jobs: []*driver.Job{
		{Analyzer: &analysis.Analyzer{Name: "inspect"}, Package: packageWithID("example.com/a", fset), Err: errors.New("boom")},
		{Analyzer: vow, Package: packageWithID("example.com/dep", fset), Diagnostics: []analysis.Diagnostic{{Pos: file.Pos(12), Message: "from a dependency"}}},
		{Analyzer: vow, Package: packageWithID("example.com/clean", fset), Root: true},
		{Analyzer: vow, Package: packageWithID("example.com/a", fset), Root: true, Diagnostics: []analysis.Diagnostic{
			{Pos: file.Pos(12), Category: "nil-safety", Message: "m", Related: []analysis.RelatedInformation{{Pos: file.Pos(22), Message: "see"}}},
			{Pos: file.Pos(2), Message: "bare"},
		}},
	}}
	assert.Equal(t, "driverReport as JSON", compactJSON(t, jsonReport(driverReport(result))),
		`{"example.com/a":{"inspect":{"error":"boom"},"vow":[{"category":"nil-safety","posn":"/src/a.go:2:3","end":"/src/a.go:2:3","message":"m","related":[{"posn":"/src/a.go:3:3","end":"/src/a.go:3:3","message":"see"}]},{"posn":"/src/a.go:1:3","end":"/src/a.go:1:3","message":"bare"}]}}`)
}

func TestJSONReportWritesTheShapeParseVetReportReads(t *testing.T) {
	raw := `{"example.com/a":{"vow":[{"category":"c","posn":"a.go:1:1","end":"a.go:1:2","message":"m","related":[{"posn":"a.go:2:1","end":"a.go:2:1","message":"see"}]}]},"example.com/b":{"vow":{"error":"boom"}}}`
	report, err := parseVetReport([]byte(raw))
	assert.MustNoError(t, "parseVetReport", err)
	assert.Equal(t, "jsonReport of the parsed report", compactJSON(t, jsonReport(report)), raw)
}

func TestJSONReportOfNothingIsADocument(t *testing.T) {
	assert.Equal(t, "jsonReport(nil)", string(jsonReport(nil)), "{}\n")
	assert.Equal(t, "jsonReport of an empty report", string(jsonReport(map[string]map[string]vetResult{})), "{}\n")
}

// TestJSONMatchesAcrossPaths runs -json on both paths over the
// narrowseam fixture. The vet path's diagnostics carry the narrow-scope
// annotation as related information, so the two reports agree only once
// vow strips it.
func TestJSONMatchesAcrossPaths(t *testing.T) {
	binary := buildVowBinary(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "narrowseam"))
	assert.MustNoError(t, "resolve fixture", err)
	narrow := []string{
		"--config-yaml", "nil_decl:\n  require_declarations: false\n",
		"--changed-files", filepath.Join(dir, "p", "a.go"), "--with-callers",
	}
	defaultText := runVowBinaryStreams(t, binary, dir, slices.Concat(narrow, []string{"./..."})...)
	defaultJSON := runVowBinaryStreams(t, binary, dir, slices.Concat(narrow, []string{"-json", "./..."})...)
	vetText := runVowBinaryStreams(t, binary, dir, slices.Concat([]string{"--vet-mode"}, narrow, []string{"./..."})...)
	vetJSON := runVowBinaryStreams(t, binary, dir, slices.Concat([]string{"--vet-mode"}, narrow, []string{"-json", "./..."})...)

	// The positive control: two runs that reported nothing would agree
	// too, so the comparisons only mean something once the kept
	// diagnostic is present.
	const kept = "may be nil through flow"
	assert.MustContains(t, "default -json stdout", defaultJSON.stdout, kept)
	assert.MustContains(t, "default -json stdout", defaultJSON.stdout, `"narrowseam/p": {`)
	assert.Equal(t, "vet-mode -json stdout", vetJSON.stdout, defaultJSON.stdout)
	assert.NotContains(t, "default -json stderr", defaultJSON.stderr, kept)
	assert.NotContains(t, "vet-mode -json stderr", vetJSON.stderr, kept)
	assert.Equal(t, "default text stdout", defaultText.stdout, "")
	assert.Equal(t, "vet-mode text stdout", vetText.stdout, "")

	if defaultText.code == 0 {
		t.Fatalf("the text run exited 0, so the exit-code comparison proves nothing:\n%s", defaultText.stderr)
	}
	assert.Equal(t, "default exit code with -json", defaultJSON.code, defaultText.code)
	assert.Equal(t, "vet-mode exit code with -json", vetJSON.code, vetText.code)
}

// TestJSONPrintsADocumentWhenNothingIsLinted pins that a run whose
// --changed-files resolve to no lintable package, which exits 0 without
// analyzing, still prints a document for a JSON consumer.
func TestJSONPrintsADocumentWhenNothingIsLinted(t *testing.T) {
	binary := buildVowBinary(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "nolintable"))
	assert.MustNoError(t, "resolve fixture", err)
	run := runVowBinaryStreams(t, binary, dir,
		"--changed-files", filepath.Join(dir, "p", "ignored.go"), "--with-callers", "-json", "./...")
	assert.MustContains(t, "stderr, which names the exit the run took", run.stderr, "nothing to lint")
	assert.Equal(t, "stdout", run.stdout, "{}\n")
	assert.Equal(t, "exit code", run.code, 0)
}

// TestJSONKeysTheTestVariantByPath pins the key each path gives a package
// compiled with its test files under -test, as docs/cli.md describes.
func TestJSONKeysTheTestVariantByPath(t *testing.T) {
	binary := buildVowBinary(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "testflag"))
	assert.MustNoError(t, "resolve fixture", err)
	args := []string{"--config-yaml", "{}", "-test", "-json", "./..."}
	defaultJSON := runVowBinaryStreams(t, binary, dir, args...)
	vetJSON := runVowBinaryStreams(t, binary, dir, slices.Concat([]string{"--vet-mode"}, args)...)
	assert.MustContains(t, "default -json stdout", defaultJSON.stdout, `"vowtestflag.example [vowtestflag.example.test]": {`)
	assert.MustContains(t, "vet-mode -json stdout", vetJSON.stdout, `"vowtestflag.example": {`)
	assert.NotContains(t, "vet-mode -json stdout", vetJSON.stdout, "[vowtestflag.example.test]")
}

// vowRun is what one run of the built binary wrote and how it exited.
type vowRun struct {
	stdout, stderr string
	code           int
}

// runVowBinaryStreams runs the built binary in dir and keeps its two
// output streams apart, since -json moves the report from stderr to
// stdout.
func runVowBinaryStreams(t *testing.T, binary, dir string, args ...string) vowRun {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	// The timing log would add lines to stderr.
	cmd.Env = append(os.Environ(), "GOFLAGS=", cli.TimingEnvVar+"=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("run vow: %v", err)
	}
	return vowRun{stdout: stdout.String(), stderr: stderr.String(), code: cmd.ProcessState.ExitCode()}
}

func packageWithID(id string, fset *token.FileSet) *packages.Package {
	return &packages.Package{ID: id, Fset: fset}
}

func compactJSON(t *testing.T, data []byte) string {
	t.Helper()
	var compact bytes.Buffer
	assert.MustNoError(t, "compact JSON", json.Compact(&compact, data))
	return compact.String()
}

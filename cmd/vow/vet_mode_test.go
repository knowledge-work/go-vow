package main

import (
	"fmt"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/cli"
	"github.com/knowledge-work/go-vow/internal/config"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestRunsAsVettool(t *testing.T) {
	type vettoolCase struct {
		args   []string
		marked bool
		want   bool
	}
	tabletest.Run(t, map[string]vettoolCase{
		"flag list request":                    {args: []string{"-flags"}, want: true},
		"build identity request":               {args: []string{"-V=full"}, want: true},
		"unit config alone":                    {args: []string{"/tmp/b001/vet.cfg"}, want: true},
		"unit config beside a forwarded flag":  {args: []string{"-json", "/tmp/b001/vet.cfg"}, want: true},
		"unit config under the wrapper marker": {args: []string{"-json", "/tmp/b001/vet.cfg"}, marked: true, want: true},
		"driver config path ending in cfg":     {args: []string{"--config-file", "policy.cfg", "./..."}, want: false},
		"driver run":                           {args: []string{"--changed-files", "a.go", "--with-callers", "./..."}, want: false},
		"driver run with no arguments":         {args: nil, want: false},
	}, func(t *testing.T, c vettoolCase) {
		assert.Equal(t, fmt.Sprintf("runsAsVettool(%q, marked=%v)", c.args, c.marked),
			runsAsVettool(c.args, c.marked), c.want)
	})
}

func TestVetConfigValueSet(t *testing.T) {
	cfg := new(config.Config)
	value := &vetConfigValue{cfg: cfg}
	assert.MustNoError(t, "Set", value.Set("nil_decl:\n  require_declarations: true\n"))
	// The decoded policy has to land in the Config the analyzer closed
	// over, and a config the go command already keyed on has to fail
	// loudly rather than run with a zero policy.
	assert.Equal(t, "NilDecl.RequireDeclarations after Set", cfg.NilDecl.RequireDeclarations, true)
	assert.Error(t, "Set with malformed yaml", value.Set("nil_decl:\n  require_declarations: not-a-bool\n"))
}

func TestParseVetReportKeepsAnAnalyzerError(t *testing.T) {
	// A package whose analyzer failed is reported as an object rather
	// than a diagnostic list, and a decoder that admits only the list
	// discards every other package's result with it.
	report, err := parseVetReport([]byte(`{"example.com/a":{"vow":{"error":"boom"}}}`))
	assert.MustNoError(t, "parseVetReport", err)
	assert.Equal(t, `report["example.com/a"]["vow"].Err`, report["example.com/a"]["vow"].Err, "boom")
}

func TestVetPackagePathStripsTestVariants(t *testing.T) {
	type vetPathCase struct{ id, want string }
	tabletest.Run(t, map[string]vetPathCase{
		"example.com/p":                      {"example.com/p", "example.com/p"},
		"example.com/p_test":                 {"example.com/p_test", "example.com/p"},
		"example.com/p.test":                 {"example.com/p.test", "example.com/p"},
		"example.com/p [example.com/p.test]": {"example.com/p [example.com/p.test]", "example.com/p"},
	}, func(t *testing.T, c vetPathCase) {
		assert.Equal(t, fmt.Sprintf("vetPackagePath(%q)", c.id), vetPackagePath(c.id), c.want)
	})
}

func TestPrintVetDiagnosticsDropsTestFilesUnlessAsked(t *testing.T) {
	report := map[string]map[string]vetResult{
		"example.com/p_test": {"vow": {Diagnostics: []vetDiagnostic{{Posn: "p/a_test.go:1:1", Message: "in a test file"}}}},
		"example.com/p":      {"vow": {Diagnostics: []vetDiagnostic{{Posn: "p/a.go:2:2", Message: "in a source file"}}}},
	}
	scope := cli.ChangedFileSet{Files: []string{"p/a.go", "p/a_test.go"}, ChangedPackages: []string{"example.com/p"}}
	printed, failed := printVetDiagnostics(report, scope, false)
	assert.Equal(t, "printVetDiagnostics without tests: printed", printed, 1)
	assert.Equal(t, "printVetDiagnostics without tests: failed", failed, 0)
	printed, failed = printVetDiagnostics(report, scope, true)
	assert.Equal(t, "printVetDiagnostics with tests: printed", printed, 2)
	assert.Equal(t, "printVetDiagnostics with tests: failed", failed, 0)
}

func TestPrintVetDiagnosticsCountsAnalyzerFailures(t *testing.T) {
	report := map[string]map[string]vetResult{
		"example.com/p": {"vow": {Err: "boom"}},
	}
	printed, failed := printVetDiagnostics(report, cli.ChangedFileSet{}, false)
	assert.Equal(t, "printVetDiagnostics printed", printed, 0)
	assert.Equal(t, "printVetDiagnostics failed", failed, 1)
}

func TestParseVetReportMergesTheStream(t *testing.T) {
	// The go command emits one report object per batch of units, so a
	// run that analyzes several packages produces several objects back
	// to back rather than one for the whole run.
	raw := []byte(`{"example.com/a":{"vow":[{"posn":"a.go:1:1","message":"first"}]}}
{"example.com/b":{"vow":[{"posn":"b.go:2:2","message":"second"}]}}`)
	report, err := parseVetReport(raw)
	assert.MustNoError(t, "parseVetReport", err)
	assert.MustEqual(t, "len(report)", len(report), 2)
	assert.Equal(t, `report["example.com/b"]["vow"].Diagnostics[0].Message`,
		report["example.com/b"]["vow"].Diagnostics[0].Message, "second")
}

func TestParseVetReportEmptyOutput(t *testing.T) {
	report, err := parseVetReport([]byte("\n"))
	assert.MustNoError(t, "parseVetReport", err)
	assert.Equal(t, "len(report) for empty output", len(report), 0)
}

func TestNarrowPackagesCoversChangedAndCallers(t *testing.T) {
	scope := narrowPackages(cli.ChangedFileSet{
		Files:           []string{"/repo/changed/a.go"},
		ChangedPackages: []string{"example.com/changed"},
		CallerPackages:  []string{"example.com/caller"},
	})
	for _, path := range []string{"example.com/changed", "example.com/caller"} {
		assert.Equal(t, fmt.Sprintf("scope[%q]", path), scope[path], true)
	}
	assert.Equal(t, `scope["example.com/other"]; a package the run did not target`,
		scope["example.com/other"], false)
}

func TestVetConfigYAMLPrefersInlineContent(t *testing.T) {
	assert.Equal(t, "vetConfigYAML with inline content",
		vetConfigYAML(driverFlags{ConfigYAML: "nil_decl: {}"}), "nil_decl: {}")
	assert.Equal(t, "vetConfigYAML with no config", vetConfigYAML(driverFlags{}), "")
}

func TestPrintVetDiagnosticsKeepsOnlyTheScope(t *testing.T) {
	report := map[string]map[string]vetResult{
		"example.com/changed": {"vow": {Diagnostics: []vetDiagnostic{{Posn: "changed.go:1:1", Message: "in scope"}}}},
		"example.com/dep":     {"vow": {Diagnostics: []vetDiagnostic{{Posn: "dep.go:2:2", Message: "out of scope"}}}},
	}
	// A dependency is analyzed so its facts reach the packages under
	// test, and `go vet` reports for it like any other package, so the
	// filter is what keeps those reports out of the run's output.
	scoped, _ := printVetDiagnostics(report, cli.ChangedFileSet{Files: []string{"changed.go"}, ChangedPackages: []string{"example.com/changed"}}, false)
	assert.Equal(t, "printVetDiagnostics printed", scoped, 1)
	unscoped, _ := printVetDiagnostics(report, cli.ChangedFileSet{}, false)
	assert.Equal(t, "printVetDiagnostics printed with an empty scope", unscoped, 2)
}

func TestDropByNarrowScopeAppliesTheDriverRule(t *testing.T) {
	narrow := cli.ChangedFileSet{
		Files:           []string{"/repo/p/a.go"},
		ChangedPackages: []string{"example.com/p"},
		CallerPackages:  []string{"example.com/caller"},
	}
	type narrowCase struct {
		diag vetDiagnostic
		pkg  string
		want bool
	}
	tabletest.Run(t, map[string]narrowCase{
		"inside a changed file":                              {diag: vetDiagnostic{Posn: "/repo/p/a.go:1:1"}, pkg: "example.com/p"},
		"outside every in-scope package":                     {diag: vetDiagnostic{Posn: "/repo/z/z.go:1:1"}, pkg: "example.com/z", want: true},
		"anywhere in a caller package":                       {diag: vetDiagnostic{Posn: "/repo/c/c.go:1:1", Related: relatedWith("call=none")}, pkg: "example.com/caller"},
		"unchanged file of a changed package, not at a call": {diag: vetDiagnostic{Posn: "/repo/p/b.go:1:1", Related: relatedWith("call=none")}, pkg: "example.com/p", want: true},
		"call whose callee is in a changed file":             {diag: vetDiagnostic{Posn: "/repo/p/b.go:2:2", Related: relatedWith("callee-file=/repo/p/a.go")}, pkg: "example.com/p"},
		"call whose callee is elsewhere":                     {diag: vetDiagnostic{Posn: "/repo/p/b.go:3:3", Related: relatedWith("callee-file=/repo/p/b.go")}, pkg: "example.com/p", want: true},
		"call whose callee could not be resolved":            {diag: vetDiagnostic{Posn: "/repo/p/b.go:4:4", Related: relatedWith("callee=unknown")}, pkg: "example.com/p", want: true},
		// An unannotated diagnostic means the rule has no opinion, and
		// dropping on no opinion loses a report with nothing to show for
		// it.
		"no annotation": {diag: vetDiagnostic{Posn: "/repo/p/b.go:5:5"}, pkg: "example.com/p"},
	}, func(t *testing.T, c narrowCase) {
		assert.Equal(t, "dropByNarrowScope", dropByNarrowScope(c.diag, c.pkg, narrow), c.want)
	})
}

// relatedWith builds the related information the analyzer attaches for a
// narrow-scope decision.
func relatedWith(annotation string) []struct {
	Message string `json:"message"`
} {
	return []struct {
		Message string `json:"message"`
	}{{Message: "vow:narrow " + annotation}}
}

func TestVetPatternsFallsBackToTheModule(t *testing.T) {
	// `go vet` with no pattern analyzes the current directory alone, so a
	// run that named no package has to be widened before the command is
	// built rather than after.
	assert.DeepEqual(t, "vetPatterns(nil)", vetPatterns(nil), []string{"./..."})
	assert.DeepEqual(t, `vetPatterns([]string{"./p"})`, vetPatterns([]string{"./p"}), []string{"./p"})
}

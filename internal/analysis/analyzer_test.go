package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/cli"
	"github.com/knowledge-work/go-vow/internal/dsl"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "readonlyWrites", "sentinels", "composed", "import_unknown", "importPkgScope", "qualified_pkg", "qualified_consumer", "suppress", "discharged", "vowUse", "useHint", "concept", "vowEmit", "nilDeclBasic", "nilDeclKeywordForm", "nilDeclNest", "nilDeclNestArity", "nilDeclNestGenericNarrow", "nilDeclLayer", "nilDeclReassign", "nilDeclNarrow", "nilDeclFieldNarrow", "nilDeclFieldDeref", "nilDeclField", "nilDeclFieldAssign", "nilDeclFieldAssignFlow", "nilDeclCallResult", "nilDeclFieldConstruct", "nilDeclFieldConstructWhole", "nilDeclFieldReach", "nilDeclFieldGeneric", "nilDeclSelfBasic", "nilDeclCallerDeadGuard", "nilDeclSelfFlow", "nilDeclSelfNest", "nilDeclSelfRecv", "nilDeclReceiver", "nilDeclInterfaceMethod", "nilDeclXPkgCallee", "nilDeclXPkgCaller", "lifecycle_defer", "lifecycle_async", "lifecycle_callback", "lifecycle_storage", "lifecycle_channel", "higher_order", "higher_order_chain", "condGrammar", "condLogicalXPkgCallee", "condLogicalXPkgCaller", "condInconsistency", "condTagExhaustive", "condRangeNarrow", "condValueSum", "condPathNarrow", "condLifecyclePair", "condLifecyclePairXPkgCallee", "condLifecyclePairXPkgCaller", "closableGeneric", "closableStdHandle", "condCallerDeadGuard", "condCallerSafetyViolation", "condCallerNegativeForm", "condCallerGeneralCmp", "condUnresolvedRuleRef", "condTupleTypedElement", "condTupleXPkgPayload", "condTupleXPkgType")
}

// TestAnalyzerWrappedReturn drives the fifth vow:emit destination
// — wrapped return — through a separate analyzer instance that
// loads the sample passthrough-emit preset from example/. The
// shipped binary carries the sentinel-error and closable presets
// only; adopters who use a wrap library register a sample preset
// alongside the built-in set the same way the test entry point
// does here.
func TestAnalyzerWrappedReturn(t *testing.T) {
	presets := builtinPresets()
	samplePath := filepath.Join("..", "..", "example", "werror.yaml")
	raw, err := os.ReadFile(samplePath)
	assert.MustNoError(t, "read sample preset "+samplePath, err)
	sample, err := dsl.Parse(raw)
	assert.MustNoError(t, "parse sample preset "+samplePath, err)
	analyzer := New(append(presets, sample), nil)
	analysistest.Run(t, analysistest.TestData(), analyzer, "wrappedReturn")
}

// TestAnalyzerClosableConfigOptOut drives the per-directory
// `vow.yaml` config: the closableConfigOptOut fixture ships its
// own vow.yaml whose `closable.built_in_closables` is the empty
// list, so the analyzer's discovery walk finds the file next to
// the package and replaces the embedded built-in closable type
// list with no entries for that scope. The fixture pins silent
// ambient leaks (`*os.File` and an `io.Closer`-constrained type
// parameter) alongside continued enforcement for user-declared
// `vow:define @Closable` types.
func TestAnalyzerClosableConfigOptOut(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "closableConfigOptOut")
}

// TestAnalyzerNarrowScope drives the narrow filter introduced via
// --changed-files / --with-callers. The fixture set runs in two phases:
// the self-scope phase confirms diagnostics inside a changed file surface
// unchanged, and the caller-scope phase confirms the per-call-site filter
// keeps diagnostics whose call site targets a changed-file callee while
// dropping diagnostics whose call site targets an unchanged callee.
// The cleanup resets the package-level cli state so neighbouring tests
// observe the empty default.
func TestAnalyzerNarrowScope(t *testing.T) {
	t.Cleanup(func() { cli.SetActive(cli.ChangedFileSet{}) })
	testDir := analysistest.TestData()

	selfFile := filepath.Join(testDir, "src", "narrowScopeSelf", "narrowScopeSelf.go")
	cli.SetActive(cli.ChangedFileSet{Files: []string{selfFile}})
	analysistest.Run(t, testDir, Analyzer, "narrowScopeSelf")

	callerFile := filepath.Join(testDir, "src", "narrowScopeXPkgCallee", "lib.go")
	cli.SetActive(cli.ChangedFileSet{
		Files:          []string{callerFile},
		WithCallers:    true,
		CallerPackages: []string{"narrowScopeXPkgCaller"},
	})
	analysistest.Run(t, testDir, Analyzer, "narrowScopeXPkgCallee", "narrowScopeXPkgOtherCallee", "narrowScopeXPkgCaller")

	// The same-package phase confirms the per-call-site rule reaches an
	// unchanged file of the changed file's own package. CallerPackages stays
	// empty because the resolver partitions the owning package away from the
	// 1-hop importers; ChangedPackages is what carries it into scope.
	samePkgFile := filepath.Join(testDir, "src", "narrowScopeSamePkg", "lib.go")
	cli.SetActive(cli.ChangedFileSet{
		Files:           []string{samePkgFile},
		WithCallers:     true,
		ChangedPackages: []string{"narrowScopeSamePkg"},
	})
	analysistest.Run(t, testDir, Analyzer, "narrowScopeSamePkg")
}

// TestAnalyzerNilDeclCompleteness drives the declaration-completeness
// rule through its per-directory opt-in: the fixture ships a vow.yaml
// carrying `nil_decl.require_declarations: true`, so the rule runs for
// that package alone. The fixture pins the demand on undeclared
// nillable positions — including the one the empty-parens arity
// exception admits — alongside the exemptions for convention-bearing
// types, type parameters, non-nillable kinds, generated files, and the
// marker shapes that do not describe the enclosing signature.
func TestAnalyzerNilDeclCompleteness(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "nilDeclCompleteness")
}

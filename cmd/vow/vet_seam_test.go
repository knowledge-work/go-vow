package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
)

// TestVetModeMatchesDriverAcrossTheNarrowSeam runs both paths over the
// one scope decision that spans the two processes: a diagnostic in an
// unchanged file that the rule keeps because its callee is declared in a
// changed file. The analyzer spells that callee's file, the driver
// compares it against the changed-file set, and the comparison is exact
// — so a difference in how either side spells the path drops the
// diagnostic with nothing to show for it.
func TestVetModeMatchesDriverAcrossTheNarrowSeam(t *testing.T) {
	binary := buildVowBinary(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "narrowseam"))
	assert.MustNoError(t, "resolve fixture", err)
	changed := filepath.Join(dir, "p", "a.go")
	const config = "nil_decl:\n  require_declarations: false\n"

	driver := runVowBinary(t, binary, dir,
		"--config-yaml", config, "--changed-files", changed, "--with-callers", "./...")
	vet := runVowBinary(t, binary, dir,
		"--vet-mode", "--config-yaml", config, "--changed-files", changed, "--with-callers", "./...")

	// The positive control: a run that reported nothing would compare
	// equal to another run that reported nothing, so the comparison only
	// means something once the kept diagnostic is present.
	if !strings.Contains(driver, "may be nil through flow") {
		t.Fatalf("the default path did not report the kept diagnostic, so the comparison proves nothing:\n%s", driver)
	}
	if driver != vet {
		t.Fatalf("paths disagree across the seam\ndefault:\n%s\nvet mode:\n%s", driver, vet)
	}
}

// buildVowBinary builds this command into a temporary directory. The vet
// path re-enters the binary as `go vet`'s tool, which a test binary
// cannot stand in for.
func buildVowBinary(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("no go command available: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "vow")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build vow: %v\n%s", err, out)
	}
	return binary
}

// runVowBinary runs the built binary in dir and returns everything it
// wrote, diagnostics included. A vow run exits non-zero when it reports,
// so the exit status is not an error here.
func runVowBinary(t *testing.T, binary, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

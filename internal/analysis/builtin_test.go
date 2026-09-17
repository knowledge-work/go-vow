package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
)

// assertEmbeddedMatchesReference pins one embedded preset against its
// repository-root copy. The check compares the two as a boolean rather
// than as values: a drifted preset would otherwise print both YAML
// files into the failure.
func assertEmbeddedMatchesReference(t *testing.T, embedded []byte, refPath ...string) {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, refPath...)...)
	ref, err := os.ReadFile(path)
	assert.MustNoError(t, "read reference preset "+path, err)
	assert.Equal(t, "the embedded copy matches "+path+"; update both files together",
		string(embedded) == string(ref), true)
}

// TestBuiltinMatchesReference guards against drift between the
// repository-root reference preset and the copy embedded into the
// analyzer binary. They are kept byte-identical on purpose: contributors
// should update both files together.
func TestBuiltinMatchesReference(t *testing.T) {
	assertEmbeddedMatchesReference(t, builtinSentinelErrorYAML, "preset", "sentinel-error.yaml")
}

func TestBuiltinStdResultMatchesReference(t *testing.T) {
	assertEmbeddedMatchesReference(t, builtinStdResultYAML, "preset", "std", "result.yaml")
}

func TestBuiltinClosableMatchesReference(t *testing.T) {
	assertEmbeddedMatchesReference(t, builtinClosableYAML, "preset", "closable.yaml")
}

func TestBuiltinPresetsLoadable(t *testing.T) {
	presets := builtinPresets()
	assert.MustLen(t, "builtinPresets()", presets, 2)
	names := map[string]bool{}
	for _, p := range presets {
		names[p.Name] = true
	}
	assert.Equal(t, "the sentinel-error preset is present", names["sentinel-error"], true)
	assert.Equal(t, "the closable preset is present", names["closable"], true)
}

func TestBuiltinStdPresetsLoadable(t *testing.T) {
	std := builtinStdPresets()
	p, ok := std["preset/std/result"]
	assert.MustEqual(t, "preset/std/result is registered", ok, true)
	_, hasOkErr := p.Rules["OkErr"]
	assert.Equal(t, "Rules carries OkErr", hasOkErr, true)
	_, hasEither := p.Rules["Either"]
	assert.Equal(t, "Rules carries Either", hasEither, true)
}

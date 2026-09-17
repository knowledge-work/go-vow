package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestParseEmpty(t *testing.T) {
	cfg, err := Parse(nil)
	assert.MustNoError(t, "Parse(nil)", err)
	assert.Nil(t, "Closable.BuiltinClosables; an empty config must keep the embedded default", cfg.Closable.BuiltinClosables)
}

func TestParseBuiltinClosablesEmpty(t *testing.T) {
	cfg, err := Parse([]byte("closable:\n  built_in_closables: []\n"))
	assert.MustNoError(t, "Parse", err)
	assert.DeepEqual(t, "Closable.BuiltinClosables; an explicit empty list must override the default", cfg.Closable.BuiltinClosables, ptrSlice())
}

func TestParseBuiltinClosablesPartial(t *testing.T) {
	cfg, err := Parse([]byte("closable:\n  built_in_closables:\n    - io.Closer\n    - \"*os.File\"\n"))
	assert.MustNoError(t, "Parse", err)
	assert.DeepEqual(t, "Closable.BuiltinClosables", cfg.Closable.BuiltinClosables, ptrSlice("io.Closer", "*os.File"))
}

func TestParseNilDeclRequireDeclarations(t *testing.T) {
	cfg, err := Parse([]byte("nil_decl:\n  require_declarations: true\n"))
	assert.MustNoError(t, "Parse", err)
	assert.Equal(t, "NilDecl.RequireDeclarations", cfg.NilDecl.RequireDeclarations, true)
}

func TestParseNilDeclDefaultsOff(t *testing.T) {
	cfg, err := Parse([]byte("closable:\n  built_in_closables: []\n"))
	assert.MustNoError(t, "Parse", err)
	assert.Equal(t, "NilDecl.RequireDeclarations with the nil_decl section omitted", cfg.NilDecl.RequireDeclarations, false)
}

// TestParseCallerResolverExcludeSuffixes pins the round-trip of
// the `caller_resolver.exclude_package_suffixes` knob: an absent
// field stays nil (keep the embedded default), an explicit
// non-empty list lands as a *[]string the driver can apply, and
// an empty list disables suffix filtering (the closable schema's
// empty-slice vs nil distinction).
func TestParseCallerResolverExcludeSuffixes(t *testing.T) {
	type excludeSuffixesCase struct {
		raw  string
		want *[]string
	}

	tabletest.Run(t, map[string]excludeSuffixesCase{
		"field absent keeps nil":        {raw: "closable:\n  built_in_closables: []\n", want: nil},
		"populated list lands verbatim": {raw: "caller_resolver:\n  exclude_package_suffixes:\n    - _test\n    - _mock\n", want: ptrSlice("_test", "_mock")},
		"empty list disables filtering": {raw: "caller_resolver:\n  exclude_package_suffixes: []\n", want: ptrSlice()},
	}, func(t *testing.T, c excludeSuffixesCase) {
		cfg, err := Parse([]byte(c.raw))
		assert.MustNoError(t, "Parse", err)
		assert.DeepEqual(t, "ExcludePackageSuffixes", cfg.CallerResolver.ExcludePackageSuffixes, c.want)
	})
}

// TestDefaultExcludePackageSuffixes pins the default the driver
// applies when no override is set. The default is empty because
// the resolver removes synthetic test packages structurally; a
// suffix entry here would silently drop real packages whose
// import path legally ends with that suffix.
func TestDefaultExcludePackageSuffixes(t *testing.T) {
	assert.Len(t, "DefaultExcludePackageSuffixes()", DefaultExcludePackageSuffixes(), 0)
}

// ptrSlice builds a *[]string from variadic args so a want value can
// be written inline.
func ptrSlice(items ...string) *[]string {
	if items == nil {
		items = []string{}
	}
	return &items
}

func TestParseInvalid(t *testing.T) {
	_, err := Parse([]byte("closable: [not a mapping]\n"))
	assert.Error(t, "Parse of a malformed payload", err)
}

// TestParseUnknownKey pins the strict-fields contract: a typo
// (`closables:` instead of `closable:`) must surface as an error
// rather than silently no-op.
func TestParseUnknownKey(t *testing.T) {
	_, err := Parse([]byte("closables:\n  built_in_closables: []\n"))
	assert.ErrorContains(t, "Parse with an unknown top-level key", err, "closables")
}

// TestResolverNearestAncestor exercises the discovery walk on two
// axes — the parent-only fallthrough (a child directory with no
// vow.yaml of its own reads the root vow.yaml) and the nested
// override (a vow.yaml deeper in the tree takes over for its
// scope) — and closes by resolving the root itself through the
// same resolver.
func TestResolverNearestAncestor(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "vow.yaml"), "closable:\n  built_in_closables:\n    - io.Closer\n")
	plainChild := filepath.Join(root, "service", "billing")
	overrideChild := filepath.Join(root, "infra", "stream")
	for _, d := range []string{plainChild, overrideChild} {
		assert.MustNoError(t, "MkdirAll", os.MkdirAll(d, 0o755))
	}
	writeFile(t, filepath.Join(root, "infra", "vow.yaml"), "closable:\n  built_in_closables: []\n")

	r := NewResolver()
	plain, err := r.Resolve(plainChild)
	assert.MustNoError(t, "Resolve(plainChild)", err)
	assert.MustNotNil(t, "Resolve(plainChild); the plain child must inherit the root vow.yaml", plain)
	assert.DeepEqual(t, "Resolve(plainChild).Closable.BuiltinClosables", plain.Closable.BuiltinClosables, ptrSlice("io.Closer"))

	override, err := r.Resolve(overrideChild)
	assert.MustNoError(t, "Resolve(overrideChild)", err)
	assert.MustNotNil(t, "Resolve(overrideChild); the override child must read infra/vow.yaml", override)
	assert.DeepEqual(t, "Resolve(overrideChild).Closable.BuiltinClosables", override.Closable.BuiltinClosables, ptrSlice())

	gotRoot, err := r.Resolve(root)
	assert.MustNoError(t, "Resolve(root)", err)
	assert.MustNotNil(t, "Resolve(root); the root must read root/vow.yaml", gotRoot)
	assert.DeepEqual(t, "Resolve(root).Closable.BuiltinClosables", gotRoot.Closable.BuiltinClosables, ptrSlice("io.Closer"))
}

// TestResolverNoConfig confirms a multi-level tree with no vow.yaml
// on the walk resolves to (nil, nil).
func TestResolverNoConfig(t *testing.T) {
	root := t.TempDir()
	leaf := filepath.Join(root, "mid", "leaf")
	assert.MustNoError(t, "MkdirAll", os.MkdirAll(leaf, 0o755))

	// The walk stops only at the filesystem root, so whatever sits above
	// the tempdir answers for the tree below it too. Resolving the
	// tempdir's parent first makes that a stated condition of the fixture
	// instead of a silent one.
	aboveFixture, err := NewResolver().Resolve(filepath.Dir(root))
	assert.MustNoError(t, "Resolve(the tempdir's parent)", err)
	assert.Nil(t, "Resolve(the tempdir's parent); the assertion below rests on this being nil", aboveFixture)

	// A second resolver, so the leaf walks the whole way instead of
	// stopping at an entry the check above left cached.
	cfg, err := NewResolver().Resolve(leaf)
	assert.NoError(t, "Resolve(leaf)", err)
	assert.Nil(t, "Resolve(leaf); no vow.yaml sits between the leaf and the filesystem root", cfg)
}

func TestResolverRejectsRelative(t *testing.T) {
	_, err := NewResolver().Resolve("relative/path")
	assert.Error(t, "Resolve with a relative starting directory", err)
}

// TestResolverCachesIntermediate pins both the intermediate-store
// contract (every directory the walk passes through holds the
// resolved config afterwards) and the parent-side cache (mutating
// the file after a Resolve must not change what a later Resolve
// returns, whether it starts at a descendant or at one of those
// directories).
func TestResolverCachesIntermediate(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "vow.yaml")
	writeFile(t, configPath, "closable:\n  built_in_closables: []\n")
	siblingA := filepath.Join(root, "a", "leaf")
	siblingB := filepath.Join(root, "b", "leaf")
	intermediateA := filepath.Join(root, "a")
	for _, d := range []string{siblingA, siblingB} {
		assert.MustNoError(t, "MkdirAll", os.MkdirAll(d, 0o755))
	}
	r := NewResolver()
	fromSiblingA, err := r.Resolve(siblingA)
	assert.MustNoError(t, "Resolve(siblingA)", err)
	assert.MustNotNil(t, "Resolve(siblingA); the fixture's root vow.yaml must be found", fromSiblingA)

	// Read the cache rather than a later Resolve's return value: the
	// root entry alone already answers a query on any descendant with
	// the same config, so the return value is the same whether or not
	// the walk recorded anything along the way.
	for _, dir := range []string{siblingA, intermediateA, root} {
		cached, ok := r.lookup(dir)
		assert.Equal(t, "cache entry for "+dir, ok, true)
		assert.Equal(t, "cached config for "+dir, cached, fromSiblingA)
	}

	// Mutate the file so a resolution not answered from the cache,
	// whether it starts at siblingB or at intermediateA, would observe
	// the new contents.
	writeFile(t, configPath, "closable:\n  built_in_closables:\n    - io.Closer\n")

	cfg, err := r.Resolve(siblingB)
	assert.MustNoError(t, "Resolve(siblingB)", err)
	assert.MustNotNil(t, "Resolve(siblingB); the cache must hold the first resolution", cfg)
	assert.DeepEqual(t, "Resolve(siblingB).Closable.BuiltinClosables", cfg.Closable.BuiltinClosables, ptrSlice())

	cachedIntermediate, err := r.Resolve(intermediateA)
	assert.MustNoError(t, "Resolve(intermediateA)", err)
	assert.MustNotNil(t, "Resolve(intermediateA); the intermediate directory must serve its cached entry", cachedIntermediate)
	assert.DeepEqual(t, "Resolve(intermediateA).Closable.BuiltinClosables", cachedIntermediate.Closable.BuiltinClosables, ptrSlice())
}

// TestResolverCachesTerminalDirectory pins the entry the walk
// leaves on the directory it stops at. Every directory below
// inherits that verdict, so no return value tells the entry apart
// from its absence; the cache is where it shows.
func TestResolverCachesTerminalDirectory(t *testing.T) {
	root := t.TempDir()
	leaf := filepath.Join(root, "mid", "leaf")
	assert.MustNoError(t, "MkdirAll", os.MkdirAll(leaf, 0o755))

	// With no vow.yaml in the fixture the walk stops at the filesystem
	// root, but only if nothing above the tempdir carries one either.
	aboveFixture, err := NewResolver().Resolve(filepath.Dir(root))
	assert.MustNoError(t, "Resolve(the tempdir's parent)", err)
	assert.MustNil(t, "Resolve(the tempdir's parent); a config above the tempdir would stop the walk short of the filesystem root", aboveFixture)

	r := NewResolver()
	_, err = r.Resolve(leaf)
	assert.MustNoError(t, "Resolve(leaf)", err)

	cached, ok := r.lookup(filesystemRoot(leaf))
	assert.Equal(t, "cache entry for the filesystem root", ok, true)
	assert.Nil(t, "cached config for the filesystem root", cached)
}

// filesystemRoot returns the ancestor of dir that is its own parent,
// which is where the discovery walk stops when it finds no vow.yaml.
func filesystemRoot(dir string) string {
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

// TestResolverConcurrent pins the documented concurrency contract:
// many goroutines resolving the same directory return the same
// effective config and do not race under -race.
func TestResolverConcurrent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "vow.yaml"), "closable:\n  built_in_closables: []\n")
	leaf := filepath.Join(root, "service", "billing")
	assert.MustNoError(t, "MkdirAll", os.MkdirAll(leaf, 0o755))

	type resolution struct {
		cfg *Config
		err error
	}
	r := NewResolver()
	const goroutines = 16
	var wg sync.WaitGroup
	results := make(chan resolution, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			cfg, err := r.Resolve(leaf)
			results <- resolution{cfg: cfg, err: err}
		}()
	}
	wg.Wait()
	close(results)
	// Assert here rather than inside the goroutines: a Must form in one
	// of them would end that goroutine, not the test, and guard nothing.
	for got := range results {
		assert.MustNoError(t, "concurrent Resolve", got.err)
		assert.MustNotNil(t, "concurrent Resolve's config", got.cfg)
		assert.DeepEqual(t, "concurrent Resolve's Closable.BuiltinClosables", got.cfg.Closable.BuiltinClosables, ptrSlice())
	}
}

// TestResolverIgnoresVowYAMLDirectory pins the !info.IsDir() guard:
// a directory called vow.yaml does not satisfy discovery; the walk
// continues ascending past it.
func TestResolverIgnoresVowYAMLDirectory(t *testing.T) {
	root := t.TempDir()
	assert.MustNoError(t, "Mkdir (vow.yaml as a directory)", os.Mkdir(filepath.Join(root, "vow.yaml"), 0o755))
	cfg, err := NewResolver().Resolve(root)
	assert.MustNoError(t, "Resolve", err)
	assert.Nil(t, "Resolve; a vow.yaml directory must not satisfy discovery", cfg)
}

// TestReadFileMissing pins what ReadFile reports for a path that
// does not exist: an error naming vow.yaml and wrapping
// fs.ErrNotExist.
func TestReadFileMissing(t *testing.T) {
	root := t.TempDir()
	_, err := ReadFile(filepath.Join(root, "does-not-exist.yaml"))
	assert.ErrorContains(t, "ReadFile on a non-existent path", err, "read vow.yaml")
	assert.ErrorIs(t, "ReadFile on a non-existent path", err, fs.ErrNotExist)
}

// TestReadFileEmpty pins the empty-file shortcut: a present but
// zero-byte vow.yaml parses to the zero Config, so every section
// keeps its embedded default.
func TestReadFileEmpty(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "vow.yaml")
	writeFile(t, path, "")
	cfg, err := ReadFile(path)
	assert.MustNoError(t, "ReadFile on an empty file", err)
	assert.DeepEqual(t, "ReadFile's Config for an empty file", cfg, &Config{})
}

// The analysis_scope section's key spelling is part of the config
// surface: a parse through the real decoder pins it against the
// KnownFields check silently renaming it.
func TestParseAnalysisScope(t *testing.T) {
	cfg, err := Parse([]byte("analysis_scope:\n  first_party_prefixes:\n    - example.com/foo\n    - other.example\n"))
	assert.MustNoError(t, "Parse", err)
	assert.DeepEqual(t, "AnalysisScope.FirstPartyPrefixes", cfg.AnalysisScope.FirstPartyPrefixes, []string{"example.com/foo", "other.example"})
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	assert.MustNoError(t, "WriteFile "+path, os.WriteFile(path, []byte(body), 0o644))
}

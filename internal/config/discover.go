package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// FileName is the file the discovery layer looks for at each
// ancestor directory.
const FileName = "vow.yaml"

// Resolver walks the filesystem from a file's package directory up
// to the filesystem root and returns the Config carried by the
// nearest ancestor vow.yaml. The walk semantics are intentionally
// minimal: the nearest vow.yaml is the full source of policy for
// its scope (replace inheritance, not merge), and intermediate
// ancestors that share the same nearest config reuse the cached
// result.
//
// Resolve is safe to call from multiple goroutines. The cache is
// guarded by a sync.RWMutex; a benign race where two goroutines
// both miss the cache on the same directory and both store the
// same parsed *Config is tolerated because the stored value is
// idempotent. The walk uses os.Stat, which follows symlinks.
type Resolver struct {
	mu    sync.RWMutex
	cache map[string]*Config
}

// NewResolver returns a Resolver with an empty cache. Callers
// share a single Resolver across an analyzer run so each directory
// pays the discovery cost only once.
//
// vow:nil () !
func NewResolver() *Resolver {
	return &Resolver{cache: map[string]*Config{}}
}

// Resolve returns the Config in effect for absDir. The function
// returns (nil, nil) when no vow.yaml exists between absDir and
// the filesystem root, indicating that the embedded defaults
// apply unchanged. absDir must already be absolute; relative
// inputs are an error because the discovery walk cannot infer the
// process's working directory safely from a relative starting
// point.
//
// vow:nil () ?,
func (r *Resolver) Resolve(absDir string) (*Config, error) {
	if !filepath.IsAbs(absDir) {
		return nil, fmt.Errorf("config.Resolve requires an absolute directory, got %q", absDir)
	}
	return r.resolve(filepath.Clean(absDir))
}

// vow:nil () ?,
func (r *Resolver) resolve(dir string) (*Config, error) {
	if cfg, ok := r.lookup(dir); ok {
		return cfg, nil
	}
	candidate := filepath.Join(dir, FileName)
	info, err := os.Stat(candidate)
	switch {
	case err == nil && !info.IsDir():
		cfg, readErr := ReadFile(candidate)
		if readErr != nil {
			return nil, readErr
		}
		r.store(dir, cfg)
		return cfg, nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("stat vow.yaml at %s: %w", dir, err)
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		r.store(dir, nil)
		return nil, nil
	}
	cfg, err := r.resolve(parent)
	if err != nil {
		return nil, err
	}
	// Cache the parent's verdict on the intermediate directory so
	// later queries against descendants stop here instead of
	// re-walking the same chain. This shape relies on replace
	// inheritance: two starting points that share the same nearest
	// vow.yaml see identical effective configs, so the cache value
	// is keyed on dir alone. A future merge-style knob would have
	// to key on (dir, walk-chain) instead.
	r.store(dir, cfg)
	return cfg, nil
}

// vow:nil () ?,
func (r *Resolver) lookup(dir string) (*Config, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.cache[dir]
	return cfg, ok
}

// vow:nil (,?)
func (r *Resolver) store(dir string, cfg *Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[dir] = cfg
}

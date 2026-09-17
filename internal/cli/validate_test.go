package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
)

func TestValidateChangedFiles_AbsolutePassthrough(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.go")
	assert.MustNoError(t, "write fixture", os.WriteFile(src, []byte("package a\n"), 0o644))
	got, err := ValidateChangedFiles([]string{src})
	assert.MustNoError(t, "ValidateChangedFiles", err)
	assert.MustLen(t, "ValidateChangedFiles", got, 1)
	assert.Equal(t, "ValidateChangedFiles[0]", got[0], filepath.Clean(src))
}

func TestValidateChangedFiles_RelativeNormalizesToAbsolute(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.go")
	assert.MustNoError(t, "write fixture", os.WriteFile(src, []byte("package a\n"), 0o644))
	cwd, err := os.Getwd()
	assert.MustNoError(t, "getwd", err)
	t.Cleanup(func() {
		if cdErr := os.Chdir(cwd); cdErr != nil {
			t.Logf("restore cwd: %v", cdErr)
		}
	})
	assert.MustNoError(t, "chdir "+dir, os.Chdir(dir))
	got, err := ValidateChangedFiles([]string{"a.go"})
	assert.MustNoError(t, "ValidateChangedFiles", err)
	assert.MustLen(t, "ValidateChangedFiles", got, 1)
	assert.Equal(t, "filepath.IsAbs(ValidateChangedFiles[0])", filepath.IsAbs(got[0]), true)
	assert.Equal(t, "filepath.Base(ValidateChangedFiles[0])", filepath.Base(got[0]), "a.go")
}

func TestValidateChangedFiles_NonexistentRejected(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.go")
	_, err := ValidateChangedFiles([]string{missing})
	assert.ErrorContains(t, "ValidateChangedFiles on a missing file", err, missing)
}

func TestValidateChangedFiles_NonGoRejected(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "README.md")
	assert.MustNoError(t, "write fixture", os.WriteFile(src, []byte("# readme\n"), 0o644))
	_, err := ValidateChangedFiles([]string{src})
	assert.ErrorContains(t, "ValidateChangedFiles on a non-Go path", err, "Go source file")
}

func TestValidateChangedFiles_DirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "pkg.go")
	assert.MustNoError(t, "mkdir", os.Mkdir(subdir, 0o755))
	_, err := ValidateChangedFiles([]string{subdir})
	assert.ErrorContains(t, "ValidateChangedFiles on a directory path", err, "directory")
}

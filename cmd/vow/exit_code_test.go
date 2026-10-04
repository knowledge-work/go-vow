package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/driver"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestExitCodes runs the built binary once per exit code, on both paths
// where the code applies, and checks each run's exit code together with
// the output that explains it.
func TestExitCodes(t *testing.T) {
	binary := buildVowBinary(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "exitcode"))
	assert.MustNoError(t, "resolve fixture", err)
	// --vet-mode requires a config, and the default-path runs get the same
	// one so that both paths analyze alike.
	const config = "nil_decl:\n  require_declarations: false\n"
	type exitCase struct {
		args    []string
		want    int
		wantOut string // text the output must contain; "" requires no output
	}
	tabletest.Run(t, map[string]exitCase{
		"clean package":                {args: []string{"--config-yaml", config, "./clean"}, want: 0},
		"clean package under vet mode": {args: []string{"--vet-mode", "--config-yaml", config, "./clean"}, want: 0},
		"diagnostic": {
			args:    []string{"--config-yaml", config, "./leak"},
			want:    3,
			wantOut: "sentinel error ErrLeak leaked",
		},
		"diagnostic under vet mode": {
			args:    []string{"--vet-mode", "--config-yaml", config, "./leak"},
			want:    3,
			wantOut: "sentinel error ErrLeak leaked",
		},
		"type error": {
			args:    []string{"--config-yaml", config, "./broken"},
			want:    1,
			wantOut: "cannot use",
		},
		"type error under vet mode": {
			args:    []string{"--vet-mode", "--config-yaml", config, "./broken"},
			want:    1,
			wantOut: "cannot use",
		},
		"pattern matching no package": {
			args:    []string{"--config-yaml", config, "vowexitcode.example/none/..."},
			want:    2,
			wantOut: "matched no packages",
		},
		"pattern matching no package under vet mode": {
			args:    []string{"--vet-mode", "--config-yaml", config, "vowexitcode.example/none/..."},
			want:    1,
			wantOut: "matched no packages",
		},
		"pattern naming a missing directory": {
			args:    []string{"--config-yaml", config, "./missing/..."},
			want:    1,
			wantOut: "no such file or directory",
		},
		"pattern naming a missing directory under vet mode": {
			args:    []string{"--vet-mode", "--config-yaml", config, "./missing/..."},
			want:    1,
			wantOut: "no such file or directory",
		},
		"rejected command line": {
			args:    []string{"--with-callers", "./clean"},
			want:    2,
			wantOut: "--with-callers requires --changed-files",
		},
	}, func(t *testing.T, c exitCase) {
		out, code := runVowBinaryExit(t, binary, dir, c.args...)
		if c.wantOut == "" {
			assert.Equal(t, "output", out, "")
		} else {
			assert.Contains(t, "output", out, c.wantOut)
		}
		assert.Equal(t, "exit code", code, c.want)
	})
}

// TestExitCodesWhenGoCannotListPackages runs every path in modules the
// go command refuses to list. The files are written at run time so that
// no tool walking testdata trips over them.
func TestExitCodesWhenGoCannotListPackages(t *testing.T) {
	binary := buildVowBinary(t)
	const config = "nil_decl:\n  require_declarations: false\n"
	type moduleCase struct {
		files   map[string]string
		wantOut string
	}
	tabletest.Run(t, map[string]moduleCase{
		"broken go.mod": {
			files:   map[string]string{"go.mod": "module broken.example\n\nrequire (\n"},
			wantOut: "errors parsing go.mod",
		},
	}, func(t *testing.T, m moduleCase) {
		dir := t.TempDir()
		for name, content := range m.files {
			assert.MustNoError(t, "write "+name, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
		}
		changed := filepath.Join(dir, "p.go")
		assert.MustNoError(t, "write p.go", os.WriteFile(changed, []byte("package p\n"), 0o600))
		type listCase struct {
			args []string
			want int
		}
		tabletest.Run(t, map[string]listCase{
			"default path": {args: []string{"--config-yaml", config, "./..."}, want: 2},
			"vet mode":     {args: []string{"--vet-mode", "--config-yaml", config, "./..."}, want: 1},
			// The resolver lists the packages before go vet runs, so vet mode
			// stops there too.
			"with callers":                {args: []string{"--config-yaml", config, "--changed-files", changed, "--with-callers"}, want: 2},
			"with callers under vet mode": {args: []string{"--vet-mode", "--config-yaml", config, "--changed-files", changed, "--with-callers"}, want: 2},
		}, func(t *testing.T, c listCase) {
			out, code := runVowBinaryExit(t, binary, dir, c.args...)
			assert.Contains(t, "output", out, m.wantOut)
			assert.Equal(t, "exit code", code, c.want)
		})
	})
}

// TestVetModeExitCodesOnUnusableReports puts a fake go command first on
// PATH, because the real one never hands vow a report it cannot read or
// an analyzer error for a package that type-checks.
func TestVetModeExitCodesOnUnusableReports(t *testing.T) {
	binary := buildVowBinary(t)
	type reportCase struct {
		report  string // what the fake go vet writes to stdout
		wantOut string
	}
	tabletest.Run(t, map[string]reportCase{
		"unreadable report": {report: "not json", wantOut: "vow: read go vet report"},
		"analyzer error":    {report: `{"p":{"vow":{"error":"boom"}}}`, wantOut: "vow: vow failed on p: boom"},
	}, func(t *testing.T, c reportCase) {
		fakeDir := t.TempDir()
		reportFile := filepath.Join(fakeDir, "report")
		assert.MustNoError(t, "write report", os.WriteFile(reportFile, []byte(c.report), 0o600))
		script := "#!/bin/sh\ncat '" + reportFile + "'\n"
		assert.MustNoError(t, "write fake go", os.WriteFile(filepath.Join(fakeDir, "go"), []byte(script), 0o700))
		t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, code := runVowBinaryExit(t, binary, fakeDir, "--vet-mode", "--config-yaml", "{}", "./...")
		assert.Contains(t, "output", out, c.wantOut)
		assert.Equal(t, "exit code", code, 1)
	})
}

// TestExitCodesWhenCallerResolutionStops runs --with-callers over
// modules whose code stops the resolver before the analysis starts.
func TestExitCodesWhenCallerResolutionStops(t *testing.T) {
	binary := buildVowBinary(t)
	type stopCase struct {
		changed string // p/p.go, the changed file
		other   string // q/q.go
		wantOut string
		want    int
	}
	tabletest.Run(t, map[string]stopCase{
		"another package fails to load": {
			changed: "package p\n\nfunc P() int { return 1 }\n",
			other:   "packge q\n",
			wantOut: "prevent complete caller resolution",
			want:    2,
		},
		// q has to import p: without a caller the resolver returns before
		// it parses the changed file, and the analysis reports the error.
		"changed file does not parse": {
			changed: "package p\n\nfunc P() int { return 1 +\n}\n",
			other:   "package q\n\nimport \"callers.example/p\"\n\nvar _ = p.P\n",
			wantOut: "parse changed files",
			want:    2,
		},
	}, func(t *testing.T, c stopCase) {
		// The go command reports files under the resolved path, and a
		// changed file spelled through a symlink (macOS's /var) owns no
		// package.
		dir, err := filepath.EvalSymlinks(t.TempDir())
		assert.MustNoError(t, "resolve temp dir", err)
		files := map[string]string{
			"go.mod": "module callers.example\n\ngo 1.25\n",
			"p/p.go": c.changed,
			"q/q.go": c.other,
		}
		for name, content := range files {
			path := filepath.Join(dir, name)
			assert.MustNoError(t, "mkdir", os.MkdirAll(filepath.Dir(path), 0o700))
			assert.MustNoError(t, "write "+name, os.WriteFile(path, []byte(content), 0o600))
		}
		out, code := runVowBinaryExit(t, binary, dir, "--changed-files", filepath.Join(dir, "p", "p.go"), "--with-callers")
		assert.Contains(t, "output", out, c.wantOut)
		assert.Equal(t, "exit code", code, c.want)
	})
}

func TestResolveExitCode(t *testing.T) {
	type exitCodeCase struct {
		jobs       []*driver.Job
		loadErrors int
		want       int
	}
	reported := []analysis.Diagnostic{{Message: "reported"}}
	tabletest.Run(t, map[string]exitCodeCase{
		"no jobs":                           {want: 0},
		"root without diagnostics":          {jobs: []*driver.Job{{Root: true}}, want: 0},
		"diagnostic on a dependency only":   {jobs: []*driver.Job{{Diagnostics: reported}}, want: 0},
		"diagnostic on a root":              {jobs: []*driver.Job{{Root: true, Diagnostics: reported}}, want: 3},
		"analyzer error":                    {jobs: []*driver.Job{{Err: errors.New("analyzer failed")}}, want: 1},
		"load error":                        {loadErrors: 1, want: 1},
		"error alongside a root diagnostic": {jobs: []*driver.Job{{Root: true, Diagnostics: reported}, {Err: errors.New("analyzer failed")}}, want: 1},
	}, func(t *testing.T, c exitCodeCase) {
		assert.Equal(t, "resolveExitCode", resolveExitCode(&driver.Result{Jobs: c.jobs}, c.loadErrors), c.want)
	})
}

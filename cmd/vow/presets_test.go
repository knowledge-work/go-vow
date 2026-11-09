package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/config"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// presetModule is a module whose package declares a concept that only
// presets/miss.yaml gives behaviour to, so the preset's diagnostic
// shows whether a run loaded the file.
var presetModule = map[string]string{
	"go.mod": "module presets.example\n\ngo 1.25\n",
	"p/p.go": "package p\n\nimport \"errors\"\n\n// vow:define @Miss\nvar ErrMiss = errors.New(\"miss\")\n\nfunc Leak() error { return ErrMiss }\n",
	"presets/miss.yaml": "name: miss\nsubjects:\n  - match: \"annotation:vow:define @Miss\"\n" +
		"obligations:\n  - type: must-consume\n    detail:\n      consumers: [errors.Is]\n",
}

// writeModule writes files under dir, creating the directories they
// sit in.
func writeModule(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		assert.MustNoError(t, "mkdir for "+name, os.MkdirAll(filepath.Dir(path), 0o755))
		assert.MustNoError(t, "write "+name, os.WriteFile(path, []byte(content), 0o600))
	}
}

// TestLoadedPresetsReachEveryPath runs the built binary over
// presetModule with the preset named through each way vow takes a
// config, under the default driver and under --vet-mode, next to a run
// that names no preset and a config naming a file that does not exist.
func TestLoadedPresetsReachEveryPath(t *testing.T) {
	binary := buildVowBinary(t)
	const leak = "vow[miss]: sentinel error ErrMiss leaked"
	type presetPathCase struct {
		files   map[string]string // written next to presetModule
		args    []string
		want    int
		wantOut string
	}
	tabletest.Run(t, map[string]presetPathCase{
		"discovered vow.yaml": {
			files: map[string]string{"vow.yaml": "presets: [presets/miss.yaml]\n"},
			args:  []string{"./..."},
			want:  1, wantOut: leak,
		},
		"--config-file resolving against the file's directory": {
			files: map[string]string{"conf/vow.yaml": "presets: [../presets/miss.yaml]\n"},
			args:  []string{"--config-file", "conf/vow.yaml", "./..."},
			want:  1, wantOut: leak,
		},
		"--config-yaml resolving against the working directory": {
			args: []string{"--config-yaml", "presets: [presets/miss.yaml]", "./..."},
			want: 1, wantOut: leak,
		},
		"--vet-mode with --config-file": {
			files: map[string]string{"conf/vow.yaml": "presets: [../presets/miss.yaml]\n"},
			args:  []string{"--vet-mode", "--config-file", "conf/vow.yaml", "./..."},
			want:  1, wantOut: leak,
		},
		"--vet-mode with --config-yaml": {
			args: []string{"--vet-mode", "--config-yaml", "presets: [presets/miss.yaml]", "./..."},
			want: 1, wantOut: leak,
		},
		"no preset named": {
			args: []string{"./..."},
			want: 1, wantOut: "vow:define @Miss: concept declared but no behavior registered",
		},
		"--config-file naming a missing preset": {
			files: map[string]string{"conf/vow.yaml": "presets: [../presets/none.yaml]\n"},
			args:  []string{"--config-file", "conf/vow.yaml", "./..."},
			want:  2, wantOut: "none.yaml: no such file or directory",
		},
		"--vet-mode naming a missing preset": {
			args: []string{"--vet-mode", "--config-yaml", "presets: [presets/none.yaml]", "./..."},
			want: 2, wantOut: "none.yaml: no such file or directory",
		},
	}, func(t *testing.T, c presetPathCase) {
		dir := t.TempDir()
		writeModule(t, dir, presetModule)
		writeModule(t, dir, c.files)
		out, code := runVowBinaryExit(t, binary, dir, c.args...)
		assert.Contains(t, "output", out, c.wantOut)
		assert.Equal(t, "exit code", code, c.want)
		if c.wantOut != leak {
			assert.NotContains(t, "output", out, "leaked")
		}
	})
}

// TestVetModeRereadsAnEditedPreset runs --vet-mode twice over one
// module, renaming the preset in between. The go command keys a
// cached result on the flags the vettool received, so the second run
// reports the new name only if the preset's content is among them.
func TestVetModeRereadsAnEditedPreset(t *testing.T) {
	binary := buildVowBinary(t)
	dir := t.TempDir()
	writeModule(t, dir, presetModule)
	args := []string{"--vet-mode", "--config-yaml", "presets: [presets/miss.yaml]", "./..."}

	first, _ := runVowBinaryExit(t, binary, dir, args...)
	assert.MustContains(t, "first run", first, "vow[miss]: sentinel error ErrMiss leaked")
	edited := map[string]string{"presets/miss.yaml": "name: renamed\n" + presetModule["presets/miss.yaml"][len("name: miss\n"):]}
	writeModule(t, dir, edited)
	second, _ := runVowBinaryExit(t, binary, dir, args...)
	assert.Contains(t, "second run", second, "vow[renamed]: sentinel error ErrMiss leaked")
}

// TestDirectGoVetFailsOnUnloadedPresets runs go vet with vow as the
// vettool, a config, and a presets flag that disagree on how many
// preset files there are, as when the config names a file the vettool
// cannot read and nothing passes its content. The run fails rather
// than reporting with presets the config does not name.
func TestDirectGoVetFailsOnUnloadedPresets(t *testing.T) {
	binary := buildVowBinary(t)
	presetsFlag := "-vow.presets=" + vetPresetsJSON(&config.Config{LoadedPresets: []config.LoadedPreset{{Source: []byte(presetModule["presets/miss.yaml"])}}})
	type mismatchCase struct {
		flags []string
		want  string
	}
	tabletest.Run(t, map[string]mismatchCase{
		"config names a file the flag does not carry": {
			flags: []string{"-vow.config=presets: [presets/miss.yaml]"},
			want:  "the config names 1 preset files and -vow.presets carries 0",
		},
		"flag carries a file the config does not name": {
			flags: []string{"-vow.config=nil_decl: {}", presetsFlag},
			want:  "the config names 0 preset files and -vow.presets carries 1",
		},
	}, func(t *testing.T, c mismatchCase) {
		dir := t.TempDir()
		writeModule(t, dir, presetModule)
		args := append(append([]string{"vet", "-vettool=" + binary}, c.flags...), "./...")
		vet := exec.Command("go", args...)
		vet.Dir = dir
		vet.Env = append(os.Environ(), "GOFLAGS=", vetToolEnv+"=")
		out, err := vet.CombinedOutput()
		assert.Error(t, "go vet", err)
		assert.Contains(t, "go vet output", string(out), c.want)
	})
}

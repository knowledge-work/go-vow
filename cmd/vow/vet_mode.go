package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/unitchecker"

	"github.com/knowledge-work/go-vow/internal/analysis"
	"github.com/knowledge-work/go-vow/internal/cli"
	"github.com/knowledge-work/go-vow/internal/config"
	"github.com/knowledge-work/go-vow/internal/seq"
)

// vetConfigFlag is the flag name the analyzer publishes under `go vet`.
// The go command prefixes it with the analyzer's name, so a vettool
// invocation spells it `-vow.config`.
const vetConfigFlag = "config"

// vetPresetsFlag is the flag that carries the content of the preset
// files the config names, as a JSON list of strings. The go command
// prefixes it as it does vetConfigFlag.
const vetPresetsFlag = "presets"

// vetToolEnv marks the child side of a --vet-mode run. The go command
// hands its own environment to the vettool, so a marker set on the `go
// vet` process reaches this binary when it is re-entered as the tool.
const vetToolEnv = "VOW_VETTOOL"

// goToolEnv is set by the go command on every tool it runs for a
// package, a vettool included, with or without -toolexec.
const goToolEnv = "TOOLEXEC_IMPORTPATH"

// vetAnnotateFlag is the flag that asks the analyzer to attach the
// narrow-scope inputs to each diagnostic. The value is the same for
// every run, so it does not divide the go command's cache the way the
// changed-file set itself would.
const vetAnnotateFlag = "narrow-annotate"

// runsAsVettool reports whether the go command invoked this binary as a
// vettool rather than a person invoking the driver. The whole protocol
// is three shapes: a request for the flag list, a request for the build
// identity, and a unit config file to analyze. marked says the
// invocation carries the wrapper's environment marker, and fromGo says
// it carries the variable the go command sets on the tools it runs.
//
// The unit config is recognised by its suffix, which a driver run can
// also carry — `--config-file policy.cfg` names such a path. The suffix
// therefore only decides when the marker is present, or when the
// invocation carries no driver flag at all.
//
// The suffix also needs marked or fromGo. A unit config names the files
// the analysis creates or overwrites, so one a person passes in could
// come from whoever wrote the file. fromGo is what keeps a direct
// `go vet -vettool=vow` working, which sets no marker.
//
// A help flag without the marker means a person, since go vet never asks
// for help. It wins only where parseDriverArgs reads it as a flag: before
// "--" and before any vettool flag, which the parser would reject first.
//
// vow:nil (?,,)
func runsAsVettool(args []string, marked, fromGo bool) bool {
	flagsEnd := slices.IndexFunc(args, func(arg string) bool { return arg == "--" || isVettoolFlag(arg) })
	if flagsEnd < 0 {
		flagsEnd = len(args)
	}
	if !marked && slices.ContainsFunc(args[:flagsEnd], isHelpFlag) {
		return false
	}
	if slices.ContainsFunc(args, isVettoolFlag) {
		return true
	}
	if !marked && hasDriverFlag(args) {
		return false
	}
	if !marked && !fromGo {
		return false
	}
	return slices.ContainsFunc(args, func(arg string) bool {
		return strings.HasSuffix(arg, ".cfg")
	})
}

// isVettoolFlag reports whether arg asks for the flag list or the build
// identity, the two vettool shapes that are flags.
//
// vow:nil ()
func isVettoolFlag(arg string) bool {
	return arg == "-flags" || strings.HasPrefix(arg, "-V=")
}

// hasDriverFlag reports whether args carry a two-dash vow driver flag.
// The go command forwards flags to a vettool with one dash, so a two-dash
// flag can only come from a person. The one-dash driver flags (-test,
// -json) do not decide: the go command forwards -json itself.
//
// vow:nil (?)
func hasDriverFlag(args []string) bool {
	return slices.ContainsFunc(args, func(arg string) bool {
		return strings.HasPrefix(arg, "--")
	})
}

// runVettool analyzes one unit under `go vet` and exits.
//
// The config arrives as the value of the config flag rather than
// through the per-directory vow.yaml walk. The go command keys a
// cached vettool result on the flags the tool received, and a file the
// tool opens itself is not part of that key, so a config read from
// disk would leave an edited policy answering with the previous run's
// verdict. Passing a non-nil Config also switches the per-package
// discovery off, which is what keeps that promise structural rather
// than a matter of remembering.
//
// The preset files the config names arrive the same way, as content on
// the presets flag. A config naming a different number of files than
// that flag carries fails every package: running with the built-in
// presets alone would report a verdict for a policy the run did not
// apply.
func runVettool() {
	cfg := new(config.Config)
	analyzer := analysis.NewDefault(cfg)
	analyzer.Flags.Var(&vetConfigValue{cfg: cfg}, vetConfigFlag, "vow.yaml content applied to every package in the run")
	analyzer.Flags.Var(vetPresetsValue{cfg: cfg}, vetPresetsFlag, "the content of the preset files the config names, as a JSON list of strings")
	analyzer.Flags.Var(annotateNarrowValue{}, vetAnnotateFlag, "attach the narrow-scope inputs to each diagnostic instead of applying the scope")
	run := analyzer.Run
	analyzer.Run = func(pass *goanalysis.Pass) (any, error) {
		if len(cfg.LoadedPresets) != len(cfg.Presets) {
			return nil, fmt.Errorf("the config names %d preset files and -vow.%s carries %d; vow --vet-mode passes their content", len(cfg.Presets), vetPresetsFlag, len(cfg.LoadedPresets))
		}
		return run(pass)
	}
	unitchecker.Main(analyzer)
}

// annotateNarrowValue publishes the narrow-annotation switch as the
// flag is parsed, which is before the first package runs. The analyzer
// reads it through the same process-wide handoff that carries the
// changed-file set.
type annotateNarrowValue struct{}

// String returns the flag's value for the go command's flag listing.
func (annotateNarrowValue) String() string { return "" }

// IsBoolFlag lets the flag be spelled without a value.
func (annotateNarrowValue) IsBoolFlag() bool { return true }

// Set publishes the switch.
func (annotateNarrowValue) Set(value string) error {
	on, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	cli.SetAnnotateNarrow(on)
	return nil
}

// vetConfigValue decodes the config flag into the Config the analyzer
// already closed over, so the decoded policy is in place before the
// first package runs.
type vetConfigValue struct {
	cfg  *config.Config
	text string
}

// String returns the flag's current value. The go command reports it
// when describing the tool's flags.
func (v *vetConfigValue) String() string {
	if v == nil {
		return ""
	}
	return v.text
}

// Set decodes yaml and replaces the analyzer's Config in place. The
// presets the presets flag loaded stay, since config.Parse loads none.
func (v *vetConfigValue) Set(yaml string) error {
	parsed, err := config.Parse([]byte(yaml))
	if err != nil {
		return err
	}
	parsed.LoadedPresets = v.cfg.LoadedPresets
	*v.cfg = *parsed
	v.text = yaml
	return nil
}

// vetPresetsValue parses the presets flag into the LoadedPresets of the
// Config the analyzer closed over.
type vetPresetsValue struct {
	cfg *config.Config
}

// String returns "": the go command reports the value when describing
// the tool's flags, and the content would only repeat the files.
func (vetPresetsValue) String() string { return "" }

// Set decodes the JSON list and parses each entry as a preset file,
// rejecting two that declare one name as the driver does.
func (v vetPresetsValue) Set(text string) error {
	var sources []string
	if err := json.Unmarshal([]byte(text), &sources); err != nil {
		return err
	}
	loaded, err := seq.ChainOf(sources...).MapOrErr(func(source string) (config.LoadedPreset, error) {
		preset, err := config.ParsePreset([]byte(source))
		return config.LoadedPreset{Source: []byte(source), Preset: preset}, err
	}).ToSlice()
	if err != nil {
		return err
	}
	if err := config.CheckPresetNames(loaded); err != nil {
		return err
	}
	v.cfg.LoadedPresets = loaded
	return nil
}

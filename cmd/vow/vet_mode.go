package main

import (
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis/unitchecker"

	"github.com/knowledge-work/go-vow/internal/analysis"
	"github.com/knowledge-work/go-vow/internal/cli"
	"github.com/knowledge-work/go-vow/internal/config"
)

// vetConfigFlag is the flag name the analyzer publishes under `go vet`.
// The go command prefixes it with the analyzer's name, so a vettool
// invocation spells it `-vow.config`.
const vetConfigFlag = "config"

// vetToolEnv marks the child side of a --vet-mode run. The go command
// hands its own environment to the vettool, so a marker set on the `go
// vet` process reaches this binary when it is re-entered as the tool.
const vetToolEnv = "VOW_VETTOOL"

// vetAnnotateFlag is the flag that asks the analyzer to attach the
// narrow-scope inputs to each diagnostic. The value is the same for
// every run, so it does not divide the go command's cache the way the
// changed-file set itself would.
const vetAnnotateFlag = "narrow-annotate"

// runsAsVettool reports whether the go command invoked this binary as a
// vettool rather than a person invoking the driver. The whole protocol
// is three shapes: a request for the flag list, a request for the build
// identity, and a unit config file to analyze. marked says the
// invocation carries the wrapper's environment marker.
//
// The unit config is recognised by its suffix, which a driver run can
// also carry — `--config-file policy.cfg` names such a path. The suffix
// therefore only decides when the marker is present, or when the
// invocation carries no driver flag at all, which is what keeps a
// direct `go vet -vettool=vow` working.
//
// vow:nil (?,)
func runsAsVettool(args []string, marked bool) bool {
	if slices.ContainsFunc(args, func(arg string) bool {
		return arg == "-flags" || strings.HasPrefix(arg, "-V=")
	}) {
		return true
	}
	if !marked && hasDriverFlag(args) {
		return false
	}
	return slices.ContainsFunc(args, func(arg string) bool {
		return strings.HasSuffix(arg, ".cfg")
	})
}

// hasDriverFlag reports whether args carry a vow driver flag. Every
// driver flag is spelled with two dashes and every flag the go command
// forwards to a vettool with one, so the prefix separates the two
// callers without naming each flag.
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
func runVettool() {
	cfg := new(config.Config)
	analyzer := analysis.NewDefault(cfg)
	analyzer.Flags.Var(&vetConfigValue{cfg: cfg}, vetConfigFlag, "vow.yaml content applied to every package in the run")
	analyzer.Flags.Var(annotateNarrowValue{}, vetAnnotateFlag, "attach the narrow-scope inputs to each diagnostic instead of applying the scope")
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

// Set decodes yaml and replaces the analyzer's Config in place.
func (v *vetConfigValue) Set(yaml string) error {
	parsed, err := config.Parse([]byte(yaml))
	if err != nil {
		return err
	}
	*v.cfg = *parsed
	v.text = yaml
	return nil
}

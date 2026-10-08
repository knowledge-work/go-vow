package main

import (
	"slices"
	"strings"

	"github.com/knowledge-work/go-vow/internal/seq"
)

// cliDocsURL is the page that describes each flag in full.
const cliDocsURL = "https://github.com/knowledge-work/go-vow/blob/main/docs/cli.md"

// flagSpec describes one flag vow takes on its command line.
type flagSpec struct {
	// names lists every name vow accepts for the flag. The first is the
	// canonical one: parseDriverArgs switches on it and the flag column
	// of docs/cli.md lists it; the same row names the others as doing the
	// same.
	names []string
	// arg is the placeholder for the flag's value, or "" for a flag
	// that takes none.
	arg   string
	usage string
	// vettool marks a flag go vet passes when it runs vow as its vettool.
	// runsAsVettool takes such a flag before parseDriverArgs runs, and
	// parseDriverArgs rejects it.
	vettool bool
}

// flagSpecs is the one list of the flags vow takes, in the order the
// docs list them. -h prints it, parseDriverArgs accepts exactly these
// names apart from the vettool flags, and a test checks that the flag
// tables of docs/cli.md match it.
var flagSpecs = []flagSpec{
	{names: []string{"-json", "--json"}, usage: "print the report on stdout as JSON instead of as text on stderr"},
	{names: []string{"-test", "--test"}, usage: "include _test.go files in the analysis"},
	{names: []string{"-h", "-help", "--help"}, usage: "print this help and exit"},
	{names: []string{"--changed-files"}, arg: "<file>...", usage: "restrict the run to the files that follow, up to the next flag or --"},
	{names: []string{"--with-callers"}, usage: "with --changed-files, lint the changed packages and their direct callers"},
	{names: []string{"--config-file"}, arg: "<path>", usage: "read the configuration from this file instead of a vow.yaml per package"},
	{names: []string{"--config-yaml"}, arg: "<content>", usage: "read the configuration from this string instead of a vow.yaml per package"},
	{names: []string{"--vet-mode"}, usage: "use go vet and its build cache; requires --config-file or --config-yaml"},
	{names: []string{"-V=full"}, usage: "print a build ID (a hash of the vow binary) and exit", vettool: true},
	{names: []string{"-flags"}, usage: "print as JSON the flags vow takes as a go vet vettool, not this list", vettool: true},
}

// driverFlagNamed returns the spec that has name among its names, and
// false if there is none or it is a vettool flag.
//
// vow:nil () ,
func driverFlagNamed(name string) (flagSpec, bool) {
	return seq.ChainOf(flagSpecs...).Find(func(spec flagSpec) bool {
		return !spec.vettool && slices.Contains(spec.names, name)
	})
}

// isHelpFlag reports whether arg is one of the names of -h.
//
// vow:nil ()
func isHelpFlag(arg string) bool {
	spec, known := driverFlagNamed(arg)
	return known && spec.names[0] == "-h"
}

// usageText returns what -h prints: the synopsis, each flag with its
// usage on the line below, and a pointer to docs/cli.md.
//
// vow:nil ()
func usageText() string {
	entries := seq.ChainOf(flagSpecs...).Map(func(spec flagSpec) string {
		names := strings.Join(spec.names, ", ")
		if spec.arg != "" {
			names += " " + spec.arg
		}
		return "  " + names + "\n      " + spec.usage + "\n"
	}).ToSlice()
	return "usage: vow [flags] [--] [packages...]\n\n" +
		strings.Join(entries, "") +
		"\nEach flag is described in full at\n  " + cliDocsURL + "\n"
}

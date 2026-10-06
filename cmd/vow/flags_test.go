package main

import (
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/seq"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// flagRowPattern matches a row of a flag table in docs/cli.md and
// captures the flag cell.
var flagRowPattern = regexp.MustCompile("^\\| `(-[^`]*)` \\|")

// aliasSentencePattern matches the sentence of a flag row that names the
// flag's other names, as in "`-help` and `--help` do the same.".
var aliasSentencePattern = regexp.MustCompile("((?:`-[^`]+`(?:, and |, | and )?)+) do(?:es)? the same\\.")

// quotedFlagPattern captures each flag a piece of docs/cli.md quotes.
var quotedFlagPattern = regexp.MustCompile("`(-[^`]+)`")

// TestFlagSpecsMatchTheDocs checks the flag tables of docs/cli.md
// against flagSpecs: the flag column lists exactly each spec's first name
// and value placeholder, in the order of flagSpecs, and each row names
// exactly the spec's other names as doing the same. It reads every flag
// row in the file, so a table in a new section is checked too.
func TestFlagSpecsMatchTheDocs(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli.md"))
	assert.MustNoError(t, "read docs/cli.md", err)
	rows := seq.NewChain(strings.Lines(string(raw))).
		FilterMap(func(line string) (flagRow, bool) {
			match := flagRowPattern.FindStringSubmatch(line)
			if match == nil {
				return flagRow{}, false
			}
			return flagRow{flag: match[1], line: line}, true
		}).
		ToSlice()
	documented := seq.ChainOf(rows...).Map(func(row flagRow) string { return row.flag }).ToSlice()
	specified := seq.ChainOf(flagSpecs...).Map(func(spec flagSpec) string {
		return strings.TrimSpace(spec.names[0] + " " + spec.arg)
	}).ToSlice()
	assert.MustDeepEqual(t, "flags in docs/cli.md", documented, specified)
	for i, spec := range flagSpecs {
		assert.DeepEqual(t, "other names in the docs/cli.md row of "+spec.names[0],
			otherNamesIn(rows[i].line), spec.names[1:])
	}
}

// flagRow is one row of a flag table in docs/cli.md.
type flagRow struct {
	flag, line string
}

// otherNamesIn returns the flags a docs/cli.md row names in its "does the
// same" sentence, in the order it names them, or an empty slice.
func otherNamesIn(line string) []string {
	sentence := aliasSentencePattern.FindStringSubmatch(line)
	if sentence == nil {
		return []string{}
	}
	return seq.ChainOf(quotedFlagPattern.FindAllStringSubmatch(sentence[1], -1)...).
		Map(func(match []string) string { return match[1] }).
		ToSlice()
}

// TestEveryFlagSpecIsAccepted checks that each name in flagSpecs reaches
// a handler: runsAsVettool for a vettool flag, and otherwise a case of
// parseDriverArgs that either changes the result or reports an error
// other than an unknown flag (--with-callers without --changed-files does
// the latter). A name with no case would leave the result untouched.
func TestEveryFlagSpecIsAccepted(t *testing.T) {
	tabletest.Run(t, namedFlagSpecs(), func(t *testing.T, c namedFlagSpec) {
		if c.spec.vettool {
			assert.Equal(t, "runsAsVettool", runsAsVettool([]string{c.name}, false), true)
			return
		}
		args := []string{c.name}
		if c.spec.arg != "" {
			args = append(args, "value")
		}
		flags, _, err := parseDriverArgs(args)
		if err != nil {
			assert.NotContains(t, "parseDriverArgs error", err.Error(), "unknown flag")
			return
		}
		assert.Equal(t, "parseDriverArgs left the flags zero", reflect.ValueOf(flags).IsZero(), false)
	})
}

// TestHelpPrintsTheUsage runs the built binary, since main rather than
// parseDriverArgs prints the usage.
func TestHelpPrintsTheUsage(t *testing.T) {
	binary := buildVowBinary(t)
	tabletest.Run(t, map[string][]string{
		"-h":                           {"-h"},
		"-help":                        {"-help"},
		"--help":                       {"--help"},
		"beside a vettool flag":        {"-h", "-flags"},
		"after a bad flag combination": {"--with-callers", "--help"},
	}, func(t *testing.T, args []string) {
		run := runVowBinaryStreams(t, binary, t.TempDir(), args...)
		assert.Equal(t, "stdout", run.stdout, usageText())
		assert.Equal(t, "stderr", run.stderr, "")
		assert.Equal(t, "exit code", run.code, 0)
	})
}

// TestUsageTextListsEveryFlag checks that the help gives each spec its
// names, value placeholder and usage.
func TestUsageTextListsEveryFlag(t *testing.T) {
	text := usageText()
	for _, spec := range flagSpecs {
		entry := strings.TrimSpace(strings.Join(spec.names, ", ") + " " + spec.arg)
		assert.Contains(t, "usageText", text, "\n  "+entry+"\n      "+spec.usage+"\n")
	}
}

// namedFlagSpec pairs one name of a flag with its spec.
type namedFlagSpec struct {
	name string
	spec flagSpec
}

func namedFlagSpecs() map[string]namedFlagSpec {
	return seq.ChainOf(flagSpecs...).
		FlatMap(func(spec flagSpec) iter.Seq[namedFlagSpec] {
			return seq.ChainOf(spec.names...).
				Map(func(name string) namedFlagSpec { return namedFlagSpec{name: name, spec: spec} }).
				ToSeq()
		}).
		ToMap(func(c namedFlagSpec) (string, namedFlagSpec) { return c.name, c })
}

// flagsQuotedButNotTaken lists the dash-prefixed names docs/cli.md quotes
// that are not vow flags: the singlechecker flags it says vow lacks, the
// bare -V it rejects, and the "--" separator.
var flagsQuotedButNotTaken = []string{"-c", "-fix", "-V", "--"}

// TestDocsQuoteOnlyTheFlagsVowTakes checks that every flag docs/cli.md
// quotes, in a table or in prose, is a name in flagSpecs or one the page
// names as not taken, so a name the parser rejects cannot be documented
// in any wording.
func TestDocsQuoteOnlyTheFlagsVowTakes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli.md"))
	assert.MustNoError(t, "read docs/cli.md", err)
	taken := seq.ChainOf(flagSpecs...).
		FlatMap(func(spec flagSpec) iter.Seq[string] { return slices.Values(spec.names) }).
		ToSlice()
	unknown := seq.ChainOf(quotedFlagPattern.FindAllStringSubmatch(string(raw), -1)...).
		Map(func(match []string) string { return strings.Fields(match[1])[0] }).
		Filter(func(name string) bool {
			return !slices.Contains(taken, name) && !slices.Contains(flagsQuotedButNotTaken, name)
		}).
		ToSlice()
	assert.DeepEqual(t, "flags docs/cli.md quotes that vow does not take", unknown, nil)
}

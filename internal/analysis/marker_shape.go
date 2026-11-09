package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"iter"
	"strings"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/seq"
)

// markerPrefix starts every marker the analyzer reads.
const markerPrefix = "vow:"

// markerShape is how the analyzer reads one marker: which bytes may
// follow the marker name (the end of the line always may), and
// whether the reader goes through CommentGroup.Text. That method
// unwraps /* */ comments but drops a `//vow:...` line written with no
// space after the slashes, because it takes the line for a directive
// such as //go:generate.
type markerShape struct {
	marker     string
	separators string
	groupText  bool
	// hint tells the author how to write the marker when the byte
	// after the name is not one of separators.
	hint string
}

const (
	hintPayloadSpace = "put a space between the marker and its payload"
	hintNoPayload    = "the marker takes no payload"
)

// markerShapes lists every marker the analyzer reads.
var markerShapes = []markerShape{
	{marker: rulesMarker, separators: " \t[", hint: hintPayloadSpace},
	{marker: conceptDefineMarker, separators: " \t", groupText: true, hint: hintPayloadSpace},
	{marker: dischargedMarker, separators: " ", groupText: true, hint: hintNoPayload},
	{marker: emitMarker, separators: " \t[", hint: hintPayloadSpace},
	{marker: importMarker, separators: " ", hint: hintPayloadSpace},
	{marker: nilDeclMarker, separators: " \t[", hint: hintPayloadSpace},
	{marker: readonlyMarker, hint: hintNoPayload},
	{marker: suppressMarker, separators: ":", hint: "write vow:suppress: <reason>"},
	{marker: useHintMarker, separators: " \t[", hint: hintPayloadSpace},
}

// commentForm is how the comment holding a marker line is written.
type commentForm int

const (
	// lineComment is `// vow:...`.
	lineComment commentForm = iota
	// directiveComment is `//vow:...`, with no space after the slashes.
	directiveComment
	// blockComment is `/* vow:... */`.
	blockComment
)

// reportMalformedMarkers reports every comment line that starts with
// vow: but that no marker reader picks up: an unknown marker name, a
// known name followed by a byte its reader does not accept, or a
// comment form its reader skips. Each reader ignores such a line, so
// without this the author would hold a contract that reads as
// enforced while nothing checks it.
func reportMalformedMarkers(pass *analysis.Pass) {
	for _, file := range pass.Files {
		for _, group := range file.Comments {
			for _, c := range group.List {
				reportMalformedMarkersIn(pass, c)
			}
		}
	}
}

// reportMalformedMarkersIn checks each line of c that starts with
// vow:. The diagnostic anchors at the marker itself so a multi-line
// /* */ comment points at the offending line.
func reportMalformedMarkersIn(pass *analysis.Pass, c *ast.Comment) {
	block := strings.HasPrefix(c.Text, "/*")
	for at := range markerLineStarts(c.Text) {
		line, _, _ := strings.Cut(c.Text[at:], "\n")
		form := lineComment
		switch {
		case block:
			form = blockComment
			line, _, _ = strings.Cut(line, "*/")
		case at == len("//"):
			form = directiveComment
		}
		if message, ok := malformedMarkerMessage(strings.TrimSpace(line), form); ok {
			vowReportf(pass, c.Pos()+token.Pos(at), "vow[annotation]: %s", message)
		}
	}
}

// markerLineStarts yields the offset in text of every vow: that only
// whitespace precedes on its line, after the // or /* opener.
func markerLineStarts(text string) iter.Seq[int] {
	return func(yield func(int) bool) {
		for from := 0; ; {
			idx := strings.Index(text[from:], markerPrefix)
			if idx < 0 {
				return
			}
			at := from + idx
			from = at + len(markerPrefix)
			lineStart := max(strings.LastIndexByte(text[:at], '\n')+1, len("//"))
			if strings.TrimSpace(text[lineStart:at]) != "" {
				continue
			}
			if !yield(at) {
				return
			}
		}
	}
}

// malformedMarkerMessage returns what is wrong with line, a trimmed
// comment line that starts with vow:, or ok=false when a reader
// accepts it. A line whose vow: is not followed by a letter is prose
// rather than a marker and stays silent.
func malformedMarkerMessage(line string, form commentForm) (message string, ok bool) {
	rest := line[len(markerPrefix):]
	nameLen := strings.IndexFunc(rest, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z')
	})
	if nameLen < 0 {
		nameLen = len(rest)
	}
	if nameLen == 0 {
		return "", false
	}
	marker := markerPrefix + rest[:nameLen]
	shape, known := seq.ChainOf(markerShapes...).Find(func(s markerShape) bool { return s.marker == marker })
	if !known {
		names := seq.ChainOf(markerShapes...).Map(func(s markerShape) string { return s.marker }).ToSlice()
		return fmt.Sprintf("%s is not a marker the analyzer reads; the markers are %s", marker, strings.Join(names, ", ")), true
	}
	switch {
	case form == blockComment && !shape.groupText:
		return fmt.Sprintf("%s in a /* */ comment is not read; write it in a // comment", marker), true
	case form == directiveComment && shape.groupText:
		return fmt.Sprintf("//%s with no space after the slashes is read as a directive and dropped; write // %s", marker, marker), true
	}
	after := rest[nameLen:]
	if after == "" {
		return "", false
	}
	next, _ := utf8.DecodeRuneInString(after)
	if strings.ContainsRune(shape.separators, next) {
		return "", false
	}
	return fmt.Sprintf("%s followed by %q is not read as the marker; %s", marker, next, shape.hint), true
}

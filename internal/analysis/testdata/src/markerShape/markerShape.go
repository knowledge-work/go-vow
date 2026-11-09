// Package markerShape is the analysistest fixture for comment lines
// that start with vow: but that no marker reader picks up. Each such
// line is reported at the marker; the well-formed lines next to them
// stay silent and keep their contract in force.
package markerShape

import "errors"

type T struct{ V int }

// Valid is the well-formed control: its contract is read, so the nil
// argument in Calls is reported.
//
// vow:nil (!)
func Valid(t *T) int { return t.V } // want Valid:"nilDecl\\(!\\)"

// NoSpaceAfterSlashes is read as well: the readers of vow:nil strip
// the slashes before matching the marker.
//
//vow:nil (!)
func NoSpaceAfterSlashes(t *T) int { return t.V } // want NoSpaceAfterSlashes:"nilDecl\\(!\\)"

// vow:nill (!) // want `vow\[annotation\]: vow:nill is not a marker the analyzer reads; the markers are vow:cond, vow:define, vow:discharged, vow:emit, vow:import, vow:nil, vow:readonly, vow:suppress, vow:use`
func Misspelt(t *T) int { return t.V }

// vow:foo (!) // want `vow\[annotation\]: vow:foo is not a marker the analyzer reads`
func Unknown(t *T) int { return t.V }

// vow:nil(!) // want `vow\[annotation\]: vow:nil followed by '\(' is not read as the marker; put a space between the marker and its payload`
func NoSpaceBeforePayload(t *T) int { return t.V }

/* vow:nil (!) */    // want `vow\[annotation\]: vow:nil in a /\* \*/ comment is not read; write it in a // comment`
func Block(t *T) int { return t.V }

/*
BlockLastLine carries the marker on a later line of the comment.
vow:cond * -> nil */       // want `vow\[annotation\]: vow:cond in a /\* \*/ comment is not read`
func BlockLastLine() error { return nil }

/* vow:define @Sentinel */
var ErrBlockDefine = errors.New("block define") // want ErrBlockDefine:"subject\\(vow:define @Sentinel\\)"

//vow:define @Sentinel // want `vow\[annotation\]: //vow:define with no space after the slashes is read as a directive and dropped; write // vow:define`
var ErrDirectiveDefine = errors.New("directive define")

// The define in a /* */ comment is read, so its leak is reported; the
// one with no space after the slashes is not, so its leak is not.
func LeakBlockDefine() error { return ErrBlockDefine } // want `sentinel error ErrBlockDefine leaked`

func LeakDirectiveDefine() error { return ErrDirectiveDefine }

// The spacing between the define marker and its concept does not
// matter, so this leak is reported too.
//
// vow:define  @Sentinel
var ErrTwoSpaces = errors.New("two spaces") // want ErrTwoSpaces:"subject\\(vow:define @Sentinel\\)"

func LeakTwoSpaces() error { return ErrTwoSpaces } // want `sentinel error ErrTwoSpaces leaked`

// A tab after vow:import is not read, so it is reported.
//
// vow:import	result "preset/std/result" // want `vow\[annotation\]: vow:import followed by '\\t' is not read as the marker`
func ImportTab() {}

// Prose that mentions vow:nill in the middle of a line stays silent,
// and so does a line where vow: is not followed by a marker name:
// vow: the analyzer.
func Prose() {}

func Calls() {
	_ = Valid(nil)               // want `argument 1 \(t\) is nil`
	_ = NoSpaceAfterSlashes(nil) // want `argument 1 \(t\) is nil`
	_ = Misspelt(nil)
	_ = Unknown(nil)
	_ = NoSpaceBeforePayload(nil)
	_ = Block(nil)
}

func Suppressed() {
	// vow:suppress: the reason follows the colon
	_ = Valid(&T{})
	// vow:suppress the colon is missing // want `vow\[annotation\]: vow:suppress followed by ' ' is not read as the marker; write vow:suppress: <reason>`
	_ = Valid(&T{})
}

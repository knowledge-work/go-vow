// Package closableStdHandle pins the exemption for the handles the
// os package exposes as package variables. The process owns
// os.Stdout, os.Stderr and os.Stdin, so a read of one carries no
// close obligation and none of the four flows the analyzer watches
// may report it. The leaking halves of the fixture use a handle
// from os.Open, which is acquired at its call site and must keep
// reporting: without them the fixture could not tell the exemption
// apart from the check being off.
package closableStdHandle

import (
	"io"
	"os"
)

// exemptCallArgument hands a std handle to a non-Closable interface
// parameter, the shape a logger constructor takes.
func exemptCallArgument() {
	consumeWriter(os.Stdout)
}

// exemptReturn returns a std handle through a non-Closable
// interface result.
func exemptReturn() io.Writer {
	return os.Stderr
}

// exemptAssign assigns a std handle to a non-Closable interface
// variable that already exists.
func exemptAssign() {
	var w io.Writer
	w = os.Stdout
	_ = w
}

// exemptAcquisition binds a std handle with a short variable
// declaration. The binding names a handle the process owns, so the
// analyzer must not demand a Close.
func exemptAcquisition() {
	f := os.Stdin
	_ = f
}

// leakingAcquisition opens a file and never closes it. This is the
// control: the exemption must not reach a handle acquired here.
func leakingAcquisition() {
	f, _ := os.Open("noop") // want `vow\[closable\]: f is acquired but not closed; add a defer Close call or declare the transfer with vow:emit`
	_ = f
}

// leakingCallArgument hands an opened file to the same non-Closable
// interface parameter exemptCallArgument uses. The two differ only
// in where the handle came from.
func leakingCallArgument() {
	f, _ := os.Open("noop") // want `vow\[closable\]: f is acquired but not closed; add a defer Close call or declare the transfer with vow:emit`
	consumeWriter(f)        // want `vow\[closable\]: passing a Closable value as io.Writer drops the close obligation; the parameter type must also be a Closable interface`
}

func consumeWriter(io.Writer) {}

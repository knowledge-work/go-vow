// Package closableSuppress pins vow:suppress on the Closable checks.
// Each shape the checks report appears twice: once under a suppress
// marker, which must stay silent, and once without, which must keep
// reporting. Without the unmarked halves the fixture could not tell
// the suppression apart from the check being off.
package closableSuppress

import (
	"io"
	"os"
)

// acquisitionSuppressedOnLine leaves an opened file unclosed under a
// trailing marker.
func acquisitionSuppressedOnLine() {
	f, _ := os.Open("noop") // vow:suppress: closed by the caller's cleanup hook
	_ = f
}

// acquisitionReported is the unmarked twin of
// acquisitionSuppressedOnLine.
func acquisitionReported() {
	f, _ := os.Open("noop") // want `vow\[closable\]: f is acquired but not closed`
	_ = f
}

// argumentSuppressedOnLine passes a Closable parameter to a
// non-Closable interface parameter under a leading marker.
func argumentSuppressedOnLine(f *os.File) {
	// vow:suppress: the reader only borrows the file
	consumeReader(f)
}

// argumentReported is the unmarked twin of argumentSuppressedOnLine.
func argumentReported(f *os.File) {
	consumeReader(f) // want `vow\[closable\]: passing a Closable value as io.Reader drops the close obligation`
}

// returnSuppressedInFunction returns a Closable parameter through a
// non-Closable interface result under a function-level marker.
//
// vow:suppress: the caller owns the file and closes it
func returnSuppressedInFunction(f *os.File) io.Reader {
	return f
}

// returnReported is the unmarked twin of returnSuppressedInFunction.
func returnReported(f *os.File) io.Reader {
	return f // want `vow\[closable\]: returning a Closable value as io.Reader drops the close obligation`
}

// assignSuppressedOnLine assigns a Closable parameter to a
// non-Closable interface variable under a trailing marker.
func assignSuppressedOnLine(f *os.File) {
	var r io.Reader
	r = f // vow:suppress: the variable only borrows the file
	_ = r
}

// assignReported is the unmarked twin of assignSuppressedOnLine.
func assignReported(f *os.File) {
	var r io.Reader
	r = f // want `vow\[closable\]: assigning a Closable value to a io.Reader drops the close obligation`
	_ = r
}

// emitMalformedUnderSuppress keeps a diagnostic about the vow:emit
// marker itself reporting under a function-level marker, which drops
// only the acquisition and upcast diagnostics.
//
// vow:suppress: the caller's cleanup hook closes the file
func emitMalformedUnderSuppress() {
	f, _ := os.Open("noop")
	// vow:emit f // want `vow\[closable\]: vow:emit destination could not be inferred`
	_ = f
}

func consumeReader(io.Reader) {}

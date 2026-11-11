// Package closableDeferredBorrow pins the call-argument upcast for a
// value the function closes itself with a deferred Close. The call
// only borrows the value, so no close obligation is dropped and the
// analyzer stays silent. Each reporting function differs from
// copyAfterDeferredClose in one point: no deferred Close, a deferred
// Close on a different value, a deferred Close that never runs when
// the function returns, or a flow other than a call argument.
package closableDeferredBorrow

import (
	"io"
	"os"
)

// copyAfterDeferredClose is the io.Copy idiom: open, defer the Close,
// then read through io.Reader.
func copyAfterDeferredClose(dst io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(dst, f)
	return err
}

// borrowParameterAfterDeferredClose closes a parameter it was handed
// and lends it to a call first.
func borrowParameterAfterDeferredClose(f *os.File) {
	defer f.Close()
	consumeReader(f)
}

// borrowWithoutClose passes a parameter the function never closes.
func borrowWithoutClose(f *os.File) {
	consumeReader(f) // want `vow\[closable\]: passing a Closable value as io.Reader drops the close obligation`
}

// borrowWhileClosingAnother defers the Close of a different value
// than the one it passes.
func borrowWhileClosingAnother(f, g *os.File) {
	defer g.Close()
	consumeReader(f) // want `vow\[closable\]: passing a Closable value as io.Reader drops the close obligation`
}

// borrowAfterReassign defers the Close of the first file and passes
// the second, which the deferred Close does not reach.
func borrowAfterReassign(f, g *os.File) {
	defer f.Close()
	f = g
	consumeReader(f) // want `vow\[closable\]: passing a Closable value as io.Reader drops the close obligation`
}

// borrowAfterRedeclare defers the Close of the first file and
// redeclares the variable with the second through `:=`.
func borrowAfterRedeclare(f *os.File, path string) error {
	defer f.Close()
	f, err := os.Open(path)
	consumeReader(f) // want `vow\[closable\]: passing a Closable value as io.Reader drops the close obligation`
	return err
}

// borrowShadowedName defers the Close of f and passes a closure
// parameter that shadows the name, which the deferred Close does not
// reach.
func borrowShadowedName(f, g *os.File) {
	defer f.Close()
	func(f *os.File) {
		consumeReader(f) // want `vow\[closable\]: passing a Closable value as io.Reader drops the close obligation`
	}(g)
}

// borrowWithCloseInUncalledClosure defers the Close inside a function
// literal, which the exemption ignores.
func borrowWithCloseInUncalledClosure(f *os.File) {
	cleanup := func() { defer f.Close() }
	_ = cleanup
	consumeReader(f) // want `vow\[closable\]: passing a Closable value as io.Reader drops the close obligation`
}

// returnAfterDeferredClose hands the value on instead of passing it to
// a call.
func returnAfterDeferredClose(f *os.File) io.Reader {
	defer f.Close()
	return f // want `vow\[closable\]: returning a Closable value as io.Reader drops the close obligation`
}

// borrowAfterRangeAssign defers the Close of the first file and then
// reuses the variable as the iteration variable of a range loop.
func borrowAfterRangeAssign(f *os.File, files []*os.File) {
	defer f.Close()
	for _, f = range files {
		consumeReader(f) // want `vow\[closable\]: passing a Closable value as io.Reader drops the close obligation`
	}
}

// borrowAfterAddressTaken defers the Close of f and hands its address
// to a call that replaces the value.
func borrowAfterAddressTaken(f, g *os.File) {
	defer f.Close()
	replaceFile(&f, g)
	consumeReader(f) // want `vow\[closable\]: passing a Closable value as io.Reader drops the close obligation`
}

// sharedFile is a package-level file another function can replace.
var sharedFile *os.File

// borrowPackageVariable defers the Close of a package-level variable,
// which the call before the borrow replaces.
func borrowPackageVariable(g *os.File) {
	defer sharedFile.Close()
	replaceShared(g)
	consumeReader(sharedFile) // want `vow\[closable\]: passing a Closable value as io.Reader drops the close obligation`
}

func replaceFile(p **os.File, g *os.File) { *p = g }

func replaceShared(g *os.File) { sharedFile = g }

func consumeReader(io.Reader) {}

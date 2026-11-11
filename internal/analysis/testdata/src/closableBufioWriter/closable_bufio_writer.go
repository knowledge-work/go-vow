// Package closableBufioWriter checks that *bufio.Writer is not a
// built-in Closable type. It has no Close method, so a defer Close
// cannot discharge it. The *os.File that is never closed shows the
// built-in list is still in effect.
package closableBufioWriter

import (
	"bufio"
	"io"
	"os"
)

// flushedWriter wraps a writer, writes, and flushes.
func flushedWriter(w io.Writer) error {
	bw := bufio.NewWriter(w)
	_, _ = bw.WriteString("x")
	return bw.Flush()
}

// leakingFile opens a file and never closes it.
func leakingFile() {
	f, _ := os.Open("noop") // want `vow\[closable\]: f is acquired but not closed`
	_ = f
}

// Package lifecycle_channel is the analysistest fixture for the
// channel hand-off form of the Closable lifecycle. The author
// sends the Closable down a channel that owns the close and
// annotates the acquisition site with `// vow:emit c` so the
// destination inference returns the Channel case and the
// analyzer accepts the hand-off as a recognised discharge.
package lifecycle_channel

// vow:define @Closable
type Conn struct{}

func (c *Conn) Close() error { return nil }

func openConnection() *Conn { return &Conn{} }

// silentBaseline hands the connection off through a channel send.
// The `vow:emit c` marker sits next to the send statement so the
// destination inference returns the Channel case and the analyzer
// accepts the lifecycle.
func silentBaseline(out chan<- *Conn) {
	c := openConnection()
	out <- c // vow:emit c
}

// diagnoseLeak acquires the Closable but neither defers a Close
// nor hands the value off through a recognised destination. The
// analyzer surfaces a diagnostic at the acquisition site so the
// missing discharge does not go unreported.
func diagnoseLeak(out chan<- *Conn) {
	c := openConnection() // want `vow\[closable\]: c is acquired but not closed; add a defer Close call or declare the transfer with vow:emit`
	_ = out
	_ = c
}

// Package lifecycle_async is the analysistest fixture for the
// goroutine hand-off form of the Closable lifecycle. The author
// launches a goroutine that owns the close and annotates the
// acquisition site with `// vow:emit c` so the analyzer
// recognises the hand-off and clears the discharge obligation.
package lifecycle_async

// vow:define @Closable
type Conn struct{}

func (c *Conn) Close() error { return nil }

func openConnection() *Conn { return &Conn{} }

// silentBaseline hands the connection off to a goroutine that
// owns the close. The `vow:emit c` marker sits next to the
// launching go statement so the destination inference returns the
// goroutine case and the analyzer accepts the lifecycle.
func silentBaseline() {
	c := openConnection()
	go func() { // vow:emit c
		defer c.Close()
		_ = c
	}()
}

// diagnoseLeak launches a goroutine that observes the connection
// but never closes it, and the author forgot the vow:emit marker.
// The analyzer surfaces a diagnostic at the acquisition.
func diagnoseLeak() {
	c := openConnection() // want `vow\[closable\]: c is acquired but not closed; add a defer Close call or declare the transfer with vow:emit`
	go func() {
		_ = c
	}()
}

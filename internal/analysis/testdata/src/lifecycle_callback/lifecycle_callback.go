// Package lifecycle_callback is the analysistest fixture for the
// callback hand-off form of the Closable lifecycle. The author
// registers a callback that owns the close and annotates the
// acquisition site with `// vow:emit c` so the analyzer
// recognises the hand-off and clears the discharge obligation.
package lifecycle_callback

// vow:define @Closable
type Conn struct{}

func (c *Conn) Close() error { return nil }

func openConnection() *Conn { return &Conn{} }

func registerCallback(handler func()) {}

// silentBaseline hands the connection off to a callback that owns
// the close. The `vow:emit c` marker sits next to the call
// expression whose argument is a callable, so destination
// inference returns the callback case and the analyzer accepts
// the lifecycle.
func silentBaseline() {
	c := openConnection()
	registerCallback(func() { // vow:emit c
		_ = c.Close()
	})
}

// diagnoseLeak registers a callback without the vow:emit marker
// so the analyzer treats the acquisition as undischarged and
// surfaces a diagnostic at the acquisition site.
func diagnoseLeak() {
	c := openConnection() // want `vow\[closable\]: c is acquired but not closed; add a defer Close call or declare the transfer with vow:emit`
	registerCallback(func() {
		_ = c
	})
}

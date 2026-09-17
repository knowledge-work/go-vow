// Package lifecycle_defer is the analysistest fixture for the
// defer-discharge form of the Closable lifecycle. The author
// acquires a Closable value and discharges it with `defer
// c.Close()` in the same function. Without the defer the analyzer
// surfaces a vow[closable] diagnostic at the acquisition.
package lifecycle_defer

// vow:define @Closable
type Conn struct{}

func (c *Conn) Close() error { return nil }

func openConnection() *Conn { return &Conn{} }

// silentBaseline discharges the acquisition with `defer c.Close()`
// so the analyzer accepts the lifecycle and emits no diagnostic.
func silentBaseline() {
	c := openConnection()
	defer c.Close()
	_ = c
}

// diagnoseLeak acquires the Closable but never discharges it.
// The analyzer surfaces a diagnostic at the acquisition site.
func diagnoseLeak() {
	c := openConnection() // want `vow\[closable\]: c is acquired but not closed; add a defer Close call or declare the transfer with vow:emit`
	_ = c
}

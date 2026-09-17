// Package lifecycle_storage is the analysistest fixture for the
// storage hand-off form of the Closable lifecycle. The author
// writes the Closable value into shared state (a struct field or
// a slice/map element) and annotates the acquisition site with
// `// vow:emit c` so the analyzer recognises the hand-off and
// clears the discharge obligation.
package lifecycle_storage

// vow:define @Closable
type Conn struct{}

func (c *Conn) Close() error { return nil }

func openConnection() *Conn { return &Conn{} }

type pool struct {
	connections map[string]*Conn
}

// silentBaseline hands the connection off to a shared-state field
// (a map entry). The `vow:emit c` marker sits next to the
// assignment so destination inference returns the storage case
// and the analyzer accepts the lifecycle.
func silentBaseline(p *pool, id string) {
	c := openConnection()
	p.connections[id] = c // vow:emit c
}

// diagnoseLeak writes the connection into the pool without the
// `vow:emit` marker. The analyzer surfaces a diagnostic at the
// acquisition site because no recognised discharge covers the
// lifecycle.
func diagnoseLeak(p *pool, id string) {
	c := openConnection() // want `vow\[closable\]: c is acquired but not closed; add a defer Close call or declare the transfer with vow:emit`
	p.connections[id] = c
}

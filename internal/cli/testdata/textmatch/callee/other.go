package callee

// NewDefault lives outside callee.go so referencing it alone does
// not mention any identifier declared in callee.go.
func NewDefault() *Widget { return &Widget{} }

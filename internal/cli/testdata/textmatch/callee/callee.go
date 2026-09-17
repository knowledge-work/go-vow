package callee

// Widget is the declaration whose field name (not type name) the
// caller_field package references.
type Widget struct{ Field int }

// New returns an empty Widget.
func New() *Widget { return &Widget{} }

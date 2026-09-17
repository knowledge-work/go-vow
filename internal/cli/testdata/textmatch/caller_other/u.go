package caller_other

import "example.com/textmatch/callee"

// W imports the callee package but never mentions an identifier
// declared in callee.go.
var W = callee.NewDefault()

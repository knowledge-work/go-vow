package caller_new

import "example.com/textmatch/callee"

// W references an identifier declared in the changed file.
var W = callee.New()

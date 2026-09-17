package caller_field

import "example.com/textmatch/callee"

// F references only the struct member declared in the changed file;
// the producer function lives in the package's other file.
var F = callee.NewDefault().Field

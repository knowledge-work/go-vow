package p

// CallFromUnchangedFile calls into the changed file from a file the
// narrow run does not name. The scope rule keeps this diagnostic
// because the callee is declared in a changed file, which is the one
// decision that needs the two processes to spell that file the same
// way.
//
// vow:nil (?)
func CallFromUnchangedFile(q *int) {
	Require(q)
}

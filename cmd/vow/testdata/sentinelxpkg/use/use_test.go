package use_test

import (
	"testing"

	"sentinelxpkg/decl"
)

// fetchInTest returns the sentinel from an external test package, which
// a -test run analyses as a package of its own.
func fetchInTest() error {
	return decl.ErrGone
}

func TestFetch(t *testing.T) {
	_ = fetchInTest()
}

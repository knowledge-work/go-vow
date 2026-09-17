package assert

import "fmt"

// recorder is a TB that records what an assertion reported instead of
// failing a test, so the assertions' own messages and stopping
// behaviour can be asserted. It cannot see which frame Helper marked,
// which is why TestAttributionAgainstRealT uses a real *testing.T.
type recorder struct {
	msgs    []string
	stopped bool
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.msgs = append(r.msgs, fmt.Sprintf(format, args...))
}

// FailNow returns, where the real one ends the test's goroutine. That
// is safe because no assertion runs anything after reporting a failure.
func (r *recorder) FailNow() {
	r.stopped = true
}

// wrapped is an error that wraps another, for exercising the wrap chain
// errors.Is walks.
type wrapped struct {
	inner error
}

func (w wrapped) Error() string { return "wrapped: " + w.inner.Error() }

func (w wrapped) Unwrap() error { return w.inner }

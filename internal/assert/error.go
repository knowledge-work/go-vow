package assert

import (
	"errors"
	"strconv"
	"strings"
)

// NoError reports a failure when got is not nil.
// vow:nil (!,,)
func NoError(tb TB, label string, got error) {
	tb.Helper()
	if got != nil {
		tb.Errorf("%s: unexpected error: %s", label, renderError(got))
	}
}

// MustNoError is NoError, and stops the test when got is not nil.
// vow:nil (!,,)
func MustNoError(tb TB, label string, got error) {
	tb.Helper()
	NoError(fatal{tb}, label, got)
}

// Error reports a failure when got is nil.
// vow:nil (!,,)
func Error(tb TB, label string, got error) {
	tb.Helper()
	if got == nil {
		tb.Errorf("%s: no error, want an error", label)
	}
}

// MustError is Error, and stops the test when got is nil.
// vow:nil (!,,)
func MustError(tb TB, label string, got error) {
	tb.Helper()
	Error(fatal{tb}, label, got)
}

// ErrorIs reports a failure when got does not match want under
// errors.Is, which walks got's wrap chain.
// Two distinct errors carrying the same message report alike here, and
// no formatting separates them: the difference is identity, not value.
// vow:nil (!,,,)
func ErrorIs(tb TB, label string, got, want error) {
	tb.Helper()
	if !errors.Is(got, want) {
		tb.Errorf("%s: error = %s, want %s", label, renderError(got), renderError(want))
	}
}

// MustErrorIs is ErrorIs, and stops the test on a mismatch.
// vow:nil (!,,,)
func MustErrorIs(tb TB, label string, got, want error) {
	tb.Helper()
	ErrorIs(fatal{tb}, label, got, want)
}

// ErrorContains reports a failure when got is nil, or when want is not
// a substring of got's message.
// vow:nil (!,,,)
func ErrorContains(tb TB, label string, got error, want string) {
	tb.Helper()
	if got == nil {
		tb.Errorf("%s: no error, want an error containing %s", label, strconv.Quote(want))
		return
	}
	if !strings.Contains(got.Error(), want) {
		tb.Errorf("%s: error = %s, want substring %s", label, renderError(got), strconv.Quote(want))
	}
}

// MustErrorContains is ErrorContains, and stops the test when the error
// is absent or its message does not contain want.
// vow:nil (!,,,)
func MustErrorContains(tb TB, label string, got error, want string) {
	tb.Helper()
	ErrorContains(fatal{tb}, label, got, want)
}

// renderError formats an error for a failure message, naming the nil
// case rather than printing it as a value.
func renderError(err error) string {
	if err == nil {
		return "no error"
	}
	return strconv.Quote(err.Error())
}

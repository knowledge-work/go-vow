// Package assert provides the assertion helpers vow's tests use instead
// of writing out an if statement and a t.Errorf call at every check.
// Each assertion comes in a plain form that records a failure and lets
// the test run on, and a Must form that stops the test on one.
//
// Every assertion takes the test's TB, a label naming the expression
// under test, got, and — where the assertion needs one — want. The
// label leads the failure message, so it reads best as the expression
// that produced got; where got is "loaded" and err is non-nil, these
// report as shown:
//
//	assert.Equal(t, "Name", got, "sentinel-error")
//	// Name = "loaded", want "sentinel-error"
//	assert.MustNoError(t, "Load", err)
//	// Load: unexpected error: "open p.yaml: no such file or directory"
//
// The plain form suits a test checking several independent properties,
// which then reports all of their mismatches in one run. The Must form
// suits the assertions the rest of the test depends on: a load that
// must have succeeded before its result is read, or a length that must
// hold before an index into it.
//
// The Must form stops the goroutine it runs on, not the test: it reaches
// FailNow, which is runtime.Goexit. Called from a goroutine the test
// started, it ends that goroutine and the test keeps running past the
// wait — so a Must there guards nothing the test body reads afterwards.
package assert

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// TB is the part of testing.TB the assertions use, satisfied by
// *testing.T, *testing.B, and *testing.F. testing.TB cannot be
// implemented outside the testing package, so naming just these three
// methods is what lets this package's own tests read back what an
// assertion reported.
type TB interface {
	Helper()
	// vow:nil (,?)
	Errorf(format string, args ...any)
	FailNow()
}

// Equal reports a failure when got and want differ under ==.
// vow:nil (!,,,)
func Equal[T comparable](tb TB, label string, got, want T) {
	tb.Helper()
	if got != want {
		gotText, wantText := renderPair(got, want)
		tb.Errorf("%s = %s, want %s", label, gotText, wantText)
	}
}

// MustEqual is Equal, and stops the test when the values differ.
// vow:nil (!,,,)
func MustEqual[T comparable](tb TB, label string, got, want T) {
	tb.Helper()
	Equal(fatal{tb}, label, got, want)
}

// DeepEqual reports a failure when got and want differ under
// reflect.DeepEqual, for the composite types == is not defined on.
// vow:nil (!,,,)
func DeepEqual[T any](tb TB, label string, got, want T) {
	tb.Helper()
	if !reflect.DeepEqual(got, want) {
		gotText, wantText := renderPair(got, want)
		tb.Errorf("%s = %s, want %s", label, gotText, wantText)
	}
}

// MustDeepEqual is DeepEqual, and stops the test when the values differ.
// vow:nil (!,,,)
func MustDeepEqual[T any](tb TB, label string, got, want T) {
	tb.Helper()
	DeepEqual(fatal{tb}, label, got, want)
}

// Len reports a failure when got does not hold want elements. Slices
// only: a map or string length reads as Equal(tb, "len(x)", len(x),
// want), which keeps the argument typed.
// vow:nil (!,,,)
func Len[S ~[]E, E any](tb TB, label string, got S, want int) {
	tb.Helper()
	if len(got) != want {
		tb.Errorf("len(%s) = %d, want %d (%s = %s)", label, len(got), want, label, render(got))
	}
}

// MustLen is Len, and stops the test on a wrong length.
// vow:nil (!,,,)
func MustLen[S ~[]E, E any](tb TB, label string, got S, want int) {
	tb.Helper()
	Len(fatal{tb}, label, got, want)
}

// Contains reports a failure when the string got does not contain the
// substring want.
// vow:nil (!,,,)
func Contains(tb TB, label, got, want string) {
	tb.Helper()
	if !strings.Contains(got, want) {
		tb.Errorf("%s = %s, want substring %s", label, render(got), render(want))
	}
}

// MustContains is Contains, and stops the test when the substring is
// absent.
// vow:nil (!,,,)
func MustContains(tb TB, label, got, want string) {
	tb.Helper()
	Contains(fatal{tb}, label, got, want)
}

// NotContains reports a failure when the string got contains the
// substring want.
// vow:nil (!,,,)
func NotContains(tb TB, label, got, want string) {
	tb.Helper()
	if strings.Contains(got, want) {
		tb.Errorf("%s = %s, want no substring %s", label, render(got), render(want))
	}
}

// MustNotContains is NotContains, and stops the test when the substring
// is present.
// vow:nil (!,,,)
func MustNotContains(tb TB, label, got, want string) {
	tb.Helper()
	NotContains(fatal{tb}, label, got, want)
}

// Nil reports a failure when got is not nil. A nil pointer, map, slice,
// channel, or func counts as nil even though the interface holding it
// does not compare equal to nil.
// vow:nil (!,,)
func Nil(tb TB, label string, got any) {
	tb.Helper()
	if !isNil(got) {
		tb.Errorf("%s = %s, want nil", label, render(got))
	}
}

// MustNil is Nil, and stops the test when got is not nil.
// vow:nil (!,,)
func MustNil(tb TB, label string, got any) {
	tb.Helper()
	Nil(fatal{tb}, label, got)
}

// NotNil reports a failure when got is nil, counting the same kinds of
// nil as Nil.
// vow:nil (!,,)
func NotNil(tb TB, label string, got any) {
	tb.Helper()
	if isNil(got) {
		tb.Errorf("%s = nil, want non-nil", label)
	}
}

// MustNotNil is NotNil, and stops the test when got is nil.
// vow:nil (!,,)
func MustNotNil(tb TB, label string, got any) {
	tb.Helper()
	NotNil(fatal{tb}, label, got)
}

// fatal turns a TB's failure report into a stopping one. Each Must form
// wraps the test's TB in it and delegates to its plain form, so the two
// cannot drift in what they compare or report.
type fatal struct {
	TB
}

// Errorf reports through the wrapped TB and then stops the test. The
// Helper call marks this frame, so the failure is attributed to the
// assertion's caller rather than to this method.
// vow:nil (,?)
func (f fatal) Errorf(format string, args ...any) {
	f.Helper()
	f.TB.Errorf(format, args...)
	f.TB.FailNow()
}

// renderPair formats got and want for a failure message, falling back
// to %#v when %+v renders them alike. That happens when the difference
// is a nil slice or map against an empty one, or a typed nil against a
// true nil: the values differ but print the same, so the message would
// otherwise read as though nothing were wrong. The fallback does not
// always separate them — values differing only by identity, such as two
// errors carrying the same message, render alike under both verbs.
func renderPair(got, want any) (string, string) {
	gotText, wantText := render(got), render(want)
	if gotText != wantText {
		return gotText, wantText
	}
	return fmt.Sprintf("%#v", got), fmt.Sprintf("%#v", want)
}

// render formats a value for a failure message with its field names.
// Strings are quoted instead, so an empty or space-padded value stays
// visible, and nil is named rather than printed, because %+v prints a
// nil slice or map the same as an empty one. Only the value itself is
// treated that way: %+v still prints a nil slice held in a field the
// same as an empty one, which is what renderPair covers.
func render(v any) string {
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	if isNil(v) {
		return "nil"
	}
	return fmt.Sprintf("%+v", v)
}

// isNil reports whether got holds a nil pointer, map, slice, channel,
// or func. Values of any other kind are never nil.
func isNil(got any) bool {
	if got == nil {
		return true
	}
	switch value := reflect.ValueOf(got); value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

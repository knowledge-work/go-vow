package assert

import (
	"errors"
	"testing"
)

var errProbe = errors.New("probe failure")

func TestNoError(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"nil error passes": {
			run: func(tb TB) { NoError(tb, "Load", nil) },
		},
		"error reports its message": {
			run:  func(tb TB) { NoError(tb, "Load", errProbe) },
			want: `Load: unexpected error: "probe failure"`,
		},
		"Must stops on an error": {
			run:     func(tb TB) { MustNoError(tb, "Load", errProbe) },
			want:    `Load: unexpected error: "probe failure"`,
			stopped: true,
		},
		"Must runs on without an error": {
			run: func(tb TB) { MustNoError(tb, "Load", nil) },
		},
	})
}

func TestError(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"error passes": {
			run: func(tb TB) { Error(tb, "Load", errProbe) },
		},
		"nil error reports the absence": {
			run:  func(tb TB) { Error(tb, "Load", nil) },
			want: "Load: no error, want an error",
		},
		"Must stops without an error": {
			run:     func(tb TB) { MustError(tb, "Load", nil) },
			want:    "Load: no error, want an error",
			stopped: true,
		},
	})
}

func TestErrorIs(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"same error passes": {
			run: func(tb TB) { ErrorIs(tb, "ParseTerm", errProbe, errProbe) },
		},
		"wrapped error passes": {
			run: func(tb TB) { ErrorIs(tb, "ParseTerm", wrapped{errProbe}, errProbe) },
		},
		"unrelated error reports both messages": {
			run: func(tb TB) {
				ErrorIs(tb, "ParseTerm", errors.New("other"), errProbe)
			},
			want: `ParseTerm: error = "other", want "probe failure"`,
		},
		"missing error names the absence": {
			run:  func(tb TB) { ErrorIs(tb, "ParseTerm", nil, errProbe) },
			want: `ParseTerm: error = no error, want "probe failure"`,
		},
		"Must stops on a mismatch": {
			run:     func(tb TB) { MustErrorIs(tb, "ParseTerm", nil, errProbe) },
			want:    `ParseTerm: error = no error, want "probe failure"`,
			stopped: true,
		},
	})
}

func TestErrorContains(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"present substring passes": {
			run: func(tb TB) { ErrorContains(tb, "Load", errProbe, "failure") },
		},
		"absent substring reports the message": {
			run:  func(tb TB) { ErrorContains(tb, "Load", errProbe, "syntax") },
			want: `Load: error = "probe failure", want substring "syntax"`,
		},
		"missing error names the absence": {
			run:  func(tb TB) { ErrorContains(tb, "Load", nil, "syntax") },
			want: `Load: no error, want an error containing "syntax"`,
		},
		"Must stops on an absent substring": {
			run:     func(tb TB) { MustErrorContains(tb, "Load", errProbe, "syntax") },
			want:    `Load: error = "probe failure", want substring "syntax"`,
			stopped: true,
		},
	})
}

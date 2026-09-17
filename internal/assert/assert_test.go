package assert

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// assertionCase drives one assertion against a recorder. want is the
// failure message the assertion is expected to report, or the empty
// string when it is expected to pass; stopped is whether it is expected
// to stop the test.
type assertionCase struct {
	run     func(tb TB)
	want    string
	stopped bool
}

type point struct {
	X int
	Y int
}

// tagged carries a slice field, for the values whose nil-ness %+v hides.
type tagged struct {
	Tags []string
	Name string
}

// probeError is a pointer-receiver error, for building a typed nil that
// an error-typed variable holds without comparing equal to nil.
type probeError struct{}

func (*probeError) Error() string { return "probe failure" }

// sameMessage is an error whose message does not depend on its identity,
// for the pair renderPair cannot separate. It carries a field because two
// pointers to a zero-size type can share an address and compare equal.
type sameMessage struct{ text string }

func (e *sameMessage) Error() string { return e.text }

func TestEqual(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"equal strings pass": {
			run: func(tb TB) { Equal(tb, "Name", "sentinel-error", "sentinel-error") },
		},
		"differing strings are quoted": {
			run:  func(tb TB) { Equal(tb, "Name", "loaded", "sentinel-error") },
			want: `Name = "loaded", want "sentinel-error"`,
		},
		"empty string stays visible": {
			run:  func(tb TB) { Equal(tb, "Name", "", "x") },
			want: `Name = "", want "x"`,
		},
		"differing ints print bare": {
			run:  func(tb TB) { Equal(tb, "len(Subjects)", 2, 1) },
			want: "len(Subjects) = 2, want 1",
		},
		"differing structs print field names": {
			run:  func(tb TB) { Equal(tb, "origin", point{1, 2}, point{0, 0}) },
			want: "origin = {X:1 Y:2}, want {X:0 Y:0}",
		},
		"typed nil is told apart from a true nil": {
			run: func(tb TB) {
				var got error = (*probeError)(nil)
				Equal(tb, "err", got, nil)
			},
			want: "err = (*assert.probeError)(nil), want <nil>",
		},
		"errors differing only by identity still report alike": {
			run: func(tb TB) {
				Equal(tb, "err", error(&sameMessage{"boom"}), error(&sameMessage{"boom"}))
			},
			want: `err = &assert.sameMessage{text:"boom"}, want &assert.sameMessage{text:"boom"}`,
		},
		"Must stops on a difference": {
			run:     func(tb TB) { MustEqual(tb, "Name", "loaded", "sentinel-error") },
			want:    `Name = "loaded", want "sentinel-error"`,
			stopped: true,
		},
		"Must runs on when equal": {
			run: func(tb TB) { MustEqual(tb, "Name", "x", "x") },
		},
	})
}

func TestDeepEqual(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"equal slices pass": {
			run: func(tb TB) {
				DeepEqual(tb, "consumers", []string{"errors.Is"}, []string{"errors.Is"})
			},
		},
		"differing slices print both": {
			run: func(tb TB) {
				DeepEqual(tb, "consumers", []string{"errors.Is"}, []string{"errors.As"})
			},
			want: "consumers = [errors.Is], want [errors.As]",
		},
		"nil and empty slices are told apart": {
			run:  func(tb TB) { DeepEqual(tb, "items", []string{}, nil) },
			want: "items = [], want nil",
		},
		"a nil field is told apart from an empty one": {
			run: func(tb TB) {
				DeepEqual(tb, "cfg", tagged{Name: "x"}, tagged{Tags: []string{}, Name: "x"})
			},
			want: `cfg = assert.tagged{Tags:[]string(nil), Name:"x"}, want assert.tagged{Tags:[]string{}, Name:"x"}`,
		},
		"Must stops on a difference": {
			run:     func(tb TB) { MustDeepEqual(tb, "items", []int{1}, []int{2}) },
			want:    "items = [1], want [2]",
			stopped: true,
		},
	})
}

func TestLen(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"matching length passes": {
			run: func(tb TB) { Len(tb, "Subjects", []string{"a"}, 1) },
		},
		"wrong length names both lengths and the elements": {
			run:  func(tb TB) { Len(tb, "Subjects", []string{"a", "b"}, 1) },
			want: "len(Subjects) = 2, want 1 (Subjects = [a b])",
		},
		"nil slice against a wanted element": {
			run:  func(tb TB) { Len(tb, "diags", []int(nil), 1) },
			want: "len(diags) = 0, want 1 (diags = nil)",
		},
		"Must stops on a wrong length": {
			run:     func(tb TB) { MustLen(tb, "Subjects", []string{"a", "b"}, 1) },
			want:    "len(Subjects) = 2, want 1 (Subjects = [a b])",
			stopped: true,
		},
	})
}

func TestContains(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"present substring passes": {
			run: func(tb TB) { Contains(tb, "output", "found 2 problems", "2 problems") },
		},
		"absent substring reports both": {
			run:  func(tb TB) { Contains(tb, "output", "found 2 problems", "no problems") },
			want: `output = "found 2 problems", want substring "no problems"`,
		},
		"Must stops on an absent substring": {
			run:     func(tb TB) { MustContains(tb, "output", "a", "b") },
			want:    `output = "a", want substring "b"`,
			stopped: true,
		},
	})
}

func TestNotContains(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"absent substring passes": {
			run: func(tb TB) { NotContains(tb, "output", "no diagnostics", "ErrTestOnly") },
		},
		"present substring reports both": {
			run:  func(tb TB) { NotContains(tb, "output", "p/a.go: ErrTestOnly leaks", "ErrTestOnly") },
			want: `output = "p/a.go: ErrTestOnly leaks", want no substring "ErrTestOnly"`,
		},
		"the empty substring is in every string": {
			run:  func(tb TB) { NotContains(tb, "output", "anything", "") },
			want: `output = "anything", want no substring ""`,
		},
		"Must stops on a present substring": {
			run:     func(tb TB) { MustNotContains(tb, "output", "ErrTestOnly", "ErrTestOnly") },
			want:    `output = "ErrTestOnly", want no substring "ErrTestOnly"`,
			stopped: true,
		},
		"Must runs on when the substring is absent": {
			run: func(tb TB) { MustNotContains(tb, "output", "clean", "ErrTestOnly") },
		},
	})
}

func TestNil(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"untyped nil passes": {
			run: func(tb TB) { Nil(tb, "Sum", nil) },
		},
		"typed nil pointer passes": {
			run: func(tb TB) { Nil(tb, "Sum", (*point)(nil)) },
		},
		"nil slice passes": {
			run: func(tb TB) { Nil(tb, "BuiltinClosables", []string(nil)) },
		},
		"empty slice is not nil": {
			run:  func(tb TB) { Nil(tb, "BuiltinClosables", []string{}) },
			want: "BuiltinClosables = [], want nil",
		},
		"non-nil pointer reports the value": {
			run:  func(tb TB) { Nil(tb, "Sum", &point{1, 2}) },
			want: "Sum = &{X:1 Y:2}, want nil",
		},
		"zero of a non-nilable kind is not nil": {
			run:  func(tb TB) { Nil(tb, "count", 0) },
			want: "count = 0, want nil",
		},
		"Must stops on a non-nil value": {
			run:     func(tb TB) { MustNil(tb, "Sum", &point{}) },
			want:    "Sum = &{X:0 Y:0}, want nil",
			stopped: true,
		},
	})
}

func TestNotNil(t *testing.T) {
	runAssertionCases(t, map[string]assertionCase{
		"non-nil pointer passes": {
			run: func(tb TB) { NotNil(tb, "Sum", &point{}) },
		},
		"typed nil pointer fails": {
			run:  func(tb TB) { NotNil(tb, "Sum", (*point)(nil)) },
			want: "Sum = nil, want non-nil",
		},
		"untyped nil fails": {
			run:  func(tb TB) { NotNil(tb, "Sum", nil) },
			want: "Sum = nil, want non-nil",
		},
		"Must stops on a nil value": {
			run:     func(tb TB) { MustNotNil(tb, "Sum", (*point)(nil)) },
			want:    "Sum = nil, want non-nil",
			stopped: true,
		},
	})
}

// TestAttributionAgainstRealT pins that a failure is reported at the
// line that called the assertion, which is what the Helper call in every
// function on the path buys. It runs a failing assertion in a subprocess
// and reads the location back out of the output, because only a real
// *testing.T resolves a location at all: recorder's Helper cannot, so a
// change that keeps the calls and loses the attribution reads the same
// to every other test here.
func TestAttributionAgainstRealT(t *testing.T) {
	if os.Getenv(attributionFixtureVar) == "1" {
		MustEqual(t, "Fixture", "got", "want")
		return
	}

	fixture := exec.Command(os.Args[0], "-test.run=TestAttributionAgainstRealT", "-test.v")
	fixture.Env = append(os.Environ(), attributionFixtureVar+"=1")
	out, err := fixture.CombinedOutput()

	Error(t, "fixture run", err)
	Contains(t, "reported location", string(out), "assert_test.go:")
	for _, inside := range []string{"assert.go:", "error.go:"} {
		NotContains(t, "reported location", string(out), inside)
	}
}

// attributionFixtureVar tells a re-executed test binary to run the
// failing assertion instead of the check around it.
const attributionFixtureVar = "VOW_ASSERT_ATTRIBUTION_FIXTURE"

// runAssertionCases runs each case against a fresh recorder and checks
// the reported message and whether the test was stopped.
func runAssertionCases(t *testing.T, cases map[string]assertionCase) {
	t.Helper()
	tabletest.Run(t, cases, func(t *testing.T, c assertionCase) {
		rec := &recorder{}
		c.run(rec)

		if c.want == "" {
			if len(rec.msgs) != 0 {
				t.Errorf("reported %q, want no failure", strings.Join(rec.msgs, "; "))
			}
		} else {
			if len(rec.msgs) != 1 {
				t.Fatalf("reported %d failures (%q), want 1", len(rec.msgs), strings.Join(rec.msgs, "; "))
			}
			if rec.msgs[0] != c.want {
				t.Errorf("reported %q, want %q", rec.msgs[0], c.want)
			}
		}
		if rec.stopped != c.stopped {
			t.Errorf("stopped = %v, want %v", rec.stopped, c.stopped)
		}
	})
}

package testflag

import (
	"errors"
	"testing"
)

// vow:define @Sentinel
var ErrTestOnly = errors.New("test-only sentinel")

func TestFoo(t *testing.T) {
	if Foo() != 1 {
		t.Fail()
	}
}

func leakedFromTest() error { return ErrTestOnly }

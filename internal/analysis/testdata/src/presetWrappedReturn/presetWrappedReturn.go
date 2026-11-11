// Package presetWrappedReturn is the analysistest fixture for the
// sample preset example/werror.yaml loaded through vow.yaml: the
// wrap signatures it registers make a vow:emit marker on a return
// through example.com/wraplib.Wrap resolve, where the built-in
// presets alone report the destination as uninferred.
package presetWrappedReturn

import (
	"errors"

	"example.com/wraplib"
)

// ErrFoo is the sentinel the fixture hands off through the wrap call.
//
// vow:define @Sentinel
var ErrFoo = errors.New("foo") // want ErrFoo:"subject\\(vow:define @Sentinel\\)"

// wrapped resolves through the registered wrap signature.
//
// vow:cond * -> ErrFoo | nil
func wrapped() error {
	// vow:emit ErrFoo
	return wraplib.Wrap(ErrFoo, "context")
}

// unwrapped returns through no registered call, so the destination
// stays uninferred.
//
// vow:cond * -> ErrFoo | nil
func unwrapped() error {
	x := ErrFoo
	// vow:emit ErrFoo // want `vow\[closable\]: vow:emit destination could not be inferred`
	return x
}

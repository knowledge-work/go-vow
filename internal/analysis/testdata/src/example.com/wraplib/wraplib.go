// Package wraplib is the mock package the wrappedReturn fixture
// imports under the RFC 2606 example.com path that the sample
// passthrough-emit preset registers. The package exposes the Wrap
// and Errorf signatures the preset names so the wrapped-return
// destination recogniser matches calls to these functions through
// pass.TypesInfo without depending on a real wrap library.
package wraplib

import "fmt"

// Wrap wraps an error with a message and returns the wrapped
// error. The analyzer recognises the call as a wrapped-return
// destination when the caller-side vow:emit marker anchors to
// a return that flows through this function.
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// Errorf formats a wrap message and returns the wrapped error.
// The preset registers the function alongside Wrap so a call to
// either lands on the wrapped-return destination.
func Errorf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}

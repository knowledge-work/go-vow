// Package decl declares the sentinel that package use refers to.
package decl

import "errors"

// ErrGone is the sentinel that package use returns without observing.
//
// vow:define @Sentinel
var ErrGone = errors.New("gone")

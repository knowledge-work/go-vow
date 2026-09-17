// Package sentinels is the analysistest fixture for the sentinel-error
// preset. Each function exercises one reference class — observe,
// chain, or leak — so the classifier's behavior reads off as a small
// table.
package sentinels

import (
	"errors"
	"fmt"
)

// ErrNotFound is the subject under test.
// vow:define @Sentinel
var ErrNotFound = errors.New("not found")

// --- observe: errors.Is inside an if condition. ---

func observedByIs(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return nil
}

// --- observe: errors.As inside an if condition. ---

func observedByAs(err error) error {
	if errors.As(err, &ErrNotFound) {
		return nil
	}
	return nil
}

// --- observe: equality comparison inside an if condition. ---

func observedByEquality(err error) error {
	if err == ErrNotFound {
		return nil
	}
	if err != ErrNotFound {
		return nil
	}
	return nil
}

// --- observe: switch-tag match. ---

func observedBySwitch(err error) error {
	switch err {
	case ErrNotFound:
		return nil
	}
	return nil
}

// --- chain: explicit propagation authorized by vow:cond. ---

// chainedThroughAnnotation forwards the sentinel to its caller. The
// annotation puts the caller on notice that this return value is the
// listed subject.
//
// vow:cond * -> ErrNotFound | nil
func chainedThroughAnnotation() error {
	return ErrNotFound
}

// --- leak: bare return without the annotation. ---

func leakByReturn(id int) error {
	if id < 0 {
		return ErrNotFound // want `vow\[sentinel-error\]: sentinel error ErrNotFound leaked: needs observation or explicit propagation`
	}
	return nil
}

// --- leak: %w wrap is still a leak without the annotation. ---

func leakByWrap(id int) error {
	if id < 0 {
		return fmt.Errorf("find %d: %w", id, ErrNotFound) // want `vow\[sentinel-error\]: sentinel error ErrNotFound leaked: needs observation or explicit propagation`
	}
	return nil
}

// --- leak: observer call outside a conditional context. ---

// leakByLooseIs invokes errors.Is, but the result is returned as a
// bool — there is no if/switch around the call, so the subject
// reference is not in a conditional position and remains a leak.
func leakByLooseIs(err error) bool {
	return errors.Is(err, ErrNotFound) // want `vow\[sentinel-error\]: sentinel error ErrNotFound leaked: needs observation or explicit propagation`
}

// --- baseline: an un-annotated sentinel is never classified. ---
//
// ErrOther deliberately omits the marker. The prose below mentions
// the marker token to verify that line-anchored detection ignores
// in-sentence references.
//
// Reference: see ErrNotFound above for the vow:define @Sentinel pattern.
var ErrOther = errors.New("other")

func returnNonSentinel() error {
	return ErrOther
}

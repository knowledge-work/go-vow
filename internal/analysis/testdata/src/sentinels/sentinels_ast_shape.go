package sentinels

import "errors"

// observedByMultiCase exercises a switch-case with two sentinel
// values in the same case list. Each subject reference is classified
// independently — both ErrNotFound and ErrPair are observe.
func observedByMultiCase(err error) error {
	switch err {
	case ErrNotFound, ErrPair:
		return nil
	}
	return nil
}

// observedInsideUnaryNot exercises the boolean-glue passthrough for
// `!errors.Is(...)` inside an if condition. The sentinel reference
// is wrapped in a UnaryExpr but still counts as observe.
func observedInsideUnaryNot(err error) error {
	if !errors.Is(err, ErrNotFound) {
		return nil
	}
	return nil
}

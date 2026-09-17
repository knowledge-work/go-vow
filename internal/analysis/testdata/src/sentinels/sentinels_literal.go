package sentinels

import "errors"

// ErrPair and ErrAlt exercise tuple-sum and value-literal positions.

// vow:define @Sentinel
var ErrPair = errors.New("pair")

// vow:define @Sentinel
var ErrAlt = errors.New("alt")

// exclusivePairOK returns one of the two declared (value, error)
// tuples. Both branches match a declared tuple — no diagnostic.
//
// vow:cond * -> (1, nil) | (0, ErrPair)
func exclusivePairOK(ok bool) (int, error) {
	if ok {
		return 1, nil
	}
	return 0, ErrPair
}

// exclusivePairLeak returns a tuple that matches neither declared
// alternative. Two diagnostics land on this line:
//
//   - the vow:cond checker reports that (2, ErrAlt) is not a member
//     of the declared sum.
//   - the must-consume rule reports a leak because the annotation
//     does not authorize ErrAlt.
//
// vow:cond * -> (1, nil) | (0, ErrPair)
func exclusivePairLeak() (int, error) {
	return 2, ErrAlt // want `vow\[sentinel-error\]: return values do not match any tuple of the declared sum` `vow\[sentinel-error\]: sentinel error ErrAlt leaked: needs observation or explicit propagation`
}

// statusCodeOK returns one of the literals listed in the sum.
//
// vow:cond * -> 200 | 404 | 500
func statusCodeOK(found bool) int {
	if found {
		return 200
	}
	return 404
}

// statusCodeLeak returns a value not in the sum. The vow:cond
// checker reports the literal mismatch; the must-consume rule is
// silent because 418 is not a subject.
//
// vow:cond * -> 200 | 404 | 500
func statusCodeLeak() int {
	return 418 // want `vow\[sentinel-error\]: return position 0: returning literal 418 but the position only accepts 200 \| 404 \| 500`
}

// mixedConstNilOK accepts either ErrPair or nil; both are listed so
// the must-consume rule treats ErrPair as chain and nil is permitted
// by the literal-nil member.
//
// vow:cond * -> ErrPair | nil
func mixedConstNilOK(ok bool) error {
	if ok {
		return nil
	}
	return ErrPair
}

package sentinels

// Fixtures for switch-statement narrowing.
//
// A `switch x { case nil: ... }` body proves `x == nil`; the
// default branch proves `x != nil`. A `switch n { case 0: ... }`
// body proves `n == 0`; the default branch proves `n != 0`. The
// SSA package lowers a non-dense switch on an interface or
// scalar value to a chain of `if x == const { ... } else { ... }`
// instructions, so the existing dominator walk that drives the
// if-statement narrowing applies to switch-case branches
// uniformly.

// switchNarrowNilCase covers the `case nil:` body of a switch on
// an interface value. The body sits inside the dominator-side
// where `err == nil` is proved; the `nil` parameter requirement
// holds. The return requirement `0` admits the literal 0.
//
// vow:cond nil -> 0
func switchNarrowNilCase(err error) int {
	switch err {
	case nil:
		return 0
	}
	return 0
}

// switchNarrowDefaultBranch covers the default branch — the
// dominator path where every preceding case was missed. For a
// switch on an interface against `nil`, the default branch
// proves `err != nil`, which matches the `nonnil` parameter
// requirement.
//
// vow:cond nonnil -> 1 | 2 | 3
func switchNarrowDefaultBranch(err error) int {
	switch err {
	case nil:
		return 0
	default:
		return 4 // want `vow\[sentinel-error\]: return position 0: returning literal 4 but the position only accepts 1 \| 2 \| 3`
	}
}

// switchNarrowZeroCase covers the `case 0:` body of a switch on
// an int. The body proves `n == 0`; the `zero` parameter
// requirement holds.
//
// vow:cond zero -> 0
func switchNarrowZeroCase(n int) int {
	switch n {
	case 0:
		return 0
	}
	return 0
}

// switchNarrowZeroDefault covers the default branch of a switch
// on an int against 0. The default proves `n != 0`; the
// `nonzero` parameter requirement holds, and the return
// requirement enforces `1 | 2 | 3` there.
//
// vow:cond nonzero -> 1 | 2 | 3
func switchNarrowZeroDefault(n int) int {
	switch n {
	case 0:
		return 0
	default:
		return 5 // want `vow\[sentinel-error\]: return position 0: returning literal 5 but the position only accepts 1 \| 2 \| 3`
	}
}

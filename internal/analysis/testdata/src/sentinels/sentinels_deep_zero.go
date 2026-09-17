package sentinels

// Coord is a nested struct used to exercise the deep-zero walker:
// every leaf field must evaluate to its kind's zero for the
// composite to count as zero.
type Coord struct {
	X int
	Y int
}

type Span struct {
	Start Coord
	End   Coord
	Label string
}

// spanAllZero returns a Span whose every field is explicitly zero.
// The deep-zero walker recurses into Start and End.
//
// vow:cond * -> nonzero Span
func spanAllZero() Span {
	return Span{Start: Coord{X: 0, Y: 0}, End: Coord{X: 0, Y: 0}, Label: ""} // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero Span`
}

// spanPartialZero leaves Label implicit but writes Start/End zeros.
// The walker treats Elts == 3 as zero only when every listed element
// is zero — here Start, End, and Label are explicitly zero.
//
// vow:cond * -> nonzero Span
func spanPartialZero() Span {
	return Span{Start: Coord{X: 0, Y: 0}, End: Coord{X: 0, Y: 0}, Label: ""} // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero Span`
}

// spanNonZeroLeaf reports zeroNo because one leaf is non-zero.
//
// vow:cond * -> nonzero Span
func spanNonZeroLeaf() Span {
	return Span{Start: Coord{X: 1, Y: 0}, End: Coord{}, Label: ""}
}

// spanNonZeroLabel reports zeroNo because the Label leaf is non-empty.
//
// vow:cond * -> nonzero Span
func spanNonZeroLabel() Span {
	return Span{Start: Coord{}, End: Coord{}, Label: "x"}
}

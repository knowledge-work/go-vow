package sentinels

// QValueA and QValueB are package-level consts used to exercise the
// const-qualifier rule: attaching a value-level predicate (the
// `nonzero` prefix or the `?` suffix) to a const reference is
// rejected because a const's value is fixed at compile time, so
// any nilability or non-zero contract on top is semantically
// empty.

const (
	QValueA = 1
	QValueB = 2
)

// constSingleQualifierLeak puts `nonzero` on a const at a Single
// Term position — the analyzer reports the violation at the
// function declaration.
//
// vow:cond * -> nonzero QValueA
func constSingleQualifierLeak() int { // want `vow\[sentinel-error\]: const QValueA cannot carry constraint "nonzero"; const values are fixed`
	return QValueA
}

// constSumQualifierLeak puts per-member `nonzero` on two consts.
// Both members are reported (the diagnostic anchors to the
// function declaration, so both messages land on the same line).
//
// vow:cond * -> nonzero QValueA | nonzero QValueB
func constSumQualifierLeak() int { // want `vow\[sentinel-error\]: const QValueA cannot carry constraint "nonzero"; const values are fixed` `vow\[sentinel-error\]: const QValueB cannot carry constraint "nonzero"; const values are fixed`
	return QValueA
}

// sumOverallQualifierOK keeps the qualifier on the sum as a whole.
// The const referents stay bare, so the const-qualifier rule is
// silent.
//
// vow:cond * -> (QValueA | QValueB)?
func sumOverallQualifierOK() int {
	return QValueA
}

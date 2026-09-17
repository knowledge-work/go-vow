package vowUse

// directViaEquality satisfies the discharge contract by comparing
// the subject identifier inside an if condition. Boolean glue
// (parens, !, &&, ||, ==, !=) transit so the comparison still
// counts even when the subject is buried inside the cond.
// vow:use ErrFoo
func directViaEquality(err error) {
	if err == ErrFoo {
		return
	}
}

// directViaSwitch satisfies the contract through a switch case
// label. The case label is recognised as a discharge site on the
// switched value: the subject identifier sits at a Cond-equivalent
// position in the AST.
// vow:use ErrFoo
func directViaSwitch(err error) {
	switch err {
	case ErrFoo:
		return
	}
}

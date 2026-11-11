// Package condOperandReference is the analysistest fixture for the
// operands of an unscoped logical-arrow vow:cond. An operand that
// names nothing in the function's own signature is reported at the
// declaration; operands that bind stay silent.
package condOperandReference

type T struct{ V int }

// positionPastReturns names a return position the function lacks.
//
// vow:cond $9 != nil => t != nil
func positionPastReturns(t *T) (*T, error) { // want `vow\[sentinel-error\]: vow:cond references \$9 but the function signature has fewer return positions`
	return t, nil
}

// unknownIdentifier names an identifier that is not in the signature.
//
// vow:cond nosuchident != nil => t != nil
func unknownIdentifier(t *T) (*T, error) { // want `vow\[sentinel-error\]: vow:cond references "nosuchident", which is not a parameter, receiver, or named return of the function signature`
	return t, nil
}

// receiverAndPosition binds the receiver and the first return.
//
// vow:cond r != nil => $1 != nil
func (r *T) receiverAndPosition() (*T, error) {
	return r, nil
}

// namedReturn binds a parameter and a named return.
//
// vow:cond t != nil <=> out != nil
func namedReturn(t *T) (out *T, err error) {
	return t, nil
}

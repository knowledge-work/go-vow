package vowUse

// voidSignature declares the discharge contract on a function
// that returns nothing. The contract is independent of the return
// shape: a body containing one qualifying discharge site keeps the
// declaration self-validated regardless of the signature.
// vow:use ErrFoo
func voidSignature(err error) {
	if err == ErrFoo {
		return
	}
}

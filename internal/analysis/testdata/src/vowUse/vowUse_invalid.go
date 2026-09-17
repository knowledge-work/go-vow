package vowUse

// missingDischarge declares the discharge contract but its body
// never references the subject in a conditional position and never
// hands the subject off to another discharge callee. The self-
// validation step reports the missing site at the declaration.
// vow:use ErrFoo
func missingDischarge(err error) { // want `vow\[sentinel-error\]: vow:use ErrFoo declaration is not self-validated: function body has no discharge site for ErrFoo`
	_ = err
}

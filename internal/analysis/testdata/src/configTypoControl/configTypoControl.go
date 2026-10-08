// Package configTypoControl is configTypo with the vow.yaml key spelled
// correctly, so the declaration-completeness rule runs.
package configTypoControl

func undeclared(id *string) string { // want `vow\[nil-decl\]: vow:nil leaves parameter id without a nullness decl`
	return *id
}

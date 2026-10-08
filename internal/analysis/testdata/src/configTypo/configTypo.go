// Package configTypo is the analysistest fixture for a vow.yaml the
// discovery walk finds but cannot parse: the key under `nil_decl`
// misspells `require_declarations`. The analyzer reports the file and
// runs with the built-in defaults, so the declaration-completeness rule
// the file asks for stays off.
package configTypo // want `vow\[config\]: parse .*/configTypo/vow\.yaml: yaml: unmarshal errors: line 2: field require_declaration not found in type config\.NilDecl; the built-in defaults apply instead`

// undeclared would be reported by the declaration-completeness rule if
// the vow.yaml next to it spelled its key correctly, as the
// configTypoControl fixture does.
func undeclared(id *string) string {
	return *id
}

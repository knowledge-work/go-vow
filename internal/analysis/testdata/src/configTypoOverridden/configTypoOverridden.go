// Package configTypoOverridden carries the same misspelled vow.yaml as
// configTypo, for a run whose driver-supplied config replaces the
// per-directory file. It is a separate fixture because configTypo
// expects the report this run must not make.
package configTypoOverridden

func undeclared(id *string) string {
	return *id
}

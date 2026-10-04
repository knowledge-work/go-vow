// Package configTypoNarrow carries a vow.yaml with two misspelled keys in
// a package of two files, for a run whose --changed-files names only the
// second, b.go.
package configTypoNarrow

func a() int { return 1 }

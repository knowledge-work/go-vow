package sentinels

// stringEscapeOK declares the same content with the same escape
// sequence. Unquote-normalized comparison succeeds; the position is
// satisfied with no diagnostic.
//
// vow:cond * -> "a\tb"
func stringEscapeOK() string {
	return "a\tb"
}

// stringEscapeMismatch declares "a\tb" but returns "a\nb"; the
// normalized strings differ.
//
// vow:cond * -> "a\tb"
func stringEscapeMismatch() string {
	return "a\nb" // want `vow\[sentinel-error\]: return position 0: returning literal "a\\nb" but the position only accepts "a\\tb"`
}

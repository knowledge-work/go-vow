package discharged

// payloadMarkerRejected carries a trailing token after the bare
// marker. The marker is documented as payload-less, so the parser
// flags the function declaration and omits it from the discharged
// set; downstream barriers must not trust a half-formed entry.
// vow:discharged subject=ErrFoo
func payloadMarkerRejected() error { // want `vow\[sentinel-error\]: vow:discharged takes no payload; declare it as a bare marker line`
	return nil
}

// lineLevelMarkerRejected places the marker on a body line. Because
// the marker is function-scoped, an in-body occurrence would
// silently be discarded by the doc scan; the analyzer surfaces it
// explicitly so the authoring error is visible.
func lineLevelMarkerRejected() error {
	// vow:discharged // want `vow\[sentinel-error\]: vow:discharged is only legal in a function doc comment`
	return nil
}

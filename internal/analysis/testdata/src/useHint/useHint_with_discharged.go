package useHint

// useWithDischargedCallee calls a vow:discharged function. The
// discharged barrier short-circuits the caller-side must-consume
// engine before vowReport runs, so the vow:use marker that
// trails the call is redundant. The fixture exists to confirm the
// coexistence is harmless: no extra diagnostic is emitted, and the
// discharged drop wins on order without the use-assertion filter
// surfacing a "no diagnostic to silence" complaint.
func useWithDischargedCallee() {
	silentlyDischarged(ErrFoo) // vow:use ErrFoo
}

// silentlyDischarged is a discharged boundary: the analyzer treats
// every caller-side reference that flows into the body as already
// discharged, regardless of what happens inside.
//
// vow:discharged
func silentlyDischarged(_ error) {}

package sentinels

// --- primitive nonzero T ---

// intNonZeroOK returns a non-zero int — silent.
//
// vow:cond * -> nonzero int
func intNonZeroOK() int {
	return 42
}

// intNonZeroLeak returns the int zero value — diagnose.
//
// vow:cond * -> nonzero int
func intNonZeroLeak() int {
	return 0 // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero int`
}

// stringNonEmptyLeak returns an empty string — diagnose.
//
// vow:cond * -> nonzero string
func stringNonEmptyLeak() string {
	return "" // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero string`
}

// boolMustBeTrueLeak returns false — diagnose.
//
// vow:cond * -> nonzero bool
func boolMustBeTrueLeak() bool {
	return false // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero bool`
}

// --- named primitive nonzero T ---

type Status int

const (
	StatusZero   Status = 0
	StatusActive Status = 1
)

// statusActiveOK returns a non-zero const Status — silent.
//
// vow:cond * -> nonzero Status
func statusActiveOK() Status {
	return StatusActive
}

// statusZeroLeak returns the zero const Status — diagnose. The
// analyzer resolves StatusZero through constant folding regardless of
// the named type.
//
// vow:cond * -> nonzero Status
func statusZeroLeak() Status {
	return StatusZero // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero Status`
}

// --- composite literal (struct zero) ---

type Profile struct{ Name string }

// profileFilledOK returns a non-zero composite literal — silent.
//
// vow:cond * -> nonzero Profile
func profileFilledOK() Profile {
	return Profile{Name: "x"}
}

// profileEmptyLeak returns an empty composite literal — diagnose.
// Deep struct inspection (Profile with every field recursively the
// zero value of its type) is covered separately; this check only
// matches the shallow Elts == 0 shape.
//
// vow:cond * -> nonzero Profile
func profileEmptyLeak() Profile {
	return Profile{} // want `vow\[sentinel-error\]: return position 0: returning the zero value at a position declared nonzero Profile`
}

// --- direct-sum integration (type-keyed, not const-keyed) ---
//
// Attaching `nonzero` to a const reference is semantically empty (the
// const's value is fixed at compile time), so the analyzer rejects
// that shape outright. The remaining direct-sum integration test
// lives in sentinels_literal.go via the literal-member form
// (`{ 200 | 404 | 500 }`).

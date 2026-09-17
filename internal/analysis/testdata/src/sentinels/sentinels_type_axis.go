package sentinels

// Types-aware Term matching widens shape-vs-Term comparison beyond
// syntactic identifier text. Two of the four matching axes consult
// the expression's Go type: typeName (the expression's named type
// as types.Type.String would print it) and underlying (one
// Underlying() step). The scenarios below pair each axis with a
// representative defined-type chain, and the negative scenario
// confirms the four-axis lookup still rejects genuine mismatches.
//
// Direct-sum positions are used throughout because Term-name matching
// is only enforced against sum members; Single-Term positions reserve
// validation for the `!` qualifier by design.

// errorAlias is a true alias to error. types.TypeOf for a variable
// declared `errorAlias` resolves transparently to error (aliases do
// not create a new named type in go/types), so the typeName axis
// produces the string "error".
type errorAlias = error

// aliasedErr is a package-level value whose declared type is the
// alias. The alias is invisible by the time the analyzer sees the
// types.Info, but the identifier text "aliasedErr" still does not
// match a `error` Term — only the typeName axis does.
var aliasedErr errorAlias

// returnsViaAlias exercises the typeName axis. With newer
// go/types versions the alias resolves to its own declared
// identifier rather than the underlying error type, so the
// matcher sees `aliasedErr` as the type's surface name; listing
// both `error` and `aliasedErr` in the sum keeps the fixture
// compatible across the alias-resolution semantics the
// go/analysis SSA support has carried over time.
//
// vow:cond * -> error | aliasedErr | nil
func returnsViaAlias() error {
	return aliasedErr
}

// IntList is a defined slice type. types.TypeOf produces
// typeName "sentinels.IntList" and underlying "[]int". The annotation
// in returnsByUnderlying spells the structural form `[]int`, which is
// only reachable via the underlying axis.
type IntList []int

// returnsByUnderlying exercises the underlying axis. The sum element
// names the structural form `[]int`. shape.name is the variable
// identifier "tokens"; shape.typeName is "sentinels.IntList"; only
// shape.underlying (`[]int`) reaches the annotation Term.
//
// vow:cond * -> []int | nil
func returnsByUnderlying() IntList {
	var tokens IntList
	return tokens
}

// returnsByTypeName exercises the typeName axis on a defined type
// (rather than an alias). The sum member spells the fully-qualified
// type name `sentinels.IntList`, which matches typeName exactly. The
// fixture confirms that defined types — not just aliases — surface
// through the new axis.
//
// vow:cond * -> sentinels.IntList | nil
func returnsByTypeName() IntList {
	var tokens IntList
	return tokens
}

// returnsTypedRejected is the negative baseline: the sum member names
// a different type that does not appear on any axis (name="tokens",
// fullName="sentinels.tokens", typeName="sentinels.IntList",
// underlying="[]int"). The checker reports the mismatch at the
// return expression so the four-axis lookup cannot be silently too
// permissive.
//
// vow:cond * -> sentinels.OtherList | nil
func returnsTypedRejected() IntList {
	var tokens IntList
	return tokens // want `vow\[sentinel-error\]: return position 0: returning tokens but the position only accepts sentinels.OtherList \| nil`
}

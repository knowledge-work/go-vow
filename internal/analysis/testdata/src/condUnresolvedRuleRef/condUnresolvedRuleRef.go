// vow:import result "preset/std/result"
package condUnresolvedRuleRef

// Foo is the payload the annotated constructors return.
type Foo struct{ v int }

// missingAlias names an alias the package never imported. The
// reference cannot resolve, so the contract the author wrote
// would otherwise be dropped without a word.
//
// vow:cond @absent.OkErr[*Foo, error]
func missingAlias() (*Foo, error) { // want `vow\[sentinel-error\]: vow:cond payload "@absent.OkErr\[\*Foo, error\]" was not applied: alias "absent" is not in scope \(missing vow:import\?\)`
	return &Foo{}, nil
}

// missingRule reaches a preset that is in scope but asks it for a
// rule it does not declare — the typo shape the lookup already
// separates from a missing alias.
//
// vow:cond @result.Absent[*Foo, error]
func missingRule() (*Foo, error) { // want `vow\[sentinel-error\]: vow:cond payload "@result.Absent\[\*Foo, error\]" was not applied: alias "result" has no rule named "Absent": unknown reference`
	return &Foo{}, nil
}

// proseMarker opens a doc line with the marker name, the shape
// the authoring tip warns about. The line fails as a syntax
// error rather than as a lookup, so it stays silent here and the
// existing prose stance is unchanged.
//
// vow:cond rules are what this comment happens to talk about
func proseMarker() (*Foo, error) {
	return &Foo{}, nil
}

// scopedMarker carries the `[X]` scope qualifier, which the
// diagnostic must echo so the reported marker reads as the author
// wrote it.
//
// vow:cond[cb] @absent.OkErr[*Foo, error]
func scopedMarker(cb func() *Foo) (*Foo, error) { // want `vow\[sentinel-error\]: vow:cond\[cb\] payload "@absent.OkErr\[\*Foo, error\]" was not applied`
	return cb(), nil
}

// plainCondition carries a reference-free rule, which resolves
// through the ordinary parser and must not reach this surface.
//
// vow:cond foo != nil <=> err == nil
func plainCondition() (foo *Foo, err error) {
	return &Foo{}, nil
}

// proseWithReference opens with the marker and happens to carry an
// `@` in the prose. The token is not a rule reference, so the line
// fails as a syntax error and stays silent like any other prose.
//
// vow:cond see @notes for the rationale
func proseWithReference() (*Foo, error) {
	return &Foo{}, nil
}

// localReference names a rule with no alias. Local rules live in
// the preset that declares the annotated function, which Go source
// never is, so the reference resolves to nothing to apply.
//
// vow:cond @Absent[*Foo, error]
func localReference() (*Foo, error) { // want `vow\[sentinel-error\]: vow:cond payload "@Absent\[\*Foo, error\]" was not applied: rule "Absent" is not defined locally`
	return &Foo{}, nil
}

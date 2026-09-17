// Package condGrammar is the analysistest fixture for the
// expanded `vow:cond` grammar that carries identifier-named
// references, postfix steps (`.field`, `[i]`, `["k"]`), comparison
// operators, and the logical-arrow rule shapes (`=>` and `<=>`).
// The fixture also exercises the left-hand-side sugar clause that
// expands `; <arrow> rhs` into a full rule by carrying the
// preceding rule's left-hand side.
//
// Most functions here are silent baselines: the new shapes round
// trip through the parser and the analyzer accepts them. The
// return-axis fixtures additionally exercise the literal-level
// evaluator that decides logical-arrow rules whose operands bind
// to named-return or `$N` positional-return references — silent
// baselines pair a rule with returns that satisfy it, and the
// diagnose cases pair the same rule shape with returns whose
// literal nil shape contradicts the rule's direction. Cross-rule
// inconsistency detection, ordered-comparison evaluation, and
// flow-driven parameter narrowing land alongside the separate
// relational-constraint analysis.
package condGrammar

// Config holds nested data so the postfix-step fixtures can walk
// through field, slice-index, and map-key forms in one place.
type Config struct {
	FeatureX  *Feature
	FeatureY  *Feature
	Endpoints map[string]*Endpoint
	Servers   []*Server
	Version   int
}

// Feature is the leaf carried by Config.FeatureX / FeatureY.
type Feature struct{ Name string }

// Endpoint is the value type behind Config.Endpoints.
type Endpoint struct{ URL string }

// Server is the element type behind Config.Servers.
type Server struct{ Host string }

// Event is the tagged-union shape exercised by the biconditional
// sugar fixture: the discriminator (Tag) implies which optional
// field is non-nil.
type Event struct {
	Tag    string
	Login  *LoginInfo
	Logout *LogoutInfo
}

// LoginInfo is the per-tag payload behind Event.Login.
type LoginInfo struct{ User string }

// LogoutInfo is the per-tag payload behind Event.Logout.
type LogoutInfo struct{ Reason string }

// implyNilCheck pairs two nil checks with a forward implication so
// the parser walks the simplest logical-arrow shape end to end.
//
// vow:cond x != nil => y != nil
func implyNilCheck(x, y *int) error { _ = x; _ = y; return nil }

// equivNilCheck threads the biconditional arrow through the same
// pair so both directions of `<=>` land in the parser.
//
// vow:cond x != nil <=> y != nil
func equivNilCheck(x, y *int) error { _ = x; _ = y; return nil }

// equalLiteralImplyNil exercises the comparison operand on the
// left and the nil check on the right.
//
// vow:cond tag == "login" => payload != nil
func equalLiteralImplyNil(tag string, payload *LoginInfo) error { _ = tag; _ = payload; return nil }

// orderedComparison exercises the ordered comparison operators
// (`>=`) together with a field-access postfix on the left.
//
// vow:cond c.Version >= 2 => c.FeatureX != nil
func orderedComparison(c *Config) error { _ = c; return nil }

// sliceAccess exercises the integer-index postfix step on a slice
// reference. The body is silent because the analyzer accepts the
// new shape without enforcing it.
//
// vow:cond c.Servers[0] != nil => c.Servers[1] != nil
func sliceAccess(c *Config) error { _ = c; return nil }

// mapAccess exercises the string-key postfix step. The key
// preserves its surrounding quotes through the AST round trip.
//
// vow:cond c.Endpoints["primary"] != nil <=> c.Endpoints["secondary"] == nil
func mapAccess(c *Config) error { _ = c; return nil }

// positionalReturn exercises the `$N` positional return reference
// on both halves of a forward implication.
//
// vow:cond $1 != nil => $2 == nil
func positionalReturn() (*Feature, *Endpoint) { return nil, nil }

// sugarImply exercises the sugar clause that carries the
// preceding rule's left-hand side across two `=>` arrows so the
// author writes the discriminator once.
//
// vow:cond c.Version >= 2 => c.FeatureX != nil ; => c.FeatureY != nil
func sugarImply(c *Config) error { _ = c; return nil }

// sugarEquiv exercises the same carry across `<=>` so the
// biconditional sugar path is pinned alongside the imply form.
//
// vow:cond e.Tag == "login" <=> e.Login != nil ; <=> e.Logout == nil
func sugarEquiv(e *Event) error { _ = e; return nil }

// Cache is the receiver-axis fixture host: methods on Cache
// declare logical rules whose head identifier is the method's
// receiver (the surface spells the receiver as it appears in the
// signature).
type Cache struct {
	Primary   *Endpoint
	Secondary *Endpoint
}

// receiverAxis exercises the receiver identifier as the head of a
// reference on both halves of a logical rule. The receiver `c`
// resolves against the method signature exactly like a regular
// parameter would on a free function.
//
// vow:cond c.Primary != nil => c.Secondary != nil
func (c *Cache) receiverAxis() error { return nil }

// namedReturnAxisSilent exercises a named return value as the head
// of a reference together with returns that satisfy the rule. Named
// returns appear in the same identifier scope as parameters and the
// receiver, so the logical rule references `result` directly without
// the `$N` positional prefix.
//
// vow:cond err == nil => result != nil
func namedReturnAxisSilent(input *Feature) (result *Feature, err error) {
	_ = input
	return &Feature{Name: "ok"}, nil
}

// namedReturnAxisDiagnose returns nil for both positions so the
// implication's left-hand side holds (err is the nil literal) but
// the right-hand side fails (result is the nil literal). The
// evaluator pairs the literal shape against the rule's direction
// and surfaces a diagnostic at the return statement.
//
// vow:cond err == nil => result != nil
func namedReturnAxisDiagnose(input *Feature) (result *Feature, err error) {
	_ = input
	return nil, nil // want `vow\[sentinel-error\]: return violates vow:cond err == nil => result != nil: err == nil holds at this return but result != nil does not`
}

// positionalReturnDiagnose mirrors the named-return diagnose case
// through the `$N` positional reference form so the literal
// evaluator pins both surface shapes that bind to a return slot.
//
// vow:cond $2 == nil => $1 != nil
func positionalReturnDiagnose() (*Feature, error) {
	return nil, nil // want `vow\[sentinel-error\]: return violates vow:cond \$2 == nil => \$1 != nil: \$2 == nil holds at this return but \$1 != nil does not`
}

// Cleanup is the callback shape paired with a Resource. A
// successful acquisition returns a non-nil Cleanup together with
// a non-nil Resource; the caller invokes Cleanup to release the
// resource. A no-op acquisition returns both slots as nil. The
// biconditional fixture below uses the two slots together so the
// pair-of-nils invariant has a natural home.
type Cleanup func()

// Resource is the resource handle paired with a Cleanup callback.
// The biconditional fixture below declares that the two slots
// travel together: both nil for a no-op acquisition, both non-nil
// for a real one.
type Resource struct{ Name string }

// equivCleanupResourceDiagnose exercises the biconditional arrow on
// return-axis references through the cleanup/resource pair shape,
// where the invariant is that the two slots are nil together or
// non-nil together. The fixture returns a nil cleanup paired with
// an address-of composite-literal resource so the two sides
// diverge, contradicting the `<=>` equivalence. The evaluator
// recognises the address-of operator as a syntactic non-nil shape,
// which is sufficient to decide the resource == nil side as false.
//
// vow:cond cleanup == nil <=> resource == nil
func equivCleanupResourceDiagnose() (cleanup Cleanup, resource *Resource) {
	return nil, &Resource{Name: "example"} // want `vow\[sentinel-error\]: return violates vow:cond cleanup == nil <=> resource == nil: cleanup == nil holds at this return but resource == nil does not`
}

// modeReturnAxisSilent declares an implication whose left-hand
// side is a string equality on a named return slot. The fixture
// returns the matching mode together with a non-nil handler so the
// implication holds and no diagnostic surfaces.
//
// vow:cond mode == "live" => handler != nil
func modeReturnAxisSilent() (mode string, handler *Server) {
	return "live", &Server{Host: "ok"}
}

// modeMismatchReturnAxisSilent threads the same rule with a mode
// that does not match the live sentinel so the implication's
// left-hand side fails. The evaluator commits the rule as
// trivially held without inspecting the right-hand side.
//
// vow:cond mode == "live" => handler != nil
func modeMismatchReturnAxisSilent() (mode string, handler *Server) {
	return "draft", nil
}

// modeReturnAxisDiagnose pairs the live mode with a nil handler so
// the implication's left-hand side holds (the mode equals the
// sentinel string) but the right-hand side fails (the handler is
// the nil literal). The evaluator surfaces a diagnostic at the
// return statement.
//
// vow:cond mode == "live" => handler != nil
func modeReturnAxisDiagnose() (mode string, handler *Server) {
	return "live", nil // want `vow\[sentinel-error\]: return violates vow:cond mode == "live" => handler != nil: mode == "live" holds at this return but handler != nil does not`
}

// versionReturnAxisSilent exercises the ordered-comparison
// operator (`>=`) on a named integer return. The fixture returns a
// version that clears the threshold together with a non-nil
// feature so the implication holds.
//
// vow:cond version >= 2 => feature != nil
func versionReturnAxisSilent() (version int, feature *Feature) {
	return 3, &Feature{Name: "ok"}
}

// versionReturnAxisDiagnose pairs a clearing version with a nil
// feature so the ordered comparison decides the left-hand side as
// true while the right-hand side fails. The evaluator surfaces a
// diagnostic at the return statement.
//
// vow:cond version >= 2 => feature != nil
func versionReturnAxisDiagnose() (version int, feature *Feature) {
	return 3, nil // want `vow\[sentinel-error\]: return violates vow:cond version >= 2 => feature != nil: version >= 2 holds at this return but feature != nil does not`
}

// callerImplySilentTrivial calls implyNilCheck with a nil first
// argument so the implication's left-hand side fails and the
// rule holds trivially without inspecting the second argument.
// The caller-side evaluator stays silent.
func callerImplySilentTrivial() { _ = implyNilCheck(nil, nil) }

// callerImplySilentBothNonNil calls implyNilCheck with two
// statically non-nil pointers so both sides of the implication
// hold and no diagnostic surfaces.
func callerImplySilentBothNonNil() {
	a, b := 1, 2
	_ = implyNilCheck(&a, &b)
}

// callerImplyDiagnose calls implyNilCheck with a non-nil first
// argument paired with a nil second argument. The caller-side
// evaluator decides the left-hand side as true (the first
// argument is an address-of operator) and the right-hand side
// as false (the second argument is the bare nil literal), so the
// implication is contradicted at the call site.
func callerImplyDiagnose() {
	a := 1
	_ = implyNilCheck(&a, nil) // want `vow\[sentinel-error\]: call violates vow:cond x != nil => y != nil: x != nil holds at this call but y != nil does not`
}

// callerEquivDiagnose calls equivNilCheck with one non-nil and
// one nil pointer so the biconditional's two sides diverge,
// surfacing a diagnostic at the call site.
func callerEquivDiagnose() {
	a := 1
	_ = equivNilCheck(&a, nil) // want `vow\[sentinel-error\]: call violates vow:cond x != nil <=> y != nil: x != nil holds at this call but y != nil does not`
}

// callerTagDiagnose calls equalLiteralImplyNil with the matching
// tag string paired with a nil payload so the literal comparison
// decides the left-hand side as true and the nil check decides
// the right-hand side as false. The caller-side evaluator
// surfaces the contradiction at the call site.
func callerTagDiagnose() {
	_ = equalLiteralImplyNil("login", nil) // want `vow\[sentinel-error\]: call violates vow:cond tag == "login" => payload != nil: tag == "login" holds at this call but payload != nil does not`
}

// callerTagMismatchSilent calls equalLiteralImplyNil with a tag
// that does not match the live sentinel so the literal
// comparison decides the left-hand side as false. The
// implication holds trivially and no diagnostic surfaces.
func callerTagMismatchSilent() {
	_ = equalLiteralImplyNil("logout", nil)
}

// callerBlockGuardSilent calls implyNilCheck inside a guarded
// block that proves the first argument is non-nil through the
// dominating `if p != nil` check. The bare identifier carries no
// literal shape the surface evaluator can read, but the dominator
// walk narrows the parameter and decides the left-hand side as
// non-nil. The second argument is the address-of operator so the
// right-hand side commits non-nil too and the implication holds.
func callerBlockGuardSilent(p *int) {
	if p != nil {
		a := 1
		_ = implyNilCheck(p, &a)
	}
}

// callerBlockGuardDiagnose threads the same guarded block but
// pairs the narrowed first argument with the bare nil literal on
// the right. The dominator walk decides the left-hand side as
// non-nil while the literal-only path decides the right-hand
// side as nil, so the implication is contradicted at the call
// site.
func callerBlockGuardDiagnose(p *int) {
	if p != nil {
		_ = implyNilCheck(p, nil) // want `vow\[sentinel-error\]: call violates vow:cond x != nil => y != nil: x != nil holds at this call but y != nil does not`
	}
}

// callerAliasSilent threads a local alias through the call. The
// address-of operator declares the alias non-nil at definition;
// the one-hop alias walk carries the nilness forward so the call
// site sees the alias as non-nil. Pairing the alias with another
// address-of operator on the right keeps both sides committed.
func callerAliasSilent() {
	a := 1
	m := &a
	b := 2
	_ = implyNilCheck(m, &b)
}

// callerAliasDiagnose pairs the alias-narrowed first argument
// with a nil literal on the right so the implication contradicts
// at the call site. The forward walk recognises the
// local-identifier alias chain back to the address-of declaration
// and commits the left-hand side as non-nil.
func callerAliasDiagnose() {
	a := 1
	m := &a
	_ = implyNilCheck(m, nil) // want `vow\[sentinel-error\]: call violates vow:cond x != nil => y != nil: x != nil holds at this call but y != nil does not`
}

// returnBlockGuardDiagnose declares a parameter-axis logical
// rule and returns under a dominating guard that proves the
// parameter is non-nil. The literal-only evaluator cannot bind
// `p` against the return statement because parameters never
// occupy a return slot; the dominator walk narrows the
// parameter through the if-guard and commits the left-hand side
// as non-nil. The bare nil literal in the return decides the
// right-hand side as nil, so the implication is contradicted.
//
// vow:cond p != nil => result != nil
func returnBlockGuardDiagnose(p *int) (result *int) {
	if p != nil {
		return nil // want `vow\[sentinel-error\]: return violates vow:cond p != nil => result != nil: p != nil holds at this return but result != nil does not`
	}
	return nil
}

// returnBlockGuardSilent threads the same rule through a guarded
// return that pairs the narrowed parameter with an address-of
// return value. The dominator walk commits the left-hand side as
// non-nil; the address-of operator commits the right-hand side
// as non-nil; the implication holds.
//
// vow:cond p != nil => result != nil
func returnBlockGuardSilent(p *int) (result *int) {
	if p != nil {
		a := 1
		return &a
	}
	return nil
}

// contrapositivePairDiagnose pins the original-first fallback: an
// implication paired with a return that violates it would commit
// through both the original rule and its derived contrapositive
// (the negated operands name the same offending claim), but the
// caller skips the contrapositive after the original commits so
// the site reports one diagnostic rather than two. The single
// `want` comment regression-tests the fallback regime.
//
// vow:cond a != nil => b != nil
func contrapositivePairDiagnose() (a, b *int) {
	v := 1
	return &v, nil // want `vow\[sentinel-error\]: return violates vow:cond a != nil => b != nil: a != nil holds at this return but b != nil does not`
}

// chainExpansionSilent declares two implications whose
// intermediate operand connects them so the transitive closure
// derives an additional chain rule (`a != nil => c != nil`). The
// fixture returns values that satisfy every link so no rule fires:
// the source implications hold, the chain holds, and the closure
// stays silent. The fixture regression-pins that the closure does
// not surface a spurious violation on a holding chain.
//
// vow:cond a != nil => b != nil ; b != nil => c != nil
func chainExpansionSilent() (a, b, c *int) {
	v := 1
	return &v, &v, &v
}

// chainExpansionFallbackDiagnose uses the same two implications
// but pairs them with a return that violates the second source
// rule. The closure derives `a != nil => c != nil`, which would
// also fire on this return, but the original-first walk reports
// the source rule once rather than re-reporting the same offending
// claim through the chain. The single `want` comment pins the
// one-diagnostic-per-claim invariant across the chain layer.
//
// vow:cond a != nil => b != nil ; b != nil => c != nil
func chainExpansionFallbackDiagnose() (a, b, c *int) {
	v := 1
	return &v, &v, nil // want `vow\[sentinel-error\]: return violates vow:cond b != nil => c != nil: b != nil holds at this return but c != nil does not`
}

// topLevelSinkValue backs the address-of operator in the
// top-level call-site fixture below. The package-level scope is
// the point of the fixture: the caller-side evaluator must visit
// call expressions that live outside any function body.
var topLevelSinkValue int = 1

// topLevelCallDiagnose pins that a call expression at file scope —
// outside any function body — still runs the caller-side
// logical-arrow rule evaluator. The call pairs a non-nil
// address-of argument with the bare nil literal so the
// implication contradicts at the call site.
var topLevelCallDiagnose error = implyNilCheck(&topLevelSinkValue, nil) // want `vow\[sentinel-error\]: call violates vow:cond x != nil => y != nil: x != nil holds at this call but y != nil does not`

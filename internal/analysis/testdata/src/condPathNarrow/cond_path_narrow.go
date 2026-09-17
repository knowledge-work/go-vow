// Package condPathNarrow is the analysistest fixture for the
// path-bearing nil-check refinement. The refinement extends the
// `vow:cond` evaluator so an operand whose reference walks through
// a struct field and a slice index or map key (`cfg.Servers[0]`,
// `cfg.Endpoints["primary"]`) commits at a call or return site
// through two narrowing tiers: the dominator-guard tier matches a
// dominating `if <expr> != nil` whose condition is the SSA
// composition of the rule's path applied to the bound head; the
// declarative tier reads the field-nil registry at the path's
// terminal field. Each detection path pairs a silent baseline with
// a diagnose case so the no-false-positive invariant carries the
// same weight as the violation surface.
package condPathNarrow

// Server is the slice element carried by Config.Servers.
type Server struct{ Host string }

// Endpoint is the map value carried by Config.Endpoints.
type Endpoint struct{ URL string }

// Config bundles the slice and map fields the path fixtures walk
// through. Servers has no field-nil decl so the slice-index path
// commits only when a dominating guard pins it; Endpoints carries
// a `![!]!` nest decl so the map-key path commits declaratively
// through the field registry.
type Config struct {
	Servers []*Server

	// vow:nil ![!]!
	Endpoints map[string]*Endpoint // want Endpoints:"fieldNil\\(!\\)"
}

// dominatorSliceCallee declares an implication whose right-hand
// side reads a slice element through the receiver's path. The
// left-hand side commits trivially through the receiver's non-nil
// shape so the call-side check reduces to deciding the rhs path
// operand.
//
// vow:cond cfg != nil => cfg.Servers[0] != nil
func dominatorSliceCallee(cfg *Config) {}

// dominatorSliceRuleHoldsSilent guards the call with a dominating
// `if cfg.Servers[0] != nil`. The path narrowing helper walks the
// dominator chain, matches the guard's SSA condition against the
// rule's slice-index path, and commits the rhs as true; the lhs
// also commits as true so the implication holds and the walker
// stays silent.
//
// vow:nil (!)
func dominatorSliceRuleHoldsSilent(cfg *Config) { // want dominatorSliceRuleHoldsSilent:"nilDecl\\(!\\)"
	if cfg.Servers[0] != nil {
		dominatorSliceCallee(cfg)
	}
}

// dominatorSliceRuleViolatesDiagnose calls the same callee from
// inside an `if cfg.Servers[0] == nil` guard so the dominator walk
// pins the rhs to known-nil. The lhs commits as true, the rhs
// commits as false, and the walker reports the violation at the
// call.
//
// vow:nil (!)
func dominatorSliceRuleViolatesDiagnose(cfg *Config) { // want dominatorSliceRuleViolatesDiagnose:"nilDecl\\(!\\)"
	if cfg.Servers[0] == nil {
		dominatorSliceCallee(cfg) // want `vow\[sentinel-error\]: call violates vow:cond cfg != nil => cfg.Servers\[0\] != nil`
	}
}

// dominatorSliceUnguardedSilent omits any dominating guard. The
// path-narrowing helper finds no guard pinning the element so the
// rhs stays undecided and the rule drops at this call under the
// under-approximation rule.
//
// vow:nil (!)
func dominatorSliceUnguardedSilent(cfg *Config) { // want dominatorSliceUnguardedSilent:"nilDecl\\(!\\)"
	dominatorSliceCallee(cfg)
}

// declarativeMapKeyCallee declares an implication whose right-hand
// side reads a map value through the receiver's path. The declared
// nest body on Config.Endpoints pins the value layer as non-nil so
// the declarative tier commits the rhs without consulting the SSA
// dominator chain.
//
// vow:cond cfg != nil => cfg.Endpoints["primary"] != nil
func declarativeMapKeyCallee(cfg *Config) {}

// declarativeMapKeyRuleHoldsSilent calls the declarative-tier
// callee without a dominating guard. The field-nil registry's nest
// body pins the map value layer as non-nil so the rhs commits and
// the walker stays silent.
//
// vow:nil (!)
func declarativeMapKeyRuleHoldsSilent(cfg *Config) { // want declarativeMapKeyRuleHoldsSilent:"nilDecl\\(!\\)"
	declarativeMapKeyCallee(cfg)
}

// declarativeMapKeyOppositeCallee declares the opposite-direction
// rule whose rhs asserts the map value is nil. The same field-nil
// registry pins the value layer as non-nil, so the rhs commits as
// false and the rule fails at every reachable call site.
//
// vow:cond cfg != nil => cfg.Endpoints["primary"] == nil
func declarativeMapKeyOppositeCallee(cfg *Config) {}

// declarativeMapKeyRuleViolatesDiagnose pairs the opposite-
// direction callee with a Config the registry pins as non-nil at
// the map value layer. The declarative tier commits the rhs as
// false, the lhs commits as true through the receiver decl, and
// the walker reports the violation at the call.
//
// vow:nil (!)
func declarativeMapKeyRuleViolatesDiagnose(cfg *Config) { // want declarativeMapKeyRuleViolatesDiagnose:"nilDecl\\(!\\)"
	declarativeMapKeyOppositeCallee(cfg) // want `vow\[sentinel-error\]: call violates vow:cond cfg != nil => cfg.Endpoints\["primary"\] == nil`
}

// Inner is the nested struct the multi-step path fixtures walk
// through.
type Inner struct {
	Items []*Server
}

// Outer wraps an Inner pointer so a `outer.Inner.Items[0]` path
// exercises two StepField hops before the StepIndex.
type Outer struct {
	Inner *Inner
}

// multiStepPathCallee declares a rule whose rhs walks two field
// steps and one index step before the nil-check. The dominator
// walker has to unwrap two FieldAddr layers and one IndexAddr
// layer when matching a guard's SSA condition.
//
// vow:cond o != nil => o.Inner.Items[0] != nil
func multiStepPathCallee(o *Outer) {}

// multiStepPathRuleHoldsSilent guards the call with the exact
// path the rule walks. The dominator tier matches the two
// FieldAddr layers and the IndexAddr in reverse, commits the rhs
// as true, and the walker stays silent.
//
// vow:nil (!)
func multiStepPathRuleHoldsSilent(o *Outer) { // want multiStepPathRuleHoldsSilent:"nilDecl\\(!\\)"
	if o.Inner.Items[0] != nil {
		multiStepPathCallee(o)
	}
}

// multiStepPathRuleViolatesDiagnose flips the guard so the
// dominator pins the path-composed value to known-nil. The rhs
// commits as false and the walker reports the violation.
//
// vow:nil (!)
func multiStepPathRuleViolatesDiagnose(o *Outer) { // want multiStepPathRuleViolatesDiagnose:"nilDecl\\(!\\)"
	if o.Inner.Items[0] == nil {
		multiStepPathCallee(o) // want `vow\[sentinel-error\]: call violates vow:cond o != nil => o.Inner.Items\[0\] != nil`
	}
}

// returnPathRuleHoldsSilent declares an implication whose left-
// hand side walks the receiver's slice-index path. The return
// statement sits inside the guarded block so the path-narrowing
// helper commits the lhs as true; the rhs evaluates to a non-nil
// shape through the AST-literal path so the implication holds.
//
// vow:cond cfg.Servers[0] != nil => out != nil
func (cfg *Config) returnPathRuleHoldsSilent() (out *int) {
	if cfg.Servers[0] != nil {
		x := 1
		out = &x
		return out
	}
	return nil
}

// returnPathRuleViolatesDiagnose mirrors the silent baseline but
// returns nil from inside the guarded block. The lhs commits as
// true through the dominator walk and the rhs is statically nil
// so the implication fails and the walker reports the violation.
//
// vow:cond cfg.Servers[0] != nil => out != nil
func (cfg *Config) returnPathRuleViolatesDiagnose() (out *int) {
	if cfg.Servers[0] != nil {
		return nil // want `vow\[sentinel-error\]: return violates vow:cond cfg.Servers\[0\] != nil => out != nil`
	}
	x := 1
	out = &x
	return out
}

// GenConfig carries the map field on a generic container, so a path
// operand that reaches the field through an instantiation holds a
// substituted Object the walker has to resolve back to this
// declaration before the declarative tier can read it.
// The field type names the type parameter, which is what makes the
// instantiation substitute the field and hand the walker an Object
// the origin does not carry.
type GenConfig[T any] struct {
	// vow:nil ![!]!
	Endpoints map[string]*T // want Endpoints:"fieldNil\\(!\\)"

	// vow:nil !
	Ref *T // want Ref:"fieldNil\\(!\\)"
}

// declarativeGenericMapKeyOppositeCallee declares the same
// opposite-direction rule against the instantiated container.
//
// vow:cond cfg != nil => cfg.Endpoints["primary"] == nil
func declarativeGenericMapKeyOppositeCallee(cfg *GenConfig[Endpoint]) {}

// declarativeGenericMapKeyRuleViolatesDiagnose is the instantiated
// counterpart of declarativeMapKeyRuleViolatesDiagnose: the field
// declaration lives on the origin, the call site names it through
// `GenConfig[int]`, and the rule must fail the same way.
//
// vow:nil (!)
func declarativeGenericMapKeyRuleViolatesDiagnose(cfg *GenConfig[Endpoint]) { // want declarativeGenericMapKeyRuleViolatesDiagnose:"nilDecl\\(!\\)"
	declarativeGenericMapKeyOppositeCallee(cfg) // want `vow\[sentinel-error\]: call violates vow:cond cfg != nil => cfg.Endpoints\["primary"\] == nil`
}

// declarativeGenericFieldOppositeCallee declares a rule whose rhs is
// the terminal field itself rather than an element below it, so the
// walker reads the field's own declared nullness.
//
// vow:cond cfg != nil => cfg.Ref == nil
func declarativeGenericFieldOppositeCallee(cfg *GenConfig[Endpoint]) {}

// declarativeGenericFieldRuleViolatesDiagnose pairs that callee with
// a container whose substituted field the registry pins as non-nil,
// so the rhs commits as false and the call reports.
//
// vow:nil (!)
func declarativeGenericFieldRuleViolatesDiagnose(cfg *GenConfig[Endpoint]) { // want declarativeGenericFieldRuleViolatesDiagnose:"nilDecl\\(!\\)"
	declarativeGenericFieldOppositeCallee(cfg) // want `vow\[sentinel-error\]: call violates vow:cond cfg != nil => cfg.Ref == nil`
}

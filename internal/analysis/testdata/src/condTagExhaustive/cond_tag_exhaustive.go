// Package condTagExhaustive is the analysistest fixture for
// the tag-discriminator exhaustiveness layer. The fixture
// exercises four detection paths the layer surfaces: a covered
// typed-string enum (silent baseline), a missing typed-string
// enum case (diagnose), an ad-hoc string literal keyed rule
// pool whose discriminator does not resolve to a typed enum
// (silent baseline), and a biconditional discriminator pool
// whose covered set fails to span the enum (diagnose). Each
// diagnose case pairs with a silent baseline that pins the
// no-false-positive invariant for the same detection path.
package condTagExhaustive

// EventTag is the typed-string enum the fixture's tagged-union
// rule sets discriminate on. The package scope declares three
// constants of that type so the exhaustiveness walk reads the
// candidate set as {"guest", "login", "logout"}.
type EventTag string

const (
	TagGuest  EventTag = "guest"
	TagLogin  EventTag = "login"
	TagLogout EventTag = "logout"
)

// LoginInfo is one of the tagged-union payload slots the fixture
// rule sets thread through their right-hand sides.
type LoginInfo struct{ User string }

// LogoutInfo is one of the tagged-union payload slots the
// fixture rule sets thread through their right-hand sides.
type LogoutInfo struct{ Reason string }

// GuestInfo is one of the tagged-union payload slots the
// fixture rule sets thread through their right-hand sides.
type GuestInfo struct{ Token string }

// Event is the tagged-union carrier the fixture's
// discriminator rule sets target. The Tag field holds the
// EventTag enum value the discriminator reads; the three
// payload pointers are the slots a covered case rule pins as
// non-nil for the matching tag.
type Event struct {
	Tag    EventTag
	Login  *LoginInfo
	Logout *LogoutInfo
	Guest  *GuestInfo
}

// exhaustiveTypedEnumSilent covers every EventTag constant the
// package scope exposes through one biconditional rule per case.
// The covered set spans the candidate set so the walk stays
// silent; the fixture pins that a fully covered tagged-union
// rule pool emits no missing-case diagnostic even when the
// discriminator resolves to a finite typed enum.
//
// vow:cond e.Tag == "login"  <=> e.Login != nil
// vow:cond e.Tag == "logout" <=> e.Logout != nil
// vow:cond e.Tag == "guest"  <=> e.Guest != nil
func exhaustiveTypedEnumSilent(e *Event) {
	_ = e
}

// missingTypedEnumCaseDiagnose covers two of the three EventTag
// constants but omits "guest". The walk resolves e.Tag to the
// EventTag type, enumerates the package-scope constants, finds
// the covered set short of one candidate, and reports the
// missing case at the function declaration.
//
// vow:cond e.Tag == "login"  <=> e.Login != nil
// vow:cond e.Tag == "logout" <=> e.Logout != nil
func missingTypedEnumCaseDiagnose(e *Event) { // want `vow\[sentinel-error\]: vow:cond rules on missingTypedEnumCaseDiagnose omit e.Tag cases: guest`
	_ = e
}

// missingTypedEnumImplyDiagnose covers two of the three
// EventTag constants through forward implications rather than
// biconditionals. The walk admits implication and biconditional
// shapes alike because both pin a positive case binding through
// their equality-comparison left-hand side; only the missing-
// case set against the candidate enum decides whether to
// diagnose.
//
// vow:cond e.Tag == "login"  => e.Login != nil
// vow:cond e.Tag == "logout" => e.Logout != nil
func missingTypedEnumImplyDiagnose(e *Event) { // want `vow\[sentinel-error\]: vow:cond rules on missingTypedEnumImplyDiagnose omit e.Tag cases: guest`
	_ = e
}

// adhocStringKeySilent keys its rule pool on an ad-hoc string
// field whose declared type is plain `string` rather than a
// named typed-string enum. The walk's finite-enum candidate
// lookup reports no enum for an unnamed string slot so the
// detection path leaves the pool silent regardless of the
// rule pool's covered set.
//
// vow:cond p.Kind == "ok"  => p.Result != nil
// vow:cond p.Kind == "err" => p.Err != nil
type plainPayload struct {
	Kind   string
	Result *LoginInfo
	Err    *LogoutInfo
}

func adhocStringKeySilent(p *plainPayload) {
	_ = p
}

// singletonTypedEnumSilent declares a typed string enum whose
// package scope holds a single constant. The walker requires
// two or more candidates before surfacing a missing-case
// diagnostic so a one-value enum stays silent regardless of
// the covered set; the policy keeps the walk silent on type
// definitions whose finite-enum signal is too weak to act on.
//
// vow:cond r.State == "open" => r.Body != nil
type singletonState string

const SingletonOpen singletonState = "open"

type singletonResource struct {
	State singletonState
	Body  *LoginInfo
}

func singletonTypedEnumSilent(r *singletonResource) {
	_ = r
}

// StatusOK and StatusErr declare a two-element `const` block
// whose constants are plain untyped strings rather than members
// of a named typed enum. The fixture's identifier-keyed rules
// resolve their right-hand side through the package scope's
// constant lookup so the missing-case walk reaches the
// untyped-sibling candidate path.
const (
	StatusOK  = "ok"
	StatusErr = "err"
)

// plainResult is the carrier the untyped-sibling rule sets
// discriminate on. The Status field is plain `string` so the
// typed-enum candidate path stays silent and the walk falls
// back to the untyped-sibling candidate path.
type plainResult struct {
	Status string
	Data   *LoginInfo
	Err    *LogoutInfo
}

// untypedSiblingCoveredSilent declares one rule per element of
// the `(StatusOK, StatusErr)` const block. The identifier-keyed
// right-hand sides resolve through the package scope so the
// covered set spans the block's sibling values; the walk stays
// silent because no candidate is missing.
//
// vow:cond r.Status == StatusOK  => r.Data != nil
// vow:cond r.Status == StatusErr => r.Err != nil
func untypedSiblingCoveredSilent(r *plainResult) {
	_ = r
}

// missingUntypedSiblingDiagnose covers `StatusOK` but omits the
// `StatusErr` sibling declared in the same const block. The
// walk's typed-enum path stays silent because the discriminator
// is plain `string`; the untyped-sibling path seeds the
// candidate set from the resolved `StatusOK` constant and
// reports the missing `err` sibling at the function
// declaration.
//
// vow:cond r.Status == StatusOK => r.Data != nil
// vow:cond r.Status == StatusOK => r.Err == nil
func missingUntypedSiblingDiagnose(r *plainResult) { // want `vow\[sentinel-error\]: vow:cond rules on missingUntypedSiblingDiagnose omit r.Status cases: err`
	_ = r
}

// mixedLiteralAndIdentifierCoveredSilent pairs a quoted-literal
// right-hand side with an identifier right-hand side. The
// quoted spelling resolves through Unquote; the identifier
// spelling resolves through the package scope to the same
// string value the typed-enum candidate path enumerates. The
// covered set collapses both spellings to the same enum slot
// so the walk reports no missing case once the discriminator
// resolves to the typed-string enum.
//
// vow:cond e.Tag == "login"  <=> e.Login != nil
// vow:cond e.Tag == TagLogout <=> e.Logout != nil
// vow:cond e.Tag == TagGuest  <=> e.Guest != nil
func mixedLiteralAndIdentifierCoveredSilent(e *Event) {
	_ = e
}

// untypedSingletonConstSilent declares an identifier-keyed rule
// against a const block whose only element is `SoloState`. The
// untyped-sibling candidate path returns a one-element set and
// the candidate threshold (two or more) drops the set so the
// walk stays silent; the policy keeps the walk silent on
// authoring shapes whose finite-enum signal is too weak to act
// on.
//
// vow:cond r.Status == SoloState => r.Data != nil
// vow:cond r.Status == SoloState => r.Err == nil
const SoloState = "solo"

func untypedSingletonConstSilent(r *plainResult) {
	_ = r
}

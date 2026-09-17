package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// signatureSubjectKind labels the signature slot a marker subject
// resolved against. The signatureSubjectUnknown zero value lets
// callers treat "no resolution yet" and "unresolvable subject" as a
// single branch without a separate flag.
type signatureSubjectKind int

const (
	signatureSubjectUnknown  signatureSubjectKind = iota
	signatureSubjectReceiver                      // the function's receiver field
	signatureSubjectParam                         // a regular parameter slot
	signatureSubjectResult                        // a named return slot
)

// String renders the kind in a form suitable for diagnostic prose
// (`"parameter"`, `"return value"`, ...). The unknown branch
// reports an empty string so diagnostics that only fire on a
// resolved subject never embed the fallback wording.
func (k signatureSubjectKind) String() string {
	switch k {
	case signatureSubjectReceiver:
		return "receiver"
	case signatureSubjectParam:
		return "parameter"
	case signatureSubjectResult:
		return "return value"
	}
	return ""
}

// signatureSubjectRef points at the signature slot a marker's
// `[subject]` scope resolved against. Field carries the *ast.Field
// the named identifier lives on so downstream passes can inspect
// the slot's declared type when wiring the higher-order contract.
// Ident is the specific *ast.Ident inside Field.Names that matched
// — when one field declares multiple names (`a, b int`), Ident
// pins the match to the right one. Index is the slot position
// (0-based) within its kind; the receiver always reports 0 because
// Go signatures expose at most one receiver.
type signatureSubjectRef struct {
	Kind  signatureSubjectKind
	Index int
	Field *ast.Field
	Ident *ast.Ident
}

// resolveSignatureSubject looks subject up against fn's receiver,
// parameter list, and named return list. A subject spelt as a
// bare identifier matches against the declared name of a slot
// (named slots only). A subject spelt as `$N` (1-based integer
// prefixed by a dollar sign) matches positionally against the
// return list, so the resolver reaches an unnamed callback that
// the function exposes as its N-th return value. Receiver names
// take precedence over parameter names, and parameter names over
// return names — duplicates across slots are an authoring error
// the Go compiler already flags, so the resolver returns the
// first site it visits without scanning later slots.
//
// An empty subject reports ok=false with a zero ref so callers can
// treat "marker carried no scope" and "subject did not resolve"
// uniformly via the boolean. A non-empty subject that does not
// match any named slot returns the zero ref with ok=false so the
// caller can emit a diagnostic at the marker's anchoring site.
func resolveSignatureSubject(fn *ast.FuncDecl, subject string) (signatureSubjectRef, bool) {
	if fn == nil || subject == "" {
		return signatureSubjectRef{}, false
	}
	if strings.HasPrefix(subject, "$") {
		return resolveReturnValueSubject(fn, subject)
	}
	if ref, ok := lookupSubjectInFieldList(fn.Recv, subject, signatureSubjectReceiver); ok {
		return ref, true
	}
	if fn.Type != nil {
		if ref, ok := lookupSubjectInFieldList(fn.Type.Params, subject, signatureSubjectParam); ok {
			return ref, true
		}
		if ref, ok := lookupSubjectInFieldList(fn.Type.Results, subject, signatureSubjectResult); ok {
			return ref, true
		}
	}
	return signatureSubjectRef{}, false
}

// resolveReturnValueSubject matches a `$N` subject against fn's
// return slot at the 1-based index N. The helper expands a
// grouped declaration (`a, b T`) into one logical slot per name
// so the index reports the position the author would point at,
// not the position of the field group. An index outside the
// return list, or a malformed positional spelling, reports
// ok=false so the caller surfaces the standard subject-
// unresolved diagnostic. The slot is reported with kind
// signatureSubjectResult; downstream callback-shape checks
// confirm whether the return value is function-typed.
func resolveReturnValueSubject(fn *ast.FuncDecl, subject string) (signatureSubjectRef, bool) {
	n, err := strconv.Atoi(subject[1:])
	if err != nil || n < 1 {
		return signatureSubjectRef{}, false
	}
	if fn.Type == nil || fn.Type.Results == nil {
		return signatureSubjectRef{}, false
	}
	target := n - 1
	index := 0
	for _, field := range fn.Type.Results.List {
		count := len(field.Names)
		if count == 0 {
			if index == target {
				return signatureSubjectRef{
					Kind:  signatureSubjectResult,
					Index: index,
					Field: field,
				}, true
			}
			index++
			continue
		}
		for _, name := range field.Names {
			if index == target {
				return signatureSubjectRef{
					Kind:  signatureSubjectResult,
					Index: index,
					Field: field,
					Ident: name,
				}, true
			}
			index++
		}
	}
	return signatureSubjectRef{}, false
}

// lookupSubjectInFieldList walks list and returns the first field
// whose declared names include subject. The function expands
// grouped declarations (`a, b T`) into one logical slot per name so
// Index reports the position the author would point at, not the
// position of the field group. A nil list reports no match without
// inspecting fields.
func lookupSubjectInFieldList(list *ast.FieldList, subject string, kind signatureSubjectKind) (signatureSubjectRef, bool) {
	if list == nil {
		return signatureSubjectRef{}, false
	}
	index := 0
	for _, field := range list.List {
		if len(field.Names) == 0 {
			index++
			continue
		}
		for _, name := range field.Names {
			if name.Name == subject {
				return signatureSubjectRef{
					Kind:  kind,
					Index: index,
					Field: field,
					Ident: name,
				}, true
			}
			index++
		}
	}
	return signatureSubjectRef{}, false
}

// chainResolutionStatus classifies the outcome of a subject chain
// walk through nested callback signatures. The zero value is
// chainResolved so a successful walk needs no explicit assignment.
type chainResolutionStatus int

const (
	// chainResolved means every chain segment matched a signature
	// slot. The walker's returned signature is non-nil only when the
	// innermost slot is itself callback-typed; cond_subject and
	// similar consumers check the signature for nil before driving
	// body validation against it.
	chainResolved chainResolutionStatus = iota
	// chainResolveFailed means a segment did not name any slot in the
	// signature at its chain depth. The walker reports the depth so
	// the caller can surface a diagnostic that names the failing
	// segment.
	chainResolveFailed
	// chainStepFailed means an intermediate segment resolved to a
	// slot that is not callback-typed, so the chain cannot step
	// inward to the next segment. The walker reports the depth of
	// the intermediate slot.
	chainStepFailed
)

// resolveSignatureSubjectChain walks chain through nested callback
// signatures starting from fn. The first segment resolves against
// fn's receiver, parameter list, and named return list; each
// non-head segment resolves against the callback signature carried
// by the previous segment's slot.
//
// On a successful walk the helper returns the callback signature of
// the innermost segment's slot — non-nil when that slot is itself
// callback-typed, nil otherwise — together with failedAt == -1 and
// status == chainResolved. cond_subject and similar consumers
// validate marker body references against the returned signature
// when it is non-nil.
//
// A segment that does not match any slot at its chain depth stops
// the walk and reports status == chainResolveFailed with failedAt
// set to that segment's 0-based index. An intermediate slot that is
// not callback-typed stops the walk and reports status ==
// chainStepFailed with failedAt set to the intermediate slot's
// 0-based index.
//
// An empty chain reports chainResolveFailed at depth 0; the helper
// treats the absence of a chain as a resolution failure so callers
// route empty input through the same diagnostic surface.
func resolveSignatureSubjectChain(pass *analysis.Pass, fn *ast.FuncDecl, chain []string) (sig *types.Signature, failedAt int, status chainResolutionStatus) {
	if len(chain) == 0 || fn == nil {
		return nil, 0, chainResolveFailed
	}
	ref, ok := resolveSignatureSubject(fn, chain[0])
	if !ok {
		return nil, 0, chainResolveFailed
	}
	currentSig, _ := callbackSignature(pass, ref)
	for depth := 1; depth < len(chain); depth++ {
		if currentSig == nil {
			return nil, depth - 1, chainStepFailed
		}
		slot, found := lookupCallbackSlot(currentSig, chain[depth])
		if !found {
			return nil, depth, chainResolveFailed
		}
		next, isCallback := callbackSignatureOfSlot(slot)
		if !isCallback && depth < len(chain)-1 {
			return nil, depth, chainStepFailed
		}
		currentSig = next
	}
	return currentSig, -1, chainResolved
}

// lookupCallbackSlot scans sig's receiver, parameters, and named
// returns for a slot whose declared name matches subject. The
// helper also recognises the `$N` positional reference (1-based)
// that addresses the N-th return slot of sig. A nil sig or
// unmatched subject reports ok=false. The returned *types.Var
// carries the slot's declared type so the caller can step into the
// next chain segment when the type is itself a function signature.
func lookupCallbackSlot(sig *types.Signature, subject string) (slot *types.Var, ok bool) {
	if sig == nil || subject == "" {
		return nil, false
	}
	if strings.HasPrefix(subject, "$") {
		return lookupCallbackPositional(sig, subject)
	}
	if recv := sig.Recv(); recv != nil && recv.Name() == subject {
		return recv, true
	}
	if params := sig.Params(); params != nil {
		for i := 0; i < params.Len(); i++ {
			if v := params.At(i); v.Name() == subject {
				return v, true
			}
		}
	}
	if results := sig.Results(); results != nil {
		for i := 0; i < results.Len(); i++ {
			if v := results.At(i); v.Name() == subject {
				return v, true
			}
		}
	}
	return nil, false
}

// lookupCallbackPositional matches a `$N` subject against sig's
// return list by index. A malformed positional spelling or an out-
// of-range index reports ok=false so the caller routes the failure
// through the standard chainResolveFailed branch.
func lookupCallbackPositional(sig *types.Signature, subject string) (*types.Var, bool) {
	n, err := strconv.Atoi(subject[1:])
	if err != nil || n < 1 {
		return nil, false
	}
	results := sig.Results()
	if results == nil || n > results.Len() {
		return nil, false
	}
	return results.At(n - 1), true
}

// callbackSignatureOfSlot unwraps slot's declared type to a
// function signature when the type reduces to a function shape.
// Non-function types (pointers, structs, channels) report
// ok=false so the chain walker stops at the intermediate slot
// rather than stepping into a type it cannot navigate.
func callbackSignatureOfSlot(slot *types.Var) (*types.Signature, bool) {
	if slot == nil {
		return nil, false
	}
	t := slot.Type()
	if t == nil {
		return nil, false
	}
	sig, ok := t.Underlying().(*types.Signature)
	if !ok {
		return nil, false
	}
	return sig, true
}

// formatSubjectChain renders chain as the `[X][Y]...` surface text
// the author wrote, so diagnostics quote the chain in the same
// bracket form the marker line carries.
func formatSubjectChain(chain []string) string {
	var b strings.Builder
	for _, s := range chain {
		b.WriteByte('[')
		b.WriteString(s)
		b.WriteByte(']')
	}
	return b.String()
}

// reportChainSubjectUnresolved emits a diagnostic at pos for a
// marker whose chain segment did not match any slot at its chain
// depth. The marker argument identifies the family so the helper
// picks the matching diagnostic category — vow:nil claims surface
// under `vow[nil-safety]`, every other family under
// `vow[sentinel-error]`. A single-element chain routes through the
// reportSubjectUnresolved diagnostic so the concise message
// `vow:cond[X] subject does not name ...` covers the common case;
// a multi-element chain quotes the full `[X][Y]...` qualifier and
// names the failing segment by index so the author sees which
// level of the chain blocked the walk.
func reportChainSubjectUnresolved(pass *analysis.Pass, pos token.Pos, marker string, chain []string, failedAt int) {
	if len(chain) == 1 {
		reportSubjectUnresolved(pass, pos, marker, chain[0])
		return
	}
	scope := "the enclosing function"
	if failedAt > 0 {
		scope = "the callback signature"
	}
	format := "vow[sentinel-error]: %s%s subject %q at chain depth %d does not name a receiver, parameter, or named return of %s"
	if marker == nilDeclMarker {
		vowReportNilSafetyf(pass, pos,
			"vow[nil-safety]: %s%s subject %q at chain depth %d does not name a receiver, parameter, or named return of %s",
			marker, formatSubjectChain(chain), chain[failedAt], failedAt, scope)
		return
	}
	vowReportf(pass, pos, format,
		marker, formatSubjectChain(chain), chain[failedAt], failedAt, scope)
}

// reportChainStepNotCallback emits a diagnostic at pos for a
// chained marker whose intermediate slot is not callback-typed, so
// the chain cannot step inward to the next segment. The diagnostic
// quotes the full chain and names the intermediate segment by
// index so the author sees which slot blocked the inward step.
func reportChainStepNotCallback(pass *analysis.Pass, pos token.Pos, marker string, chain []string, failedAt int) {
	format := "vow[sentinel-error]: %s%s subject %q at chain depth %d is not a callback; chain only steps through function-typed slots"
	if marker == nilDeclMarker {
		vowReportNilSafetyf(pass, pos,
			"vow[nil-safety]: %s%s subject %q at chain depth %d is not a callback; chain only steps through function-typed slots",
			marker, formatSubjectChain(chain), chain[failedAt], failedAt)
		return
	}
	vowReportf(pass, pos, format,
		marker, formatSubjectChain(chain), chain[failedAt], failedAt)
}

// reportSubjectChainNotCallback emits a diagnostic at pos for a
// marker whose innermost slot resolves cleanly but is not
// callback-typed, so the family's higher-order contract cannot
// host the declaration. The trailing reason describes why the
// callback shape matters for the family (`emission only propagates
// through function-typed slots`, `discharge only propagates...`,
// `the contract only carries through a function-typed slot`).
// A single-element chain quotes the concise `vow:emit[X] subject
// is not a callback; <reason>` form; a multi-element chain
// quotes the full `[X][Y]...` qualifier so the author traces the
// innermost slot.
func reportSubjectChainNotCallback(pass *analysis.Pass, pos token.Pos, marker string, chain []string, reason string) {
	if len(chain) == 1 {
		if marker == nilDeclMarker {
			vowReportNilSafetyf(pass, pos,
				"vow[nil-safety]: %s[%s] subject is not a callback; %s",
				marker, chain[0], reason)
			return
		}
		vowReportf(pass, pos,
			"vow[sentinel-error]: %s[%s] subject is not a callback; %s",
			marker, chain[0], reason)
		return
	}
	if marker == nilDeclMarker {
		vowReportNilSafetyf(pass, pos,
			"vow[nil-safety]: %s%s innermost subject is not a callback; %s",
			marker, formatSubjectChain(chain), reason)
		return
	}
	vowReportf(pass, pos,
		"vow[sentinel-error]: %s%s innermost subject is not a callback; %s",
		marker, formatSubjectChain(chain), reason)
}

// reportCallerEmitChainUnresolved emits a `vow[closable]` chain-
// unresolved diagnostic for statement-scope `vow:emit`. The
// caller-side surface routes its subject failures through
// `vow[closable]` rather than the function-doc family's
// `vow[sentinel-error]`; a single-element chain keeps the concise
// per-segment phrasing the existing caller-side check used so the
// statement-scope diagnostic does not change shape for unchained
// authoring.
func reportCallerEmitChainUnresolved(pass *analysis.Pass, pos token.Pos, chain []string, failedAt int) {
	if len(chain) == 1 {
		vowReportf(pass, pos,
			"vow[closable]: %s[%s] subject does not name a receiver, parameter, or named return of the enclosing function",
			emitMarker, chain[0])
		return
	}
	scope := "the enclosing function"
	if failedAt > 0 {
		scope = "the callback signature"
	}
	vowReportf(pass, pos,
		"vow[closable]: %s%s subject %q at chain depth %d does not name a receiver, parameter, or named return of %s",
		emitMarker, formatSubjectChain(chain), chain[failedAt], failedAt, scope)
}

// reportSubjectUnresolved emits a diagnostic at pos for a marker
// whose `[subject]` scope did not match any named slot in the
// enclosing signature. The marker argument identifies which marker
// family raised the failure so the author sees one diagnostic per
// offending claim, and so the helper can pick the diagnostic
// category that matches the family — vow:nil claims surface under
// the `vow[nil-safety]` axis the same way malformed nil signatures
// do, while every other marker family routes through
// `vow[sentinel-error]`.
func reportSubjectUnresolved(pass *analysis.Pass, pos token.Pos, marker, subject string) {
	if marker == nilDeclMarker {
		vowReportNilSafetyf(
			pass,
			pos,
			"vow[nil-safety]: %s[%s] subject does not name a receiver, parameter, or named return of the enclosing function",
			marker,
			subject,
		)
		return
	}
	vowReportf(
		pass,
		pos,
		"vow[sentinel-error]: %s[%s] subject does not name a receiver, parameter, or named return of the enclosing function",
		marker,
		subject,
	)
}

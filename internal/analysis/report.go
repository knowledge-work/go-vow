package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/cli"
)

// passStates registers the *passState bound to each in-flight
// analyzer Pass so vowReport's filter chain can read the
// suppression sets without threading state through every emission
// site. runFunc stores the binding at entry and clears it on
// return, so the map size is bounded by the number of concurrent
// passes (typically one per package) and the registry never
// outlives a single analyzer run.
var passStates sync.Map

// registerPassState publishes state on the per-pass registry and
// returns a cleanup function the caller defers. The returned
// function deregisters the binding so a recycled Pass pointer in a
// later run never reads through to a stale state.
func registerPassState(pass *analysis.Pass, state *passState) func() {
	passStates.Store(pass, state)
	return func() { passStates.Delete(pass) }
}

// lookupPassState returns the *passState bound to pass, or nil when
// vowReport runs outside a registered pass (e.g. unit tests that
// invoke vowReport with a fresh Pass that never went through
// runFunc). A nil return makes vowReport behave as a transparent
// pass-through to pass.Report so the caller-side filters never
// observe orphan diagnostics from those harnesses.
func lookupPassState(pass *analysis.Pass) *passState {
	v, ok := passStates.Load(pass)
	if !ok {
		return nil
	}
	return v.(*passState)
}

// vowReport is the single entry point for every diagnostic the vow
// analyzer emits. The filter chain runs in order: first the narrow-
// scope drop driven by --changed-files / --with-callers, then the
// all-verification barrier for discharged callees, then the must-
// consume drop for suppress sites, then the caller-side discharge
// assertion drop for vow:use markers. The narrow-scope filter
// consults cli.Active() and applies regardless of whether the pass
// was registered through runFunc, so test harnesses observe it
// whenever cli.SetActive publishes a non-empty ChangedFileSet. The
// remaining filters consult the per-pass state registered by
// runFunc; diagnostics whose Pass has no registered passState (test
// harnesses, embedded reuse) bypass the state-driven filters and
// surface unchanged. Emission-point short-circuits in the SSA leak
// walk and the return-position validator remain in place as
// redundant defence; the chain catches diagnostics that future
// emission paths might reach without a short-circuit of their own.
func vowReport(pass *analysis.Pass, diag analysis.Diagnostic) {
	if cli.AnnotateNarrow() {
		// The scope decision belongs to the process holding the
		// changed-file set, so this pass supplies its inputs instead of
		// deciding.
		diag.Related = append(diag.Related, analysis.RelatedInformation{
			Pos:     diag.Pos,
			Message: narrowAnnotation(pass, diag),
		})
	} else if shouldDropByNarrowScope(pass, diag) {
		return
	}
	state := lookupPassState(pass)
	if state != nil && shouldDropByDischarged(pass, state, diag) {
		return
	}
	if state != nil && shouldSuppressBySuppress(pass, state, diag) {
		return
	}
	if state != nil && shouldDropByUseAssertion(pass, state, diag) {
		return
	}
	pass.Report(diag)
}

// shouldDropByDischarged reports whether diag was emitted inside a
// call to a function declared as a barrier callee. Both the SSA
// leak walk and the return-position validator already
// short-circuit on barrier boundaries, so this filter fires only
// when a future emission path reaches vowReport without a
// short-circuit of its own. Keeping the check in the chain makes
// the all-verification invariant a property of the dispatcher
// rather than of every emission site. Diagnostics whose Pos is
// not enclosed by any call (function-level reports, parser
// errors) surface unchanged.
func shouldDropByDischarged(pass *analysis.Pass, state *passState, diag analysis.Diagnostic) bool {
	call := enclosingCallExpr(pass, diag.Pos)
	if call == nil {
		return false
	}
	return isDischargedASTCall(call, pass, state)
}

// enclosingCallExpr returns the innermost CallExpr in pass.Files
// whose source range contains pos, or nil when pos is outside
// every call. The search walks each file once and records the
// deepest match so an emission inside a nested call resolves to
// its closest enclosing call rather than an outer one. The
// containing file is found through token positions rather than a
// FileSet lookup, keeping the helper independent of the analyzer
// run's FileSet state.
func enclosingCallExpr(pass *analysis.Pass, pos token.Pos) *ast.CallExpr {
	for _, file := range pass.Files {
		if pos < file.Pos() || pos > file.End() {
			continue
		}
		var found *ast.CallExpr
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				return false
			}
			if pos < n.Pos() || pos > n.End() {
				return false
			}
			if call, ok := n.(*ast.CallExpr); ok {
				found = call
			}
			return true
		})
		return found
	}
	return nil
}

// vowReportf builds a diagnostic from a position and a formatted
// message and dispatches it through vowReport. Call sites route
// through this helper instead of pass.Reportf so any filter
// installed in vowReport observes both struct-form and formatted
// emissions on the same path.
func vowReportf(pass *analysis.Pass, pos token.Pos, format string, args ...any) {
	vowReport(pass, analysis.Diagnostic{
		Pos:     pos,
		Message: fmt.Sprintf(format, args...),
	})
}

// vowReportNilSafetyf is the nil-safety variant of vowReportf.
// Diagnostics emitted here carry the nil-safety category tag so the
// vow:suppress filter can identify and drop them. The surface is
// the caller-side argument and receiver nil checks; non-nil-safety
// emissions continue to flow through vowReportf unchanged.
func vowReportNilSafetyf(pass *analysis.Pass, pos token.Pos, format string, args ...any) {
	vowReport(pass, analysis.Diagnostic{
		Pos:      pos,
		Category: categoryNilSafety,
		Message:  fmt.Sprintf(format, args...),
	})
}

// vowReportNilDeclf is the declaration-completeness variant of
// vowReportf. Diagnostics emitted here carry the nil-decl category so
// the vow:suppress filter can drop them and so a CI integration can
// separate them from the nil-safety findings.
func vowReportNilDeclf(pass *analysis.Pass, pos token.Pos, format string, args ...any) {
	vowReport(pass, analysis.Diagnostic{
		Pos:      pos,
		Category: categoryNilDecl,
		Message:  fmt.Sprintf(format, args...),
	})
}

// vowReportMustConsumef is the must-consume variant of vowReportf.
// Diagnostics emitted here carry a must-consume tag in Category so
// the suppress filter can identify and drop them; the same Category
// also encodes the subject (sentinel name) so the use-assertion
// filter can match the caller-side `// vow:use X` override against
// the specific subject the leak names. Other diagnostic categories
// (return-position, annotation-shape, parser errors) continue to
// flow through vowReportf unchanged.
func vowReportMustConsumef(pass *analysis.Pass, pos token.Pos, subject, format string, args ...any) {
	vowReport(pass, analysis.Diagnostic{
		Pos:      pos,
		Category: mustConsumeCategoryFor(subject),
		Message:  fmt.Sprintf(format, args...),
	})
}

// mustConsumeCategoryFor returns the Diagnostic.Category string a
// must-consume diagnostic should carry given its subject. A non-
// empty subject is appended after a `:` so downstream filters can
// extract it without parsing the human-readable message; an empty
// subject (which is unreachable from the analyzer's own
// emission sites, but kept supported as a defensive fallback)
// produces the bare category string.
func mustConsumeCategoryFor(subject string) string {
	if subject == "" {
		return categoryMustConsume
	}
	return categoryMustConsume + ":" + subject
}

// isMustConsumeCategory reports whether cat names a must-consume
// diagnostic. The bare `must-consume` form and the subject-encoded
// `must-consume:SUBJECT` form both qualify, so filters that care
// about category alone can treat them uniformly.
func isMustConsumeCategory(cat string) bool {
	return cat == categoryMustConsume || strings.HasPrefix(cat, categoryMustConsume+":")
}

// subjectFromMustConsumeCategory extracts the subject the diagnostic
// names, or the empty string when cat is bare or non-must-consume.
// The use-assertion filter consults this helper to match a caller-
// side `// vow:use X` hint against the diagnostic's subject; the
// suppress filter ignores the return value because it drops every
// must-consume diagnostic regardless of subject.
func subjectFromMustConsumeCategory(cat string) string {
	prefix := categoryMustConsume + ":"
	if !strings.HasPrefix(cat, prefix) {
		return ""
	}
	return strings.TrimPrefix(cat, prefix)
}

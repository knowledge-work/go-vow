package analysis

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// validateSubjectScopedNilDecls walks every function-doc
// `vow:nil[subject]` marker and surfaces two additional
// diagnostics that the function-level discovery walk leaves
// unreported. The first reports when the resolved subject is not
// function-typed — a `vow:nil` contract only carries through a
// callback signature, so a non-function slot cannot host the
// declaration. The second reports when the contract names more
// positions than the callback's own signature exposes; the
// enclosing function's arity check is skipped on subject-scoped
// decls (the contract describes the callback's signature, not the
// enclosing one) so this walker fills the missing safety net.
//
// The walker re-parses the marker payload through
// `parseSubjectScopedNilSignature` rather than calling
// `collectNilSignature` so parse-error and duplicate-marker
// diagnostics — already surfaced once by `userNilDecls` — do not
// double-report.
func validateSubjectScopedNilDecls(pass *analysis.Pass, state *passState) {
	_ = state
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}
			subject, sig, ok := parseSubjectScopedNilSignature(fn)
			if !ok {
				continue
			}
			validateSubjectScopedNilSignature(pass, fn, subject, sig)
		}
	}
}

// parseSubjectScopedNilSignature returns the first well-formed
// `vow:nil[subject]` marker on fn's doc. The walker shares the
// payload-stripping rule with `collectNilSignature` but stays
// silent on parse errors so the diagnostic responsibility stays
// with the function-level collector. The first valid scoped
// signature wins; later markers are silently dropped because
// `collectNilSignature` already reports the "more than one
// marker" diagnostic on the duplicates.
func parseSubjectScopedNilSignature(fn *ast.FuncDecl) (dsl.SubjectChain, *dsl.NilSignature, bool) {
	for _, c := range fn.Doc.List {
		line := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		chain, payload, ok := stripMarkerScopeChain(line, nilDeclMarker)
		if !ok || payload == "" || len(chain) == 0 {
			continue
		}
		parsed, err := dsl.ParseNilSignature(payload)
		if err != nil || parsed == nil {
			continue
		}
		parsed.Subject = dsl.SubjectChain(chain)
		return parsed.Subject, parsed, true
	}
	return nil, nil, false
}

// validateSubjectScopedNilSignature surfaces the callback-type
// and callback-arity diagnostics for a parsed subject-scoped
// signature. The helper splits the report sites by family: a
// non-callback subject anchors at the function declaration so the
// author sees the type mismatch alongside the contract; arity
// mismatches (in either direction) on parameter or return
// positions anchor at the same line so the author sees the count
// discrepancy in one glance. A chain-step failure (intermediate
// non-callback) routes to the chain-step diagnostic so the
// offending depth is named in the message. Both axes go through
// arityAcceptable.
func validateSubjectScopedNilSignature(pass *analysis.Pass, fn *ast.FuncDecl, chain dsl.SubjectChain, sig *dsl.NilSignature) {
	cbSig, failedAt, status := resolveSignatureSubjectChain(pass, fn, chain)
	if status == chainResolveFailed {
		return
	}
	if status == chainStepFailed {
		reportChainStepNotCallback(pass, fn.Pos(), nilDeclMarker, chain, failedAt)
		return
	}
	if cbSig == nil {
		reportSubjectChainNotCallback(pass, fn.Pos(), nilDeclMarker, chain,
			"the contract only carries through a function-typed slot")
		return
	}
	if !arityAcceptable(len(sig.Params), cbSig.Params().Len()) {
		vowReportNilSafetyf(
			pass,
			fn.Pos(),
			"vow[nil-safety]: vow:nil%s declares %d parameter positions but the callback signature has %d",
			formatSubjectChain(chain),
			len(sig.Params),
			cbSig.Params().Len(),
		)
	}
	if !arityAcceptable(len(sig.Returns), cbSig.Results().Len()) {
		vowReportNilSafetyf(
			pass,
			fn.Pos(),
			"vow[nil-safety]: vow:nil%s declares %d return positions but the callback signature has %d",
			formatSubjectChain(chain),
			len(sig.Returns),
			cbSig.Results().Len(),
		)
	}
}

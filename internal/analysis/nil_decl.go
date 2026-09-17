package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// nilDeclMarker is the doc-comment marker that introduces a
// signature-mirror nil declaration. The payload after the marker
// follows the grammar parsed by dsl.ParseNilSignature: an
// optional receiver decl, a parenthesised parameter list, and an
// optional comma-separated return list.
const nilDeclMarker = "vow:nil"

// reasonNilDecl is the human-readable rationale a nil-safety
// diagnostic uses when the vow:nil signature authorised the
// reporting. Centralised next to reasonFieldNilDecl so the two
// reason strings stay aligned at one source.
const reasonNilDecl = "callee declared non-nil at this position via vow:nil"

// userNilDecls walks every FuncDecl in pass.Files and returns a
// map from each function's type-checker Object to the parsed
// nil-signature it carries. Functions whose doc holds no
// well-formed `vow:nil` marker do not appear in the map; a
// malformed payload surfaces a transducer-style diagnostic at the
// function declaration so the parse error never silently disables
// the contract.
//
// Declarations whose subject scope is non-empty are excluded from
// the function-level map because they describe a higher-order
// contract a layer wires to executable semantics; folding
// them into the same map would misroute the contract as the
// enclosing function's own nil signature.
//
// Parsing goes through dsl.ParseNilSignature directly rather than
// the analyzer's parsePayloadByMarker path so it does not depend
// on the file's RuleScope. `vow:nil` carries no rule references,
// so RuleScope is irrelevant to its parse.
func userNilDecls(pass *analysis.Pass) map[types.Object]*dsl.NilSignature {
	result := map[types.Object]*dsl.NilSignature{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}
			sig, ok := collectNilSignature(pass, fn)
			if !ok {
				continue
			}
			if len(sig.Subject) > 0 {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			if obj == nil {
				continue
			}
			result[obj] = sig
		}
	}
	return result
}

// userInterfaceMethodNilDecls walks every interface method Field in
// pass.Files and returns a map from each method's type-checker
// Object to its parsed nil-signature, mirroring userNilDecls at
// method granularity. `TypesInfo.Defs` resolves an interface
// method's name identifier to a `*types.Func` exactly as it does
// for a top-level function, so the returned map keys into the same
// Object space the caller-side checks and the Facts mechanism
// already consult.
func userInterfaceMethodNilDecls(pass *analysis.Pass) map[types.Object]*dsl.NilSignature {
	result := map[types.Object]*dsl.NilSignature{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				iface, ok := ts.Type.(*ast.InterfaceType)
				if !ok || iface.Methods == nil {
					continue
				}
				for _, field := range iface.Methods.List {
					collectInterfaceMethodNilDecl(pass, field, result)
				}
			}
		}
	}
	return result
}

// collectInterfaceMethodNilDecl parses field's `vow:nil` marker (if
// any) and records the parsed signature into result, keyed by the
// method's type-checker Object. An embedded-interface element
// carries no method name (field.Names is empty) and is skipped by
// the arity check; a defensive *ast.FuncType assertion also guards
// against any other non-method interface element the grammar might
// carry. Both shapes are out of scope for a signature-mirror
// declaration and are skipped silently.
func collectInterfaceMethodNilDecl(pass *analysis.Pass, field *ast.Field, result map[types.Object]*dsl.NilSignature) {
	if field.Doc == nil || len(field.Names) != 1 {
		return
	}
	ft, ok := field.Type.(*ast.FuncType)
	if !ok {
		return
	}
	sig, ok := collectInterfaceMethodNilSignature(pass, field, ft)
	if !ok {
		return
	}
	obj := pass.TypesInfo.Defs[field.Names[0]]
	if obj == nil {
		return
	}
	result[obj] = sig
}

// collectInterfaceMethodNilSignature walks field's doc comments for
// the first well-formed `vow:nil` marker and returns the parsed
// signature. The parse-error and duplicate-marker diagnostics
// mirror collectNilSignature; the anchor is the field's own
// position because an interface method has no enclosing
// *ast.FuncDecl to anchor at.
//
// A marker that carries a `[subject]` scope is rejected with a
// diagnostic rather than silently dropped: the subject-scope
// mechanism retargets a contract at a named callback parameter's
// signature, and resolving that requires a function body to bind
// the callback against, which an interface method declaration does
// not have. Surfacing the rejection keeps a mistaken `vow:nil[cb]`
// on an interface method from looking like a no-op.
func collectInterfaceMethodNilSignature(pass *analysis.Pass, field *ast.Field, ft *ast.FuncType) (*dsl.NilSignature, bool) {
	var (
		sig  *dsl.NilSignature
		seen bool
	)
	for _, c := range field.Doc.List {
		line := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		chain, payload, ok := stripMarkerScopeChain(line, nilDeclMarker)
		if !ok {
			continue
		}
		if len(chain) > 0 {
			vowReportNilSafetyf(pass, field.Pos(), "vow[nil-safety]: %s: subject-scoped markers are not supported on interface methods", nilDeclMarker)
			continue
		}
		if payload == "" {
			continue
		}
		parsed, err := dsl.ParseNilSignature(payload)
		if err != nil {
			vowReportNilSafetyf(pass, field.Pos(), "vow[nil-safety]: %s: %v", nilDeclMarker, err)
			continue
		}
		if seen {
			vowReportNilSafetyf(pass, field.Pos(), "vow[nil-safety]: interface method carries more than one %s marker; keeping the first", nilDeclMarker)
			continue
		}
		sig = parsed
		seen = true
	}
	if !seen {
		return nil, false
	}
	checkInterfaceMethodSignatureArity(pass, field, ft, sig)
	checkNilDeclSignatureLayers(pass, field.Pos(), ft, sig)
	return sig, true
}

// checkInterfaceMethodSignatureArity reports when the parsed
// signature-mirror declaration's slot count differs from the
// interface method's Go signature — in either direction —
// mirroring checkSignatureArity at method granularity, including
// the empty-expression exception on both axes.
func checkInterfaceMethodSignatureArity(pass *analysis.Pass, field *ast.Field, ft *ast.FuncType, sig *dsl.NilSignature) {
	if sig == nil {
		return
	}
	paramCount := fieldListPositionCount(ft.Params)
	if !arityAcceptable(len(sig.Params), paramCount) {
		vowReportNilSafetyf(
			pass,
			field.Pos(),
			"vow[nil-safety]: %s declares %d parameter positions but the signature has %d",
			nilDeclMarker,
			len(sig.Params),
			paramCount,
		)
	}
	returnCount := fieldListPositionCount(ft.Results)
	if !arityAcceptable(len(sig.Returns), returnCount) {
		vowReportNilSafetyf(
			pass,
			field.Pos(),
			"vow[nil-safety]: %s declares %d return positions but the signature has %d",
			nilDeclMarker,
			len(sig.Returns),
			returnCount,
		)
	}
}

// collectNilSignature walks fn's doc comments for the first
// well-formed `vow:nil` marker and returns the parsed signature.
// Multiple `vow:nil` lines on the same function are ill-formed —
// the first parsed signature wins and the function declaration
// emits a diagnostic so the author resolves the duplicate.
//
// The optional `[<scope>]` qualifier is recognised at the lexer
// level so authors can write higher-order declarations such as
// `vow:nil[cb] !` that retarget the contract at a signature-scope
// identifier. The scope is stored on NilSignature.Subject; an empty
// Subject means the marker carried no scope and the declaration
// applies to the enclosing function as usual.
//
// vow:cond * -> _, ok (bool) reflects "marker present, parse succeeded"
// vs "no marker or parse failed". Parse-failure diagnostics anchor
// at the function declaration so the author sees one error per
// malformed marker site.
func collectNilSignature(pass *analysis.Pass, fn *ast.FuncDecl) (*dsl.NilSignature, bool) {
	var (
		sig  *dsl.NilSignature
		seen bool
	)
	for _, c := range fn.Doc.List {
		line := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		chain, payload, ok := stripMarkerScopeChain(line, nilDeclMarker)
		if !ok {
			continue
		}
		if payload == "" {
			continue
		}
		parsed, err := dsl.ParseNilSignature(payload)
		if err != nil {
			vowReportNilSafetyf(pass, fn.Pos(), "vow[nil-safety]: %s: %v", nilDeclMarker, err)
			continue
		}
		if seen {
			vowReportNilSafetyf(pass, fn.Pos(), "vow[nil-safety]: function carries more than one %s marker; keeping the first", nilDeclMarker)
			continue
		}
		parsed.Subject = dsl.SubjectChain(chain)
		sig = parsed
		seen = true
	}
	if !seen {
		return nil, false
	}
	if len(sig.Subject) > 0 {
		_, failedAt, status := resolveSignatureSubjectChain(pass, fn, sig.Subject)
		if status == chainResolveFailed {
			reportChainSubjectUnresolved(pass, fn.Pos(), nilDeclMarker, sig.Subject, failedAt)
		}
		// Subject-scoped declarations bind their arity to the named
		// callback's signature, not the enclosing one, so the
		// enclosing-signature arity check would surface a false
		// mismatch. validateSubjectScopedNilDecls handles the
		// chain-step and final-not-callback diagnostics so the
		// arity gate lives in one place.
		return sig, true
	}
	checkSignatureArity(pass, fn, sig)
	checkNestArity(pass, fn, sig)
	checkNilDeclLayers(pass, fn, sig)
	return sig, true
}

// checkSignatureArity reports when the parsed vow:nil signature's
// slot count differs from the Go signature — in either direction.
// Both too-many and too-few are authoring bugs: too-many would
// otherwise drop extra positions silently, and too-few leaves the
// reader unsure which trailing slots the omitted decls intended,
// so the marker must mirror the signature arity exactly. Marker
// tokens (`!` / `?`) within each slot remain optional. The one
// exception is spelled by arityAcceptable and applies to both axes.
func checkSignatureArity(pass *analysis.Pass, fn *ast.FuncDecl, sig *dsl.NilSignature) {
	if sig == nil {
		return
	}
	paramCount := len(signatureParamNames(fn))
	if !arityAcceptable(len(sig.Params), paramCount) {
		vowReportNilSafetyf(
			pass,
			fn.Pos(),
			"vow[nil-safety]: %s declares %d parameter positions but the signature has %d",
			nilDeclMarker,
			len(sig.Params),
			paramCount,
		)
	}
	returnCount := returnPositionCount(fn)
	if !arityAcceptable(len(sig.Returns), returnCount) {
		vowReportNilSafetyf(
			pass,
			fn.Pos(),
			"vow[nil-safety]: %s declares %d return positions but the signature has %d",
			nilDeclMarker,
			len(sig.Returns),
			returnCount,
		)
	}
}

// arityAcceptable reports whether a declared slot count matches the
// Go signature's actual position count. Exact match, with one
// exception that both axes share: an empty expression covers 0 or 1
// positions, because a separator is what carves a slot out and a
// lone empty (platform) slot has no other spelling.
func arityAcceptable(declared, actual int) bool {
	if declared == 0 && actual <= 1 {
		return true
	}
	return declared == actual
}

// returnPositionCount counts the number of return positions on
// decl, expanding grouped declarations (`(a, b int)`) into one
// entry per name. A function with no return clause yields 0.
func returnPositionCount(decl *ast.FuncDecl) int {
	if decl == nil || decl.Type == nil {
		return 0
	}
	return fieldListPositionCount(decl.Type.Results)
}

// fieldListPositionCount counts the number of declared positions in
// fields, expanding grouped declarations (`a, b int`) into one
// entry per name. A nil field list (no parens content, or a params
// / results clause the declaration omits) yields 0. Shared by the
// function-level and interface-method-level arity checks so both
// surfaces count positions identically.
func fieldListPositionCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	n := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			n++
			continue
		}
		n += len(field.Names)
	}
	return n
}

// nilDeclSet returns the pass-wide map from a function or interface
// method object to its parsed `vow:nil` signature. Same-package
// callers consult the map; cross-package callers go through the
// Facts mechanism once it is not covered. Built lazily on first
// access and reused for the remainder of the pass.
//
// userNilDecls calls vowReportNilSafetyf for malformed markers,
// and vowReport's filter chain takes the same passState mutex.
// Running the build outside the locked section keeps the non-
// reentrant sync.Mutex deadlock-free; the worst case is two
// concurrent callers each computing the same set, which is
// harmless because the result is deterministic.
func (s *passState) nilDeclSet(pass *analysis.Pass) map[types.Object]*dsl.NilSignature {
	s.mu.Lock()
	built := s.nilDeclBuilt
	cached := s.nilDeclFuncs
	s.mu.Unlock()
	if built {
		return cached
	}
	out := userNilDecls(pass)
	for obj, sig := range userInterfaceMethodNilDecls(pass) {
		out[obj] = sig
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.nilDeclBuilt {
		s.nilDeclFuncs = out
		s.nilDeclBuilt = true
	}
	return s.nilDeclFuncs
}

// nilDeclParamRequirements returns the argument positions that the
// callee's `vow:nil` signature declares as non-nil (`!`). The
// receiver and the return list are not consulted here; the
// caller-side argument check fields only the parameter list.
// Positions left at platform (no decl) or nillable (`?`) do not
// contribute requirements so the caller-side check stays silent
// on them.
func nilDeclParamRequirements(sig *dsl.NilSignature, paramNames []string) []nonNilArgRequirement {
	if sig == nil || len(sig.Params) == 0 {
		return nil
	}
	out := make([]nonNilArgRequirement, 0, len(sig.Params))
	for i, decl := range sig.Params {
		if decl == nil || decl.Nullness != dsl.NullnessNonNil {
			continue
		}
		out = append(out, nonNilArgRequirement{
			argIndex:  i,
			paramName: paramNameAt(paramNames, i),
			reason:    reasonNilDecl,
		})
	}
	return out
}

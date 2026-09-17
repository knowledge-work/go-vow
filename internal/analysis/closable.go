package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// closableTypeSet returns the set of qualified type-name strings the
// analyzer treats as Closable resources. The set merges two sources:
// user-defined type declarations carrying a `vow:define @Closable`
// marker and the built-in closable types. A nearest-ancestor
// `vow.yaml` whose `closable.built_in_closables` is set replaces the
// embedded preset list for this package's scope (an empty list means
// "no built-ins"); an absent key leaves the embedded list in place.
// The result is built lazily on first access and reused for the
// remainder of the pass.
func (s *passState) closableTypeSet(pass *analysis.Pass) map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closableBuilt {
		override := s.packageBuiltinClosableOverride(pass)
		s.closableTypeNames = discoverClosableTypeNames(pass, s.presets, override)
		s.closableBuilt = true
	}
	return s.closableTypeNames
}

// packageBuiltinClosableOverride consults the driver-supplied
// override first and falls back to the file-local `vow.yaml` for
// a closable.built_in_closables override. The return value is the
// override list when set (including the empty slice, which means
// "no built-ins"), or nil when no override applies — a missing
// config, an absent key, or any discovery error all fall through
// to the embedded preset default.
func (s *passState) packageBuiltinClosableOverride(pass *analysis.Pass) *[]string {
	if s.configOverride != nil {
		return s.configOverride.Closable.BuiltinClosables
	}
	if s.configResolver == nil {
		return nil
	}
	dir := passPackageDirectory(pass)
	if dir == "" {
		return nil
	}
	cfg, err := s.configResolver.Resolve(dir)
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.Closable.BuiltinClosables
}

// passPackageDirectory returns the absolute directory that holds
// the package's first source file, or "" when the pass carries no
// resolvable position (synthetic test passes, empty file lists).
func passPackageDirectory(pass *analysis.Pass) string {
	if pass == nil || pass.Fset == nil || len(pass.Files) == 0 {
		return ""
	}
	filename := pass.Fset.Position(pass.Files[0].Pos()).Filename
	if filename == "" {
		return ""
	}
	abs, err := filepath.Abs(filename)
	if err != nil {
		return ""
	}
	return filepath.Dir(abs)
}

// discoverClosableTypeNames walks the package's type declarations
// for `vow:define @Closable` annotations and folds the built-in
// closable types into the same set. The qualified-name
// representation is the type expression as it appears at use sites
// (e.g. `*os.File`, `io.Closer`); the analyzer side matches this
// against types.Type strings when classifying a value as Closable.
//
// builtinOverride controls the built-in list: a nil pointer uses
// every preset's BuiltinClosables, while a non-nil pointer replaces
// the preset list with the override's entries (an empty list means
// "no built-ins").
func discoverClosableTypeNames(pass *analysis.Pass, presets []*dsl.Preset, builtinOverride *[]string) map[string]struct{} {
	result := map[string]struct{}{}
	// User-declared Closable types: a type spec whose doc comment
	// carries `vow:define @Closable` registers the type's package-
	// qualified name.
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if ok && insp != nil {
		filter := []ast.Node{(*ast.GenDecl)(nil)}
		insp.Preorder(filter, func(n ast.Node) {
			gd := n.(*ast.GenDecl)
			if gd.Tok != token.TYPE {
				return
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name == nil {
					continue
				}
				if !typeSpecDeclaresClosable(gd, ts) {
					continue
				}
				qualified := qualifiedTypeName(pass, ts.Name.Name)
				result[qualified] = struct{}{}
			}
		})
	}
	addBuiltin := func(name string) {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return
		}
		result[trimmed] = struct{}{}
	}
	if builtinOverride != nil {
		for _, name := range *builtinOverride {
			addBuiltin(name)
		}
		return result
	}
	for _, preset := range presets {
		for _, name := range preset.BuiltinClosables {
			addBuiltin(name)
		}
	}
	return result
}

// typeSpecDeclaresClosable reports whether the type spec or its
// enclosing GenDecl group doc carries a `vow:define @Closable`
// marker. The group-doc fall-through mirrors the concept-define
// diagnostic walker so a `type ( ... )` block whose group comment
// declares the concept registers every spec inside.
func typeSpecDeclaresClosable(gd *ast.GenDecl, ts *ast.TypeSpec) bool {
	if docDeclaresConcept(ts.Doc, builtinConceptClosable) {
		return true
	}
	return docDeclaresConcept(gd.Doc, builtinConceptClosable)
}

// docDeclaresConcept reports whether doc carries a `vow:define
// @<concept>` line that names the requested concept. The check is
// line-by-line so a doc that mentions the concept inside prose (not
// as a marker line) does not register the declaration.
func docDeclaresConcept(doc *ast.CommentGroup, concept string) bool {
	if doc == nil {
		return false
	}
	for _, line := range strings.Split(doc.Text(), "\n") {
		name, ok := parseConceptDefineLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		if name == concept {
			return true
		}
	}
	return false
}

// qualifiedTypeName returns the package-qualified name of a type
// declared in the current pass. Local type declarations land as
// `<pkg-path>.<name>`; type aliases share the same scheme. Callers
// match this string against the type-expression form recorded in the
// Closable set so cross-package references resolve identically.
func qualifiedTypeName(pass *analysis.Pass, localName string) string {
	if pass.Pkg == nil {
		return localName
	}
	return pass.Pkg.Path() + "." + localName
}

// validateClosableLifetime walks every FuncDecl in pass.Files and
// reports a diagnostic for each Closable value the body acquires
// without a same-function discharge. Discharges are a defer Close
// call on the acquired identifier and a statement-scope `vow:emit`
// marker that hands the value off to a recognised destination
// (goroutine launch, callback argument, storage write, or channel
// send). Functions whose body is nil (interface declarations,
// externally-implemented functions) are skipped — the resource
// contract has no body to inspect.
func validateClosableLifetime(pass *analysis.Pass, state *passState) {
	closables := state.closableTypeSet(pass)
	if len(closables) == 0 {
		return
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			checkClosableAcquisitions(pass, fn, closables)
			checkUpcastInvariant(pass, fn, closables)
		}
	}
}

// closableAcquisition records a function-local Closable acquisition
// site: the identifier the value is bound to and the assignment's
// position. The pair drives both the discharge match (an identifier
// lookup against deferred Close receivers) and the diagnostic
// anchor when no discharge is found.
type closableAcquisition struct {
	name string
	pos  token.Pos
}

// checkClosableAcquisitions inspects fn.Body for Closable acquisitions
// and discharge sites, then emits a diagnostic at every acquisition
// whose identifier never reaches a defer Close call in the same
// function. The walk is deliberately shallow: it credits a discharge
// when a deferred call's receiver name matches the acquisition's
// identifier, leaving deeper alias chasing and value-flow narrowing
// to a separate SSA pass.
func checkClosableAcquisitions(pass *analysis.Pass, fn *ast.FuncDecl, closables map[string]struct{}) {
	acquisitions := collectClosableAcquisitions(pass, fn, closables)
	if len(acquisitions) == 0 {
		return
	}
	deferred := collectDeferredCloseReceivers(fn)
	emitted := emitDischargesForFunction(pass, fn)
	for _, acq := range acquisitions {
		if _, ok := deferred[acq.name]; ok {
			continue
		}
		if _, ok := emitted[acq.name]; ok {
			continue
		}
		vowReportf(
			pass,
			acq.pos,
			"vow[closable]: %s is acquired but not closed; add a defer Close call or declare the transfer with vow:emit",
			acq.name,
		)
	}
}

// collectClosableAcquisitions returns every short-variable
// declaration in fn.Body whose right-hand side resolves to a
// Closable type. The recognition uses pass.TypesInfo so the type
// match honours aliasing and selector qualification rather than
// matching source-text substrings. Multiple-assignment shapes
// (`f, err := os.Open(...)`) credit the position of each Closable-
// typed LHS independently.
func collectClosableAcquisitions(pass *analysis.Pass, fn *ast.FuncDecl, closables map[string]struct{}) []closableAcquisition {
	var out []closableAcquisition
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE {
			return true
		}
		paired := len(assign.Lhs) == len(assign.Rhs)
		for i, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok || ident.Name == "_" {
				continue
			}
			// `f := os.Stdout` binds a handle the process owns, not an
			// acquisition. The pairing only holds when the shapes match:
			// `f, err := os.Open(p)` has one right-hand expression for
			// two names, and its handle is acquired.
			if paired && isStdHandleExpr(pass, assign.Rhs[i]) {
				continue
			}
			obj := pass.TypesInfo.Defs[ident]
			if obj == nil {
				continue
			}
			if !isClosableType(obj.Type(), closables) {
				continue
			}
			out = append(out, closableAcquisition{name: ident.Name, pos: ident.Pos()})
		}
		return true
	})
	return out
}

// collectDeferredCloseReceivers walks fn.Body for `defer X.Close()`
// statements and returns the set of receiver identifier names. The
// match is intentionally narrow: only a selector-style call against
// a bare identifier counts. Aliasing chains, method values, and
// deferred wrappers are not covered by this walk.
func collectDeferredCloseReceivers(fn *ast.FuncDecl) map[string]struct{} {
	out := map[string]struct{}{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		def, ok := n.(*ast.DeferStmt)
		if !ok {
			return true
		}
		call, ok := def.Call.Fun.(*ast.SelectorExpr)
		if !ok || call.Sel == nil || call.Sel.Name != "Close" {
			return true
		}
		recv, ok := call.X.(*ast.Ident)
		if !ok {
			return true
		}
		out[recv.Name] = struct{}{}
		return true
	})
	return out
}

// isClosableType reports whether t is registered as a Closable type.
// The match honours four axes: the fully-qualified type string as
// it appears at use sites (e.g. `*os.File`), the underlying named
// type after stripping a single pointer indirection, the underlying
// type (alias / embedded chain resolution), and — for generic type
// parameters — the parameter's interface constraint when the
// constraint embeds a registered Closable interface.
func isClosableType(t types.Type, closables map[string]struct{}) bool {
	if t == nil {
		return false
	}
	if _, ok := closables[t.String()]; ok {
		return true
	}
	if ptr, ok := t.(*types.Pointer); ok && ptr.Elem() != nil {
		if _, ok := closables[ptr.Elem().String()]; ok {
			return true
		}
	}
	if under := t.Underlying(); under != nil && under != t {
		if _, ok := closables[under.String()]; ok {
			return true
		}
	}
	if typeParam, ok := t.(*types.TypeParam); ok {
		return typeParamConstraintIsClosable(typeParam, closables)
	}
	return false
}

// typeParamConstraintIsClosable reports whether the type parameter's
// constraint resolves to a Closable interface. The walker first
// matches the constraint's qualified-name string against the
// Closable set so a bare `io.Closer` constraint hits the built-in
// preset entry directly; on a miss it unwraps to the constraint's
// interface and credits the parameter when any one of the embedded
// types is registered as Closable. Nested embeddings beyond the
// first level and type-set unions (`A | B`) fall through unsupported
// — the walker does not widen if a real constraint shape
// demands it.
func typeParamConstraintIsClosable(typeParam *types.TypeParam, closables map[string]struct{}) bool {
	constraint := typeParam.Constraint()
	if constraint == nil {
		return false
	}
	if _, ok := closables[constraint.String()]; ok {
		return true
	}
	iface, ok := constraint.Underlying().(*types.Interface)
	if !ok {
		return false
	}
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		embed := iface.EmbeddedType(i)
		if embed == nil {
			continue
		}
		if _, ok := closables[embed.String()]; ok {
			return true
		}
	}
	return false
}

// checkUpcastInvariant walks fn.Body for sites where a Closable
// value is upcast to a non-Closable interface and reports a
// diagnostic at each occurrence. Three site kinds are recognised:
// return statements whose result is a Closable value but whose
// function signature declares a non-Closable interface return; call
// arguments where a Closable value is passed to a parameter typed
// as a non-Closable interface; and short-variable declarations
// where the LHS is typed as a non-Closable interface but the RHS
// resolves to a Closable value. Reads of the os package's
// process-owned handles are exempt at all three. The recogniser
// stays aggressive per the project's detection policy: any
// remaining ambiguity reports. These diagnostics carry no category,
// so the vow:suppress filter cannot drop them — a site that must
// stand is changed in the code, or its type is taken out of
// closable.built_in_closables. Where that list comes from a
// --config-file / --config-yaml override the change is run-wide: an
// override suppresses the per-directory vow.yaml walk.
func checkUpcastInvariant(pass *analysis.Pass, fn *ast.FuncDecl, closables map[string]struct{}) {
	if fn.Type == nil {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.ReturnStmt:
			checkUpcastReturn(pass, fn, v, closables)
		case *ast.CallExpr:
			checkUpcastCallArgs(pass, v, closables)
		case *ast.AssignStmt:
			checkUpcastAssign(pass, v, closables)
		}
		return true
	})
}

// checkUpcastReturn reports an upcast at every return whose result
// expression is a Closable value but whose enclosing function
// signature declares a non-Closable interface for the matching
// return position.
func checkUpcastReturn(pass *analysis.Pass, fn *ast.FuncDecl, ret *ast.ReturnStmt, closables map[string]struct{}) {
	results := fn.Type.Results
	if results == nil {
		return
	}
	declared := flattenFieldList(results)
	for i, expr := range ret.Results {
		if i >= len(declared) {
			return
		}
		declType := pass.TypesInfo.TypeOf(declared[i])
		exprType := pass.TypesInfo.TypeOf(expr)
		if upcastDropsObligation(pass, declType, exprType, expr, closables) {
			vowReportf(pass, expr.Pos(), "vow[closable]: returning a Closable value as %s drops the close obligation; declare the return type as a Closable interface or hand off explicitly", typeDisplay(declType))
		}
	}
}

// checkUpcastCallArgs reports an upcast at every call argument
// whose value is Closable but whose declared parameter type is a
// non-Closable interface. Variadic parameters share the parameter
// type across the trailing arguments, so each variadic position is
// checked against the same declared type.
func checkUpcastCallArgs(pass *analysis.Pass, call *ast.CallExpr, closables map[string]struct{}) {
	calleeType := pass.TypesInfo.TypeOf(call.Fun)
	sig, ok := unwrapSignature(calleeType)
	if !ok {
		return
	}
	for i, arg := range call.Args {
		paramType := signatureParamType(sig, i)
		if paramType == nil {
			continue
		}
		argType := pass.TypesInfo.TypeOf(arg)
		if upcastDropsObligation(pass, paramType, argType, arg, closables) {
			vowReportf(pass, arg.Pos(), "vow[closable]: passing a Closable value as %s drops the close obligation; the parameter type must also be a Closable interface", typeDisplay(paramType))
		}
	}
}

// checkUpcastAssign reports an upcast at every short-variable
// declaration where the RHS is a Closable value but the LHS is
// declared with a non-Closable interface type. Multi-assign shapes
// pair each LHS / RHS by position, mirroring how the language
// itself assigns the values.
func checkUpcastAssign(pass *analysis.Pass, assign *ast.AssignStmt, closables map[string]struct{}) {
	if assign.Tok != token.DEFINE && assign.Tok != token.ASSIGN {
		return
	}
	if len(assign.Lhs) != len(assign.Rhs) {
		return
	}
	for i, lhs := range assign.Lhs {
		lhsType := pass.TypesInfo.TypeOf(lhs)
		rhsType := pass.TypesInfo.TypeOf(assign.Rhs[i])
		if upcastDropsObligation(pass, lhsType, rhsType, assign.Rhs[i], closables) {
			vowReportf(pass, assign.Rhs[i].Pos(), "vow[closable]: assigning a Closable value to a %s drops the close obligation; the destination type must also be a Closable interface", typeDisplay(lhsType))
		}
	}
}

// upcastDropsObligation reports whether srcExpr flowing into dst
// drops a close obligation. A read of a process-owned handle carries
// no obligation, so it never does; every other flow falls through to
// the type-level test.
func upcastDropsObligation(pass *analysis.Pass, dst, src types.Type, srcExpr ast.Expr, closables map[string]struct{}) bool {
	if isStdHandleExpr(pass, srcExpr) {
		return false
	}
	return isNonClosableInterfaceUpcast(dst, src, closables)
}

// isNonClosableInterfaceUpcast reports whether dst is a non-Closable
// interface and src is a Closable concrete type. The check stays
// narrow: only assignments that change the static type from a
// Closable form to a non-Closable interface qualify. Same-type
// assignments and value-to-value flows fall through.
func isNonClosableInterfaceUpcast(dst, src types.Type, closables map[string]struct{}) bool {
	if dst == nil || src == nil {
		return false
	}
	if !isClosableType(src, closables) {
		return false
	}
	dstUnder := dst.Underlying()
	if _, isInterface := dstUnder.(*types.Interface); !isInterface {
		return false
	}
	if isClosableType(dst, closables) {
		return false
	}
	return true
}

// stdHandleNames are the process-lifetime handles the os package
// exposes as package variables. A read of one of them carries no
// close obligation: the process owns the descriptor, and closing it
// breaks every later write through it.
var stdHandleNames = map[string]struct{}{
	"Stdout": {},
	"Stderr": {},
	"Stdin":  {},
}

// isStdHandleExpr reports whether expr reads one of the os package
// handles named in stdHandleNames. The match resolves the selector
// to its object, so a local variable shadowing os, a same-named
// variable in another package, and a field named Stdout all fall
// through.
func isStdHandleExpr(pass *analysis.Pass, expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	obj, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Var)
	if !ok || obj.Pkg() == nil || obj.Pkg().Path() != "os" {
		return false
	}
	if obj.Parent() != obj.Pkg().Scope() {
		return false
	}
	_, ok = stdHandleNames[obj.Name()]
	return ok
}

// flattenFieldList expands an *ast.FieldList into a per-position
// slice of type expressions. A grouped declaration like
// `(a, b SomeType)` contributes the same type expression twice so
// caller-side indexing matches the language-level position.
func flattenFieldList(fl *ast.FieldList) []ast.Expr {
	if fl == nil {
		return nil
	}
	var out []ast.Expr
	for _, field := range fl.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			out = append(out, field.Type)
		}
	}
	return out
}

// unwrapSignature returns the function signature represented by t,
// unwrapping a *types.Named alias when present. Non-function types
// (built-in conversions, method values whose signature the analyzer
// cannot resolve) return ok=false so the caller skips the check.
func unwrapSignature(t types.Type) (*types.Signature, bool) {
	if t == nil {
		return nil, false
	}
	sig, ok := t.Underlying().(*types.Signature)
	if !ok {
		return nil, false
	}
	return sig, true
}

// signatureParamType returns the declared parameter type at
// position i, expanding the variadic tail across positions past the
// final fixed parameter. Out-of-range positions return nil so the
// caller can skip the argument.
func signatureParamType(sig *types.Signature, i int) types.Type {
	params := sig.Params()
	if params == nil {
		return nil
	}
	n := params.Len()
	if n == 0 {
		return nil
	}
	if i < n {
		return params.At(i).Type()
	}
	if sig.Variadic() {
		return params.At(n - 1).Type()
	}
	return nil
}

// typeDisplay returns a short, human-readable rendering of t for
// diagnostic messages. nil types render as `unknown` so the message
// stays sensible when the analyzer cannot resolve the destination.
func typeDisplay(t types.Type) string {
	if t == nil {
		return "unknown"
	}
	return t.String()
}

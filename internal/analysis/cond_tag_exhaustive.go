package analysis

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// checkTagExhaustiveness inspects every function whose `vow:cond`
// payload binds a discriminator reference to a typed-string
// constant family and reports the constant values the rule set
// fails to cover. A discriminator is any reference operand whose
// resolved Go type is a named string with two or more
// package-level typed constants of that same type. The walk only
// fires when the rule pool reaches both the equality-comparison
// shape and the typed-constant resolution so authors who key
// their rules on ad-hoc string literals avoid noisy diagnostics
// at a slot the analyzer cannot recognise as a finite enum.
//
// The walk groups rules by their discriminator reference rendered
// through the existing Reference.String helper so two rules
// keyed on the same field of the same receiver share a single
// missing-case report. Each group's covered set is the union of
// the rule literals after Unquote; the missing set is the
// finite enum's value set minus the covered set. A missing set
// of one or more values surfaces a single diagnostic at the
// function declaration with the missing values rendered in
// source order.
func checkTagExhaustiveness(pass *analysis.Pass, state *passState) {
	for obj, rules := range state.logicalRulesSet(pass) {
		if obj == nil || len(rules) < 2 {
			continue
		}
		typesSig, ok := signatureFromObj(obj)
		if !ok {
			continue
		}
		ctx := buildSignatureNameContext(typesSig)
		reportTagExhaustiveness(pass, obj, typesSig, rules, ctx)
	}
}

// tagEntryKind discriminates the derivation path a tagEntry
// took: an author-written original or a transitive chain
// closure derived from one. The exhaustiveness reporter reads
// the kind to render the diagnostic prefix and to keep the
// missing-case set keyed on author intent.
type tagEntryKind int

const (
	tagEntryOriginal tagEntryKind = iota
	tagEntryTransitive
)

// tagEntry pairs an arrow rule whose left-hand side compares a
// reference to a string literal or to an identifier referencing
// a string constant with the extracted discriminator reference
// and the right-hand side spelling. The Kind field records
// whether the entry came from an author-written rule or from a
// transitive chain closure so the reporter can attribute
// missing-case diagnostics to the right derivation; the
// LiteralSpelling field holds the right-hand side's surface
// form so the pass-bound resolver decodes a quoted literal
// through strconv.Unquote and an identifier through the package
// scope's constant lookup at report time.
type tagEntry struct {
	Rule            *dsl.ArrowRule
	Discriminator   dsl.Reference
	LiteralSpelling string
	Kind            tagEntryKind
}

// collectTagEntries returns the rule pool the exhaustiveness
// walk inspects. Only forward implications and biconditionals
// whose left-hand side is an equality comparison against a
// string literal contribute; other shapes leave no
// discriminator signal. Derived contrapositives stay out of the
// pool because their left-hand side carries an inequality (`!=`)
// against a literal and therefore conveys an exclusion rather
// than a positive case binding. Transitive chain closures stay
// in the pool because the chain expansion can surface a fresh
// positive binding the author did not write directly.
func collectTagEntries(rules []*dsl.ArrowRule) []tagEntry {
	var entries []tagEntry
	for _, group := range groupRulesWithContrapositives(rules) {
		if entry, ok := tagEntryFromRule(group.Original, tagEntryOriginal); ok {
			entries = append(entries, entry)
		}
		for _, t := range group.Transitives {
			if entry, ok := tagEntryFromRule(t.Rule, tagEntryTransitive); ok {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

// tagEntryFromRule extracts the discriminator reference and the
// right-hand side spelling from rule's left-hand side. The rule
// must be a logical-arrow rule (`=>` or `<=>`) whose left
// operand is an equality comparison (`==`) whose right-hand side
// is either a quoted string literal or an identifier; other
// shapes report ok=false so the caller drops the rule from the
// exhaustiveness pool. The literal-versus-identifier
// disambiguation runs at report time so the pass-bound resolver
// can route an identifier through the package scope's constant
// lookup with access to the analysis pass.
func tagEntryFromRule(rule *dsl.ArrowRule, kind tagEntryKind) (tagEntry, bool) {
	if rule == nil || rule.Left == nil {
		return tagEntry{}, false
	}
	if rule.Direction != dsl.DirImply && rule.Direction != dsl.DirEquiv {
		return tagEntry{}, false
	}
	left := rule.Left
	if left.Kind != dsl.ExprComparison || left.Op != dsl.OpEq {
		return tagEntry{}, false
	}
	// Tag exhaustiveness reads one literal spelling per rule so a
	// multi-member sum on the discriminator side does not participate
	// in the finite-enum walk; the per-site evaluator stays the
	// source of commitments for sum-RHS rules.
	if len(left.Members) != 1 || left.Members[0].Nil {
		return tagEntry{}, false
	}
	spelling := left.Members[0].Literal
	if !literalLooksLikeStringOrIdent(spelling) {
		return tagEntry{}, false
	}
	return tagEntry{
		Rule:            rule,
		Discriminator:   left.Ref,
		LiteralSpelling: spelling,
		Kind:            kind,
	}, true
}

// literalLooksLikeStringOrIdent reports whether spelling reads as
// a quoted string literal or as a bare Go identifier. The
// quoted shape begins with `"`, "`", or `'`; an identifier
// begins with a letter or underscore and contains only the
// identifier-class run thereafter. Anything else (numeric
// literals, bool keywords) routes to ok=false so the
// exhaustiveness layer leaves non-string operands silent.
func literalLooksLikeStringOrIdent(spelling string) bool {
	if len(spelling) == 0 {
		return false
	}
	first := spelling[0]
	if first == '"' || first == '`' || first == '\'' {
		return true
	}
	return isIdentStart(first) && allIdentRunes(spelling[1:])
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentCont(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

func allIdentRunes(rest string) bool {
	for i := 0; i < len(rest); i++ {
		if !isIdentCont(rest[i]) {
			return false
		}
	}
	return true
}

// reportTagExhaustiveness groups the rule pool by discriminator
// reference and reports each group whose covered literal set
// fails to span the discriminator's finite enum candidates. A
// group needs at least one author-written original so a pool
// of pure chain closures stays silent; the missing-case report
// otherwise would attribute author intent to a derivation the
// author did not write. Each entry's right-hand side spelling
// resolves through resolveLiteralStringValue so a quoted
// literal routes through Unquote and an identifier routes
// through the package scope's constant lookup; the resolved
// constant object, when present, also seeds the untyped sibling
// candidate path so a rule pool keyed on identifiers from a
// shared const block discovers its missing-case set even when
// the discriminator's type is not a named typed string.
func reportTagExhaustiveness(pass *analysis.Pass, obj types.Object, sig *types.Signature, rules []*dsl.ArrowRule, ctx signatureNameContext) {
	entries := collectTagEntries(rules)
	if len(entries) == 0 {
		return
	}
	groups, order := groupTagEntries(entries)
	for _, key := range order {
		group := groups[key]
		if len(group) < 2 {
			continue
		}
		if !groupHasOriginal(group) {
			continue
		}
		ref := group[0].Discriminator
		covered, sibSeed := coveredTagValues(pass, group)
		if len(covered) == 0 {
			continue
		}
		candidates, ok := finiteEnumCandidates(pass, sig, ref, ctx, sibSeed)
		if !ok {
			continue
		}
		missing := missingTagValues(candidates, covered)
		if len(missing) == 0 {
			continue
		}
		vowReportf(
			pass,
			obj.Pos(),
			"vow[sentinel-error]: vow:cond rules on %s omit %s cases: %s",
			obj.Name(),
			ref.String(),
			strings.Join(missing, ", "),
		)
	}
}

// resolveLiteralStringValue decodes a tagEntry right-hand side
// spelling into the string value the discriminator binds
// against. A quoted spelling routes through strconv.Unquote so
// the surface round-trips through the same decoder the parser
// uses on string tokens. An identifier spelling routes through
// the analysis pass's package scope lookup: when the name
// binds to a constant whose value is a string, the helper
// returns the decoded value together with the constant object
// so the caller can enumerate the constant's owning
// `const` block siblings. Spellings that neither route resolves
// (a numeric literal masquerading as an identifier, an
// identifier the package scope does not expose, a constant
// whose value is not a string) report ok=false so the caller
// drops the entry from the covered set rather than guessing.
func resolveLiteralStringValue(pass *analysis.Pass, spelling string) (string, *types.Const, bool) {
	if len(spelling) == 0 {
		return "", nil, false
	}
	first := spelling[0]
	if first == '"' || first == '`' || first == '\'' {
		v, err := strconv.Unquote(spelling)
		if err != nil {
			return "", nil, false
		}
		return v, nil, true
	}
	if pass == nil || pass.Pkg == nil {
		return "", nil, false
	}
	obj, ok := pass.Pkg.Scope().Lookup(spelling).(*types.Const)
	if !ok {
		return "", nil, false
	}
	val := obj.Val()
	if val.Kind() != constant.String {
		return "", nil, false
	}
	return constant.StringVal(val), obj, true
}

// groupTagEntries partitions entries by the rendered
// discriminator reference. The order slice records the first
// appearance of each key so the reporter emits diagnostics in
// source order regardless of map iteration order.
func groupTagEntries(entries []tagEntry) (map[string][]tagEntry, []string) {
	groups := map[string][]tagEntry{}
	var order []string
	for _, entry := range entries {
		key := entry.Discriminator.String()
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], entry)
	}
	return groups, order
}

// groupHasOriginal reports whether group carries at least one
// author-written rule. Groups whose entries all derive from
// transitive chain closures stay silent because the missing-
// case set would otherwise attribute author intent to a
// derivation the author did not write directly.
func groupHasOriginal(group []tagEntry) bool {
	return slices.ContainsFunc(group, func(entry tagEntry) bool {
		return entry.Kind == tagEntryOriginal
	})
}

// coveredTagValues collects the literal values group's rules
// already pin. The set is keyed on the resolved string so two
// rules that name the same enum value through different
// spellings (a quoted literal and a constant identifier that
// resolves to the same value) still collapse to a single
// covered slot. The walker also returns the first constant
// object an identifier spelling resolved through; the caller
// uses the constant as the untyped sibling enumeration seed
// when the discriminator's type does not expose a typed enum.
func coveredTagValues(pass *analysis.Pass, group []tagEntry) (map[string]bool, *types.Const) {
	covered := map[string]bool{}
	var sibSeed *types.Const
	for _, entry := range group {
		value, sibObj, ok := resolveLiteralStringValue(pass, entry.LiteralSpelling)
		if !ok {
			continue
		}
		covered[value] = true
		if sibSeed == nil && sibObj != nil {
			sibSeed = sibObj
		}
	}
	return covered, sibSeed
}

// missingTagValues returns the finite enum candidates the
// covered set fails to span, rendered in the candidate order
// the package scope first revealed so the diagnostic mirrors
// the declaration order rather than a hash-driven shuffle.
func missingTagValues(candidates []string, covered map[string]bool) []string {
	var missing []string
	for _, candidate := range candidates {
		if !covered[candidate] {
			missing = append(missing, candidate)
		}
	}
	return missing
}

// finiteEnumCandidates resolves the candidate value set the
// missing-case walk diffs against. The typed-enum path fires
// first: when the discriminator's resolved Go type is a named
// string whose package scope holds two or more typed constants
// of that same type, the helper returns those constants' string
// values. When the typed path stays silent (the type is plain
// `string`, the typed-constant pool is too small), the helper
// falls back to the untyped-sibling path: when the rule pool
// surfaced an identifier-keyed constant, the helper enumerates
// the sibling values declared in the same `const` block. The
// helper reports ok=false when neither path commits a
// candidate set with two or more values so the exhaustiveness
// reporter leaves the group silent rather than firing on a
// discriminator the analyzer cannot recognise as a finite
// enum.
func finiteEnumCandidates(pass *analysis.Pass, sig *types.Signature, ref dsl.Reference, ctx signatureNameContext, sibSeed *types.Const) ([]string, bool) {
	if values, ok := typedEnumCandidates(sig, ref, ctx); ok {
		return values, true
	}
	if sibSeed == nil {
		return nil, false
	}
	values := constSiblingStringValues(pass, sibSeed)
	if len(values) < 2 {
		return nil, false
	}
	return values, true
}

// typedEnumCandidates returns the typed-enum candidate values
// the discriminator's resolved Go type exposes through its
// package scope. The path commits when the resolved type is a
// named string and the scope holds two or more typed
// constants of that same type; other shapes report ok=false so
// the caller falls back to the untyped-sibling path.
func typedEnumCandidates(sig *types.Signature, ref dsl.Reference, ctx signatureNameContext) ([]string, bool) {
	headType, ok := resolveReferenceHeadType(sig, ref, ctx)
	if !ok {
		return nil, false
	}
	resolved, ok := walkReferencePath(headType, ref.Path)
	if !ok {
		return nil, false
	}
	named, ok := resolved.(*types.Named)
	if !ok {
		return nil, false
	}
	basic, ok := named.Underlying().(*types.Basic)
	if !ok || basic.Kind() != types.String {
		return nil, false
	}
	values := typedStringConstantValues(named)
	if len(values) < 2 {
		return nil, false
	}
	return values, true
}

// resolveReferenceHeadType returns the Go type the reference
// head names within sig. A bare identifier head resolves
// through the parameter list, the receiver, or the return
// list; `$N` resolves through the positional return slot. The
// helper reports ok=false when the head does not bind to any
// signature position so the caller skips the reference rather
// than reading an unrelated identifier from an enclosing scope.
func resolveReferenceHeadType(sig *types.Signature, ref dsl.Reference, ctx signatureNameContext) (types.Type, bool) {
	switch ref.Kind {
	case dsl.RefIdent:
		if sig.Recv() != nil && ctx.RecvName != "" && ref.Name == ctx.RecvName {
			return sig.Recv().Type(), true
		}
		params := sig.Params()
		for i := 0; i < params.Len(); i++ {
			if params.At(i).Name() == ref.Name {
				return params.At(i).Type(), true
			}
		}
		results := sig.Results()
		for i := 0; i < results.Len(); i++ {
			if results.At(i).Name() == ref.Name {
				return results.At(i).Type(), true
			}
		}
	case dsl.RefDollar:
		index, err := strconv.Atoi(ref.Name)
		if err != nil || index < 1 {
			return nil, false
		}
		results := sig.Results()
		if index > results.Len() {
			return nil, false
		}
		return results.At(index - 1).Type(), true
	}
	return nil, false
}

// walkReferencePath threads typ through ref.Path one postfix
// step at a time. StepField unwraps pointers, looks the field
// up on the underlying struct, and continues with the field's
// type; the index-keyed steps unwrap pointers, arrays, slices,
// or maps to the element type. A step that does not match the
// current type's shape reports ok=false so the caller leaves
// the reference unresolved rather than guessing.
func walkReferencePath(typ types.Type, path []dsl.PathStep) (types.Type, bool) {
	for _, step := range path {
		switch step.Kind {
		case dsl.StepField:
			field, ok := lookupStructField(typ, step.Name)
			if !ok {
				return nil, false
			}
			typ = field.Type()
		case dsl.StepIndex, dsl.StepIdentKey, dsl.StepStringKey:
			elem, ok := indexableElementType(typ)
			if !ok {
				return nil, false
			}
			typ = elem
		default:
			return nil, false
		}
	}
	return typ, true
}

// lookupStructField unwraps pointers to find the underlying
// struct and returns the field whose name matches. The helper
// reports ok=false when the type does not wrap a struct or
// when the struct holds no field by that name so the path
// walker stops rather than skipping a missing field.
func lookupStructField(typ types.Type, name string) (*types.Var, bool) {
	for {
		ptr, ok := typ.(*types.Pointer)
		if !ok {
			break
		}
		typ = ptr.Elem()
	}
	named, ok := typ.(*types.Named)
	if ok {
		typ = named.Underlying()
	}
	strct, ok := typ.(*types.Struct)
	if !ok {
		return nil, false
	}
	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if field.Name() == name {
			return field, true
		}
	}
	return nil, false
}

// indexableElementType returns the element type of an array,
// slice, or map. Pointers are followed through one level so a
// `*[]T` reads the same as `[]T`. Other shapes report
// ok=false because the postfix grammar's index step only
// targets element accesses.
func indexableElementType(typ types.Type) (types.Type, bool) {
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	under := typ
	if named, ok := typ.(*types.Named); ok {
		under = named.Underlying()
	}
	switch t := under.(type) {
	case *types.Array:
		return t.Elem(), true
	case *types.Slice:
		return t.Elem(), true
	case *types.Map:
		return t.Elem(), true
	}
	return nil, false
}

// typedStringConstantValues enumerates the package-level typed
// constants whose declared type matches named and returns their
// string values in package scope declaration order. The walker
// inspects every name in the named type's package scope, picks
// the constant objects, and accepts only those whose declared
// type is identical to named so untyped constants whose value
// happens to match a string slot stay out of the candidate set.
func typedStringConstantValues(named *types.Named) []string {
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return nil
	}
	scope := pkg.Scope()
	var values []string
	for _, name := range scope.Names() {
		con, ok := scope.Lookup(name).(*types.Const)
		if !ok {
			continue
		}
		if !types.Identical(con.Type(), named) {
			continue
		}
		val := con.Val()
		if val.Kind() != constant.String {
			continue
		}
		values = append(values, constant.StringVal(val))
	}
	sort.Strings(values)
	return values
}

// constSiblingStringValues returns the string values of every
// constant declared in the same `const` block as obj. The
// walker scans every file's top-level declarations for the
// const GenDecl whose Pos / End span contains obj.Pos(), then
// reads every ValueSpec's names back through the analysis
// pass's type information to collect each sibling's constant
// value. Constants whose value is not a string stay out of
// the candidate set so a mixed-kind block does not leak
// non-string values into the missing-case diagnostic. A
// singleton block (obj's GenDecl carries exactly one
// ValueSpec with one name) returns the single value; the
// caller's threshold drops candidate sets shorter than two so
// a one-value block surfaces no exhaustiveness diagnostic.
func constSiblingStringValues(pass *analysis.Pass, obj *types.Const) []string {
	if pass == nil || obj == nil {
		return nil
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			if obj.Pos() < gen.Pos() || obj.Pos() > gen.End() {
				continue
			}
			values := constBlockStringValues(pass, gen)
			sort.Strings(values)
			return values
		}
	}
	return nil
}

// constBlockStringValues walks every ValueSpec inside gen and
// collects the string values of the constant objects each name
// resolves to. Names that do not resolve to a *types.Const, or
// whose value is not a string, stay out of the returned slice
// so the caller's covered-set diff stays type-coherent.
func constBlockStringValues(pass *analysis.Pass, gen *ast.GenDecl) []string {
	var values []string
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range vs.Names {
			sibObj, ok := pass.TypesInfo.Defs[name].(*types.Const)
			if !ok {
				continue
			}
			val := sibObj.Val()
			if val.Kind() != constant.String {
				continue
			}
			values = append(values, constant.StringVal(val))
		}
	}
	return values
}

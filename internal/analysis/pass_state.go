package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"sync"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/config"
	"github.com/knowledge-work/go-vow/internal/dsl"
)

// passState carries the per-pass caches shared across the various
// helpers that traverse a Go package's AST. Created once at the top
// of each analyzer run and discarded when the run completes — so
// every key in the maps is guaranteed to belong to the same package
// snapshot.
type passState struct {
	mu                   sync.Mutex
	anno                 map[*ast.FuncDecl]annoEntry
	scope                dsl.RuleScope
	scopeBuilt           bool
	parentMaps           map[*ast.File]map[ast.Node]ast.Node
	transducers          map[types.Object]transducerSpec
	transducersBuilt     bool
	dischargedFuncs      map[types.Object]bool
	dischargedBuilt      bool
	useFuncs             map[types.Object][]string
	useBuilt             bool
	emitFuncs            map[types.Object][]string
	emitBuilt            bool
	closableTypeNames    map[string]struct{}
	closableBuilt        bool
	nilDeclFuncs         map[types.Object]*dsl.NilSignature
	nilDeclBuilt         bool
	nilDeclRequired      bool
	nilDeclRequiredBuilt bool
	logicalRulesByObj    map[types.Object][]*dsl.ArrowRule
	logicalRulesBuilt    bool
	fieldNilFields       map[types.Object]*dsl.PositionDecl
	fieldNilBuilt        bool
	presets              []*dsl.Preset
	configResolver       *config.Resolver
	configOverride       *config.Config
	subjectsUnion        subjectSet
	subjectsUnionOK      bool
	suppressedFuncRanges []suppressFuncRange
	suppressedLines      map[suppressLineKey]struct{}
	useAssertedFuncs     []useAssertedFuncRange
	useAssertedLines     map[suppressLineKey]map[string]struct{}
}

// useAssertedFuncRange is the function-level analogue of
// suppressFuncRange for vow:use caller-side discharge assertions.
// The same half-open AST range carries a per-function set of
// asserted subject names, so a function-doc `// vow:use ErrFoo`
// scopes its claim narrowly to ErrFoo without silencing leaks of
// other subjects in the same body.
type useAssertedFuncRange struct {
	start    token.Pos
	end      token.Pos
	subjects map[string]struct{}
}

// annoEntry caches a parseConditionsFromDoc result. The boolean
// preserves the "no annotation present" case so a repeated lookup
// returns the same outcome without re-walking the doc. The cached
// value is a []*dsl.Condition because the analyzer treats each
// doc-comment line as an independent case under the case-style
// semantics — a function with a single annotation line lands here
// as a 1-element slice and the iteration cost is negligible.
type annoEntry struct {
	conds []*dsl.Condition
	ok    bool
}

func newPassState(presets []*dsl.Preset, resolver *config.Resolver, override *config.Config) *passState {
	return &passState{
		anno:             map[*ast.FuncDecl]annoEntry{},
		parentMaps:       map[*ast.File]map[ast.Node]ast.Node{},
		presets:          presets,
		configResolver:   resolver,
		configOverride:   override,
		suppressedLines:  map[suppressLineKey]struct{}{},
		useAssertedLines: map[suppressLineKey]map[string]struct{}{},
	}
}

// parentMap returns the cached AST parent map for file, computing
// it on first lookup. The map is keyed by AST node and maps every
// non-root node to its immediate parent. Statement-scope callers
// (`stmtIsInsideGoroutine`, `closableEnclosingCallExpr`) reuse the
// same walk result when both inspect the same file in one pass.
func (s *passState) parentMap(file *ast.File) map[ast.Node]ast.Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached, ok := s.parentMaps[file]; ok {
		return cached
	}
	parents := buildParentMap(file)
	s.parentMaps[file] = parents
	return parents
}

// markFunctionSuppressed registers fn's AST range as a function-level
// vow:suppress site. Every must-consume diagnostic whose position
// lies inside the range is dropped at vowReport time. The range
// includes fn.Doc deliberately omitted: doc-position diagnostics do
// not exist for must-consume, and excluding the doc keeps the range
// equivalent to "the function body and signature the author saw".
func (s *passState) markFunctionSuppressed(fn *ast.FuncDecl) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suppressedFuncRanges = append(s.suppressedFuncRanges, suppressFuncRange{start: fn.Pos(), end: fn.End()})
}

// markLineSuppressed registers a (filename, line) pair as a line-
// level vow:suppress site. The resolution from comment line to
// suppressed line — trailing claims the comment's own line, leading
// claims the next line — happens before this method runs so the
// caller hands us the line authors expect to be silent.
func (s *passState) markLineSuppressed(filename string, line int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suppressedLines[suppressLineKey{filename: filename, line: line}] = struct{}{}
}

// functionIsSuppressed reports whether pos falls inside any
// function-level suppression range registered for the current pass.
// The lookup is linear in the number of suppressed functions; a
// typical file carries a handful at most, so the constant factor
// stays cheaper than the per-diagnostic AST walk a map-from-Pos
// would require to determine the enclosing FuncDecl.
func (s *passState) functionIsSuppressed(pos token.Pos) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.ContainsFunc(s.suppressedFuncRanges, func(r suppressFuncRange) bool {
		return pos >= r.start && pos <= r.end
	})
}

// lineIsSuppressed reports whether pos resolves to a (filename, line)
// pair the current pass has registered as line-level suppressed.
// The FileSet lookup happens inside the locked section so a parallel
// run that mutates suppressedLines cannot race the read.
func (s *passState) lineIsSuppressed(pass *analysis.Pass, pos token.Pos) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := pass.Fset.Position(pos)
	_, ok := s.suppressedLines[suppressLineKey{filename: p.Filename, line: p.Line}]
	return ok
}

// markFunctionUseAsserted registers a function-level vow:use
// assertion: fn's AST range is bound to a set of subject names for
// which discharge is unconditionally credited. Repeat invocations
// (e.g. multiple vow:use lines on the same function doc) accumulate
// subjects in the same range so authors can split a long subject
// list across several comments without losing earlier ones.
func (s *passState) markFunctionUseAsserted(fn *ast.FuncDecl, subjects []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.useAssertedFuncs {
		if s.useAssertedFuncs[i].start == fn.Pos() && s.useAssertedFuncs[i].end == fn.End() {
			for _, name := range subjects {
				s.useAssertedFuncs[i].subjects[name] = struct{}{}
			}
			return
		}
	}
	set := make(map[string]struct{}, len(subjects))
	for _, name := range subjects {
		set[name] = struct{}{}
	}
	s.useAssertedFuncs = append(s.useAssertedFuncs, useAssertedFuncRange{start: fn.Pos(), end: fn.End(), subjects: set})
}

// markLineUseAsserted registers a (filename, line) pair as a line-
// level vow:use site for the supplied subject names. Multiple
// markers on the same target line accumulate into a single set so
// `// vow:use ErrFoo` on one line and a separate `// vow:use ErrBar`
// trailing the same statement together silence both.
func (s *passState) markLineUseAsserted(filename string, line int, subjects []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := suppressLineKey{filename: filename, line: line}
	set := s.useAssertedLines[key]
	if set == nil {
		set = map[string]struct{}{}
		s.useAssertedLines[key] = set
	}
	for _, name := range subjects {
		set[name] = struct{}{}
	}
}

// functionAssertsUseOf reports whether pos falls inside any
// function-level vow:use range that asserts discharge for the
// named subject. The lookup is linear in the number of asserted
// functions; a typical file carries a handful at most, so the
// constant factor stays cheaper than the per-diagnostic AST walk a
// map-from-Pos would require to determine the enclosing FuncDecl.
func (s *passState) functionAssertsUseOf(pos token.Pos, subject string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.ContainsFunc(s.useAssertedFuncs, func(r useAssertedFuncRange) bool {
		if pos < r.start || pos > r.end {
			return false
		}
		_, ok := r.subjects[subject]
		return ok
	})
}

// lineAssertsUseOf reports whether pos resolves to a (filename, line)
// pair the current pass has registered as line-level vow:use-
// asserted for the named subject. The FileSet lookup happens inside
// the locked section so a parallel run that mutates useAssertedLines
// cannot race the read.
func (s *passState) lineAssertsUseOf(pass *analysis.Pass, pos token.Pos, subject string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := pass.Fset.Position(pos)
	set, ok := s.useAssertedLines[suppressLineKey{filename: p.Filename, line: p.Line}]
	if !ok {
		return false
	}
	_, ok = set[subject]
	return ok
}

// lineRangeAssertsUseOf reports whether any line in [startLine,
// endLine] inside filename carries a vow:use assertion for the
// named subject. The callee self-validation pass for vow:use uses
// this helper to count a statement-scope vow:use marker as an
// in-body discharge site.
func (s *passState) lineRangeAssertsUseOf(filename string, startLine, endLine int, subject string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for line := startLine; line <= endLine; line++ {
		set, ok := s.useAssertedLines[suppressLineKey{filename: filename, line: line}]
		if !ok {
			continue
		}
		if _, ok := set[subject]; ok {
			return true
		}
	}
	return false
}

// labelledSubjects returns the union of every preset's subject
// set under the current pass. The cache is computed lazily on
// first request so passes that never consult the union (e.g. test
// presets that declare no obligations) skip the per-preset
// findSubjects scan entirely.
//
// The label-aware matcher (`_` placeholder fall-through) consults
// this union to decide whether a return expression resolves to a
// labelled subject. A non-subject value is admitted by `_`; a
// labelled subject is not — the same semantic the dsl-reference
// documents under the placeholder term shape.
func (s *passState) labelledSubjects(pass *analysis.Pass) subjectSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subjectsUnionOK {
		return s.subjectsUnion
	}
	union := subjectSet{}
	for _, p := range s.presets {
		for obj := range findSubjects(pass, p) {
			union[obj] = struct{}{}
		}
	}
	s.subjectsUnion = union
	s.subjectsUnionOK = true
	return union
}

// useSet returns the pass-wide map from a FuncDecl object to
// the subjects it discharges via vow:use X annotations. Both
// caller-side discharge crediting (must-consume observe-class and
// SSA backward walk) and callee-side self-validation read through
// this map, so populating it once per pass guarantees a single
// source of truth. Built lazily and reused for the remainder of
// the pass.
func (s *passState) useSet(pass *analysis.Pass) map[types.Object][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.useBuilt {
		s.useFuncs = userUse(pass)
		s.useBuilt = true
	}
	return s.useFuncs
}

// emitSet returns the pass-wide map from a FuncDecl object to the
// subjects it declares as possible return values via vow:emit X
// annotations (and the equivalent vow:emit shorthand). Both
// caller-side chain authorisation crediting and callee-side
// self-validation read through this map, so populating it once per
// pass guarantees a single source of truth. Built lazily and
// reused for the remainder of the pass.
func (s *passState) emitSet(pass *analysis.Pass) map[types.Object][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.emitBuilt {
		s.emitFuncs = userEmit(pass)
		s.emitBuilt = true
	}
	return s.emitFuncs
}

// dischargedSet returns the pass-wide map of FuncDecl objects marked
// with a well-formed vow:discharged annotation. The map seeds every
// discharged-aware barrier (SSA leak walk, AST alias resolution,
// return-position validation) so caller-side verifications stop at
// the declared boundary. Built lazily and reused for the remainder
// of the pass; concurrent callers serialise through the same mutex
// that guards the rest of passState.
func (s *passState) dischargedSet(pass *analysis.Pass) map[types.Object]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dischargedBuilt {
		s.dischargedFuncs = userDischarged(pass)
		s.dischargedBuilt = true
	}
	return s.dischargedFuncs
}

// userTransducerSet returns the pass-wide map of FuncDecl objects
// promoted to transducer status. A `vow:use X` declaration on a
// function whose signature returns a single bool registers the
// function for X: each entry carries the subjects the function
// transduces to bool so the classifier can decide whether a
// given call-site, sitting in conditional context, discharges
// the reference's specific subject under the transduced-
// conditional path. The map is built lazily and cached for the
// remainder of the pass.
func (s *passState) userTransducerSet(pass *analysis.Pass) map[types.Object]transducerSpec {
	s.mu.Lock()
	built := s.transducersBuilt
	cached := s.transducers
	s.mu.Unlock()
	if built {
		return cached
	}
	out := userUseTransducers(pass)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.transducersBuilt {
		s.transducers = out
		s.transducersBuilt = true
	}
	return s.transducers
}

// annotation returns the parsed Conditions for decl, computing
// and caching them on first lookup. Identical lookups across the
// same pass share the cached value. The returned slice covers
// every doc-comment line that parsed successfully — vow:cond
// (lifted into Condition with Prereq=nil) and vow:cond
// (parsed directly). Multi-condition functions land here with
// length > 1; the common single-line case lands here with
// length 1 and behaves identically to the single-line path.
func (s *passState) annotation(decl *ast.FuncDecl, scope dsl.RuleScope) ([]*dsl.Condition, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.anno[decl]; ok {
		return entry.conds, entry.ok
	}
	conds, ok := parseConditionsFromDoc(decl, scope)
	s.anno[decl] = annoEntry{conds: conds, ok: ok}
	return conds, ok
}

// logicalRulesSet returns the pass-wide map from a FuncDecl
// object to the logical-arrow vow:cond rules its annotation
// carries. Subject-scoped conditions, structural arrow-rules,
// and atom-rules are skipped: they drive dedicated paths that do
// not route through this set. The map is built lazily on first
// access and reused for the remainder of the pass.
//
// state.annotation and state.ruleScope take the same mutex, so
// the build runs outside the locked section to keep the non-
// reentrant sync.Mutex deadlock-free. The two-phase pattern
// matches nilDeclSet: lock briefly to read the built flag,
// release, build under no lock, then lock again to publish.
func (s *passState) logicalRulesSet(pass *analysis.Pass) map[types.Object][]*dsl.ArrowRule {
	s.mu.Lock()
	built := s.logicalRulesBuilt
	cached := s.logicalRulesByObj
	s.mu.Unlock()
	if built {
		return cached
	}
	out := map[types.Object][]*dsl.ArrowRule{}
	for _, file := range pass.Files {
		scope := s.ruleScope(pass)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			conds, ok := s.annotation(fn, scope)
			if !ok {
				continue
			}
			rules := collectLogicalRules(conds)
			if len(rules) == 0 {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name]
			if obj == nil {
				continue
			}
			out[obj] = rules
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.logicalRulesBuilt {
		s.logicalRulesByObj = out
		s.logicalRulesBuilt = true
	}
	return s.logicalRulesByObj
}

// ruleScope returns the cached package-wide RuleScope, computing it
// on first lookup. Caching keeps the scan of pass.Files off the hot
// path and fires the unknown-path and alias-conflict diagnostics at
// most once per package.
func (s *passState) ruleScope(pass *analysis.Pass) dsl.RuleScope {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scopeBuilt {
		return s.scope
	}
	s.scope = scopeFromPackage(pass)
	s.scopeBuilt = true
	return s.scope
}

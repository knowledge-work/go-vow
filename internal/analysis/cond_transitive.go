package analysis

import (
	"slices"
	"strings"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// transitiveDepthLimit caps the chain walk so an annotation list
// with many connectable rules does not produce a combinatorial
// explosion of derived implications. A chain longer than this
// depth is unlikely to be the path an author intends as a
// readable contract; raise the cap if user reports motivate a
// deeper chain.
const transitiveDepthLimit = 5

// transitiveEntryKind names the seed origin a transitive chain
// expands from. A chain whose seed is an author-written
// implication carries the Forward kind; a chain whose seed is a
// derived contrapositive carries the Contrapositive kind. Each
// materialised entry stores the seed kind so the chain's origin
// stays readable without re-walking the rule pool.
type transitiveEntryKind int

const (
	transitiveEntryForward transitiveEntryKind = iota
	transitiveEntryContrapositive
)

// transitiveEntry pairs a derived chain rule with a description of
// the chain that produced it so the diagnostic announces which
// source rules combined to surface the violation. The Rule slot
// carries a synthetic implication whose Left and Right come from
// the chain's first and last operand respectively. Kind records
// the seed origin (author-written or derived contrapositive).
type transitiveEntry struct {
	Rule      *dsl.ArrowRule
	ChainDesc string
	Kind      transitiveEntryKind
}

// chainPoolEntry pairs a rule with its origin so the chain
// renderer can announce a derived contrapositive through the
// `contrapositive of` prefix rather than the negated surface the
// synthetic rule would otherwise print. The Origin slot also
// drives `seedKind` when the entry seeds an `expandTransitives`
// walk: a nil Origin marks an author-written rule whose chain
// reads as a forward derivation, and a non-nil Origin names the
// implication the derived contrapositive came from so the chain
// reads as a contrapositive-rotated derivation. Callers must set
// Origin to nil only when Rule is an author-written rule, and to
// a non-nil source implication only when Rule is the derived
// contrapositive of that source.
type chainPoolEntry struct {
	Rule   *dsl.ArrowRule
	Origin *dsl.ArrowRule
}

// expandTransitives walks every implication in pool and chains it
// forward up to the depth limit, emitting a synthetic `A => Z`
// rule for every chain whose intermediate operands link
// end-to-end. The walk seeds from each entry in seeds so the
// caller chooses which rules act as chain heads — author-written
// rules plus their contrapositives, so a violation surfaces
// through the most natural reading of the chain.
//
// The expansion only chains forward implications (`=>`) because a
// chain that mixes implications with biconditionals would yield a
// weaker forward implication that the evaluator cannot soundly
// distinguish from a pure-implication chain at this layer. A
// biconditional source carries the same operand pair as an
// implication when read in the forward direction, so the chain
// remains consistent.
//
// The cycle break is the chain itself: a rule already in chain is
// skipped on the next step so the walk terminates even when the
// rule graph contains a cycle the depth limit alone would not
// close.
func expandTransitives(seeds []chainPoolEntry, pool []chainPoolEntry) []transitiveEntry {
	var out []transitiveEntry
	var walk func(base chainPoolEntry, current chainPoolEntry, chain []chainPoolEntry, depth int)
	walk = func(base chainPoolEntry, current chainPoolEntry, chain []chainPoolEntry, depth int) {
		if depth >= transitiveDepthLimit {
			return
		}
		for _, next := range pool {
			if next.Rule == nil || next.Rule == current.Rule {
				continue
			}
			if next.Rule.Direction != dsl.DirImply {
				continue
			}
			if !expressionsEqual(current.Rule.Right, next.Rule.Left) {
				continue
			}
			if chainContains(chain, next.Rule) {
				continue
			}
			newChain := append(chain[:len(chain):len(chain)], next)
			derived := &dsl.ArrowRule{
				Direction: dsl.DirImply,
				Left:      base.Rule.Left,
				Right:     next.Rule.Right,
			}
			out = append(out, transitiveEntry{
				Rule:      derived,
				ChainDesc: renderChain(newChain),
				Kind:      seedKind(base),
			})
			walk(base, next, newChain, depth+1)
		}
	}
	for _, seed := range seeds {
		if seed.Rule == nil || seed.Rule.Direction != dsl.DirImply {
			continue
		}
		walk(seed, seed, []chainPoolEntry{seed}, 1)
	}
	return out
}

// seedKind reads a chainPoolEntry's origin and reports the
// transitive kind a chain seeded from it carries. A nil Origin
// marks an author-written rule so the chain reads as a forward
// derivation; a non-nil Origin marks a derived contrapositive so
// the chain reads as a contrapositive-rotated derivation.
func seedKind(seed chainPoolEntry) transitiveEntryKind {
	if seed.Origin == nil {
		return transitiveEntryForward
	}
	return transitiveEntryContrapositive
}

// expressionsEqual reports whether two Expression operands carry
// the same kind, reference, operator, and literal payload. The
// equality is structural and the helper does not normalise surface
// variations: the parser canonicalises each form before producing
// the Expression value, so two parsed expressions that share the
// same surface text reach this helper with byte-identical fields.
func expressionsEqual(a, b *dsl.Expression) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind || a.Op != b.Op {
		return false
	}
	if !comparisonMembersEqual(a.Members, b.Members) {
		return false
	}
	return referencesEqual(a.Ref, b.Ref)
}

// comparisonMembersEqual reports whether two ExprComparison
// right-hand-side sums carry the same members in the same order.
// The parser preserves source order so a stable byte-by-byte
// comparison is sufficient; the helper does not normalise
// permutations because the surface form encodes the order the
// author wrote.
func comparisonMembersEqual(a, b []dsl.ExprMember) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Nil != b[i].Nil || a[i].Literal != b[i].Literal {
			return false
		}
	}
	return true
}

// referencesEqual reports whether two Reference values name the
// same head and walk the same postfix chain. Each PathStep compares
// by kind plus the slot that kind reads (Name for field / key /
// ident-key shapes, Index for the integer-index shape).
func referencesEqual(a, b dsl.Reference) bool {
	if a.Kind != b.Kind || a.Name != b.Name || len(a.Path) != len(b.Path) {
		return false
	}
	for i := range a.Path {
		if a.Path[i].Kind != b.Path[i].Kind {
			return false
		}
		if a.Path[i].Name != b.Path[i].Name || a.Path[i].Index != b.Path[i].Index {
			return false
		}
	}
	return true
}

// chainContains reports whether r is already present in chain by
// pointer identity. The grouping helper interns each contrapositive
// once and shares the pointer between every pool entry that
// references it, so pointer equality recognises the same rule
// re-entered through any seed without confusing structurally equal
// rules built from independent derivations.
func chainContains(chain []chainPoolEntry, r *dsl.ArrowRule) bool {
	return slices.ContainsFunc(chain, func(c chainPoolEntry) bool {
		return c.Rule == r
	})
}

// renderChain returns an ` and `-separated rendering of the source
// rules that contributed to a derived chain. Author-written rules
// render through their `vow:cond` surface form; derived
// contrapositives render through a `contrapositive of vow:cond ...`
// prefix that names the original implication rather than printing
// the synthetic negated operands the chain walked through.
func renderChain(chain []chainPoolEntry) string {
	parts := make([]string, 0, len(chain))
	for _, c := range chain {
		if c.Origin != nil {
			parts = append(parts, "contrapositive of vow:cond "+c.Origin.String())
		} else {
			parts = append(parts, "vow:cond "+c.Rule.String())
		}
	}
	return strings.Join(parts, " and ")
}

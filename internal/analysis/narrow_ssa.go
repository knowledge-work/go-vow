package analysis

import (
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// fact identifies the atomic comparison the dominator-walk matcher
// looks for. Both supported facts compare a subject value against a
// kind of constant — the nil constant for nillable types, or the
// Go zero literal for primitive types.
type fact int

const (
	factNil  fact = iota // `param == nil`
	factZero             // `param == 0` / `""` / `false`
)

// blockProvesFact reports whether the atomic `fact` holds for
// `subject` at the entry of `block` under the requested sense.
//
// Two recognition paths are tried in order:
//
//  1. **Dominator walk** — some dominating *ssa.If guards the
//     chain on a side whose condition proves the fact for the
//     subject. This catches the common case where the proof-side
//     branch directly dominates the block under question.
//
//  2. **Predecessor join** — every incoming edge to `block`
//     either proves the fact directly (the edge is the
//     proof-side successor of an *ssa.If on a fact-matching
//     condition) or carries the fact forward from a predecessor
//     that itself satisfies path 1 or 2 recursively. The walk
//     uses a visited map so a cycle in the predecessor graph
//     (e.g. a loop back-edge) is treated conservatively as
//     non-proving rather than diverging.
//
// A recognised guard is accepted only when the subject also agrees the
// proof survives the distance to the use site — see
// narrowSubject.proofHoldsFrom.
//
// The matcher is intentionally conservative — only the patterns
// the helper recognises return true, so an unrecognised control
// shape reads as "not proved" and leaves the caller's own
// judgement in force. The join path admits proofs that the
// dominator walk alone misses: a
// path-merge block whose predecessors each prove the fact via
// independent comparisons (typical pattern: two arms of an
// if-else both performing the same `if subject == nil { return }`
// guard before flowing into a shared continuation block).
func blockProvesFact(block *ssa.BasicBlock, subject narrowSubject, f fact, positive bool) bool {
	return proves(block, subject, f, positive, make(map[*ssa.BasicBlock]bool))
}

// proves implements blockProvesFact with cycle guarding so the
// predecessor join can recurse safely across arbitrary SSA
// control-flow graphs. A block re-entered during the same walk
// returns false — the analyzer prefers a missed proof over an
// unsound one when a loop back-edge would otherwise close the
// search.
func proves(block *ssa.BasicBlock, subject narrowSubject, f fact, positive bool, visiting map[*ssa.BasicBlock]bool) bool {
	if block == nil || visiting[block] {
		return false
	}
	visiting[block] = true
	defer delete(visiting, block)
	if dominatorWalkProvesFact(block, subject, f, positive) {
		return true
	}
	if len(block.Preds) == 0 {
		return false
	}
	for _, pred := range block.Preds {
		if !edgeProvesFact(pred, block, subject, f, positive, visiting) {
			return false
		}
	}
	return true
}

// dominatorWalkProvesFact walks the dominator chain of `block`
// and returns true when some dominating *ssa.If guards the chain
// on a side whose condition proves `fact == positive` for
// `subject`. The walk visits each ancestor at most once and stops
// at the function entry. For every step it consults the
// immediate dominator's terminating If, decides which successor
// of that If dominates `block`, and asks whether that side
// establishes the fact under the requested sense.
func dominatorWalkProvesFact(block *ssa.BasicBlock, subject narrowSubject, f fact, positive bool) bool {
	for cur := block; cur != nil; cur = cur.Idom() {
		idom := cur.Idom()
		if idom == nil {
			return false
		}
		ifInstr, ok := terminatingIf(idom)
		if !ok {
			continue
		}
		side, ok := dominantSide(ifInstr, block)
		if !ok {
			continue
		}
		if branchProvesFact(ifInstr.Cond, subject, f, side, positive) && subject.proofHoldsFrom(idom) {
			return true
		}
	}
	return false
}

// edgeProvesFact reports whether crossing the edge `pred → succ`
// preserves the fact under the requested sense. Two sub-cases
// contribute:
//
//   - The edge is the proof-side successor of a terminating
//     *ssa.If on `pred` whose condition fact-matches `subject`.
//     Taking that edge establishes the fact regardless of what
//     held at pred's entry, so the fact holds on the succ side.
//   - Otherwise, the edge carries no new information; the fact
//     must already hold at pred-entry. That sub-question
//     recurses through `proves` so a chain of joins resolves
//     uniformly.
func edgeProvesFact(pred, succ *ssa.BasicBlock, subject narrowSubject, f fact, positive bool, visiting map[*ssa.BasicBlock]bool) bool {
	if ifInstr, ok := terminatingIf(pred); ok {
		host := ifInstr.Block()
		if host != nil && len(host.Succs) >= 2 {
			var then bool
			matched := false
			switch succ {
			case host.Succs[0]:
				then = true
				matched = true
			case host.Succs[1]:
				then = false
				matched = true
			}
			if matched && branchProvesFact(ifInstr.Cond, subject, f, then, positive) && subject.proofHoldsFrom(host) {
				return true
			}
		}
	}
	return proves(pred, subject, f, positive, visiting)
}

// terminatingIf returns the *ssa.If at the end of block when block
// ends in a conditional branch. Returns (nil, false) for blocks
// that fall through, panic, or unconditionally return.
func terminatingIf(block *ssa.BasicBlock) (*ssa.If, bool) {
	if block == nil || len(block.Instrs) == 0 {
		return nil, false
	}
	ifInstr, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	return ifInstr, ok
}

// dominantSide reports which successor of ifInstr dominates block.
// Returns (true, true) when the then successor dominates,
// (false, true) when the else successor does, and (_, false) when
// neither dominates (block sits on a merge that both successors
// reach). The If instruction itself does not expose successors —
// they live on its containing block, with index 0 as the then
// successor and index 1 as the else successor.
func dominantSide(ifInstr *ssa.If, block *ssa.BasicBlock) (then bool, ok bool) {
	host := ifInstr.Block()
	if host == nil || len(host.Succs) < 2 {
		return false, false
	}
	switch {
	case dominates(host.Succs[0], block):
		return true, true
	case dominates(host.Succs[1], block):
		return false, true
	}
	return false, false
}

// dominates reports whether ancestor dominates b (transitively) by
// walking b's idom chain. The SSA package's BasicBlock does not
// expose a public Dominates method, but the idom chain gives the
// same answer in O(depth).
func dominates(ancestor, b *ssa.BasicBlock) bool {
	for cur := b; cur != nil; cur = cur.Idom() {
		if cur == ancestor {
			return true
		}
	}
	return false
}

// branchProvesFact decides whether taking `then` side of an If whose
// condition is `cond` proves that the atomic `fact` holds for
// `subject` under the requested sense (positive=true means the fact
// itself, positive=false means its negation).
//
// Three condition shapes contribute:
//
//   - Equality comparison against the fact-target constant
//     (`subject == nil`, `subject != 0`, ...). EQL on the then side
//     proves the fact positively; NEQ proves it negated; the
//     else side flips both senses.
//   - `errors.As(<subject>, ...)` call. Returning true (the then
//     side) implies `subject` is non-nil because the standard
//     library short-circuits on a nil first argument.
//   - Comma-ok type assertion of `<subject>` (`_, ok := subject.(T)`).
//     `ok == true` implies `subject` has a concrete dynamic type,
//     so the nil-interface case is ruled out. The matcher reads
//     this through the `*ssa.Extract` of the type-assert tuple
//     used as the If condition; Go's type-switch lowering emits
//     the same shape, so a type-switch case body is admitted by
//     the same rule.
//
// Conditions the matcher does not recognise (literal-equality
// chains, sentinel-equality, transitive comparisons) return
// false so the branch contributes no proof.
func branchProvesFact(cond ssa.Value, subject narrowSubject, f fact, then, positive bool) bool {
	switch c := cond.(type) {
	case *ssa.BinOp:
		return binopProvesFact(c, subject, f, then, positive)
	case *ssa.Call:
		return callProvesFact(c, subject, f, then, positive)
	case *ssa.Extract:
		return extractProvesFact(c, subject, f, then, positive)
	}
	return false
}

// binopProvesFact recognises an `==` / `!=` comparison of subject
// against the fact-target constant. The truth table is the
// classic flip on then / else × EQL / NEQ.
func binopProvesFact(bop *ssa.BinOp, subject narrowSubject, f fact, then, positive bool) bool {
	if !compareToTargetConst(bop, subject, f) {
		return false
	}
	switch {
	case bop.Op == token.EQL && then:
		return positive
	case bop.Op == token.NEQ && then:
		return !positive
	case bop.Op == token.EQL && !then:
		return !positive
	case bop.Op == token.NEQ && !then:
		return positive
	}
	return false
}

// callProvesFact recognises `errors.As(<subject>, ...)` used as an
// If condition. A true result (then side) implies the first
// argument is non-nil; the else side carries no proof because
// errors.As also returns false on a type-mismatched non-nil error.
func callProvesFact(call *ssa.Call, subject narrowSubject, f fact, then, positive bool) bool {
	if f != factNil || !then || positive {
		return false
	}
	if !isErrorsAsCall(call) {
		return false
	}
	args := call.Call.Args
	return len(args) >= 1 && subject.matches(args[0])
}

// extractProvesFact recognises the boolean half of a comma-ok
// type assertion of `subject`. The Extract instruction projects
// the second tuple element of an *ssa.TypeAssert; a true result
// (then side) implies `subject` holds a concrete dynamic type and
// is therefore not the nil interface value. The mirror sense on
// the else side is not provable — the assertion can fail for a
// type-mismatched non-nil interface as well as for nil.
func extractProvesFact(ext *ssa.Extract, subject narrowSubject, f fact, then, positive bool) bool {
	if f != factNil || !then || positive {
		return false
	}
	if ext.Index != 1 {
		return false
	}
	ta, ok := ext.Tuple.(*ssa.TypeAssert)
	if !ok || !ta.CommaOk {
		return false
	}
	return subject.matches(ta.X)
}

// isErrorsAsCall reports whether the SSA Call instruction targets
// the standard-library `errors.As`. The package-path match keeps
// the recognition from triggering on a vendored or shadowed
// helper that happens to share the bare name.
func isErrorsAsCall(call *ssa.Call) bool {
	fn := call.Call.StaticCallee()
	if fn == nil || fn.Pkg == nil || fn.Pkg.Pkg == nil {
		return false
	}
	return fn.Pkg.Pkg.Path() == "errors" && fn.Name() == "As"
}

// compareToTargetConst reports whether bop compares `subject` against
// the "target" constant for `f` (nil for factNil; 0 / "" / false
// for factZero). The operand order is irrelevant — the matcher
// accepts both `subject OP const` and `const OP subject`.
func compareToTargetConst(bop *ssa.BinOp, subject narrowSubject, f fact) bool {
	switch {
	case subject.matches(bop.X) && isFactConst(bop.Y, f):
		return true
	case subject.matches(bop.Y) && isFactConst(bop.X, f):
		return true
	}
	return false
}

// isFactConst reports whether v is the SSA constant for `f`. The
// nil-fact accepts only the untyped-nil constant (Value == nil
// inside *ssa.Const); the zero-fact accepts the kind-typed Go zero
// literal (0 for ints, 0.0 for floats, "" for strings, false for
// bools).
func isFactConst(v ssa.Value, f fact) bool {
	c, ok := v.(*ssa.Const)
	if !ok {
		return false
	}
	if f == factNil {
		return c.Value == nil
	}
	return c.IsNil() || isZeroLiteral(c)
}

// isZeroLiteral reports whether c is the Go zero literal for its
// kind — `0`, `0.0`, `""`, or `false`. Typed nil pointers / nil
// interfaces are handled by the factNil branch above; this helper
// only fields the kind-typed zero comparisons factZero drives.
func isZeroLiteral(c *ssa.Const) bool {
	if c.Value == nil {
		return false
	}
	switch c.Value.Kind() {
	case constant.Int:
		return c.Int64() == 0
	case constant.String:
		return constant.StringVal(c.Value) == ""
	case constant.Bool:
		return !constant.BoolVal(c.Value)
	case constant.Float:
		return c.Float64() == 0
	case constant.Complex:
		re, im := real(c.Complex128()), imag(c.Complex128())
		return re == 0 && im == 0
	}
	return false
}

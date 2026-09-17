package analysis

import (
	"golang.org/x/tools/go/ssa"
	"slices"
)

// proofHoldsFrom reports whether a guard on this subject's field,
// found on guardBlock's terminating branch, still describes the field
// at the subject's use site. A field is memory rather than a value, so
// the guard's reading survives only while nothing writes to it in
// between.
//
// Two kinds of instruction end the proof:
//
//   - A store into the field or into the whole struct.
//   - A call handed the struct or the field's own address, since this
//     pass does not read callee bodies.
//
// The scan covers every instruction that can run after the branch and
// before the use, which is the intersection of what guardBlock reaches
// and what reaches the use block. guardBlock's own instructions are
// outside that span: they all run before the comparison, so the guard
// already reflects them.
//
// A use block on a cycle could see a write from a later iteration reach
// an earlier use, so a cycle through it declines rather than reasons.
//
// What survives is an unsoundness this pass does not close: a write
// reaching the field through a pointer the callee already held, or
// through a package-level variable, is invisible here. Closing it
// needs alias analysis. The stance matches the rest of the nil surface
// — recognise the shapes that can be recognised, and let the ones that
// cannot fall outside rather than guessing.
func (s fieldSubject) proofHoldsFrom(guardBlock *ssa.BasicBlock) bool {
	if guardBlock == nil || s.useBlock == nil {
		return false
	}
	span := blocksBetween(guardBlock, s.useBlock)
	if span[s.useBlock] && cycleReachesItself(s.useBlock, span) {
		return false
	}
	for block := range span {
		if block == guardBlock {
			continue
		}
		if s.blockWritesField(block) {
			return false
		}
	}
	return true
}

// blockWritesField reports whether block carries an instruction that
// ends the proof, scanning only the instructions that can run before
// the use — the whole block, or the part ahead of the using
// instruction when this is the use block.
func (s fieldSubject) blockWritesField(block *ssa.BasicBlock) bool {
	end := len(block.Instrs)
	if block == s.useBlock && s.useIndex >= 0 && s.useIndex < end {
		end = s.useIndex
	}
	return slices.ContainsFunc(block.Instrs[:end], func(instr ssa.Instruction) bool {
		return s.instructionEndsProof(instr)
	})
}

// instructionEndsProof reports whether instr can change what the
// guard read: a store landing on the field or on the enclosing struct,
// or a call handed either of those addresses.
func (s fieldSubject) instructionEndsProof(instr ssa.Instruction) bool {
	switch node := instr.(type) {
	case *ssa.Store:
		return node.Addr == s.base || s.matchesAddr(node.Addr)
	case ssa.CallInstruction:
		return s.callReceivesBase(node)
	}
	return false
}

// callReceivesBase reports whether the call hands the callee something
// it can write the field through: the struct itself, or the field's own
// address. Both the operand list and the receiver an invoke-mode call
// keeps outside it are inspected.
//
// The field-address case runs through the same matcher a store does, so
// it demands the same field index — a call given `&h.Other` cannot
// reach `h.Maybe` and leaves the proof standing.
func (s fieldSubject) callReceivesBase(call ssa.CallInstruction) bool {
	common := call.Common()
	if common == nil {
		return false
	}
	if s.handsOverField(common.Value) {
		return true
	}
	return slices.ContainsFunc(common.Args, func(arg ssa.Value) bool {
		return s.handsOverField(arg)
	})
}

// handsOverField reports whether passing v to a callee gives it a way
// to write this subject's field.
func (s fieldSubject) handsOverField(v ssa.Value) bool {
	return v != nil && (v == s.base || s.matchesAddr(v))
}

// blocksBetween returns the blocks that can run after from's branch
// and before to is reached: those reachable from from intersected with
// those that reach to. Both endpoints are included when they satisfy
// the two directions.
func blocksBetween(from, to *ssa.BasicBlock) map[*ssa.BasicBlock]bool {
	forward := reachableBlocks(from, func(b *ssa.BasicBlock) []*ssa.BasicBlock { return b.Succs })
	backward := reachableBlocks(to, func(b *ssa.BasicBlock) []*ssa.BasicBlock { return b.Preds })
	span := map[*ssa.BasicBlock]bool{}
	for block := range forward {
		if backward[block] {
			span[block] = true
		}
	}
	return span
}

// reachableBlocks returns start plus every block reachable from it by
// following edges, where edges names either the successor or the
// predecessor list.
func reachableBlocks(start *ssa.BasicBlock, edges func(*ssa.BasicBlock) []*ssa.BasicBlock) map[*ssa.BasicBlock]bool {
	seen := map[*ssa.BasicBlock]bool{}
	if start == nil {
		return seen
	}
	stack := []*ssa.BasicBlock{start}
	for len(stack) > 0 {
		block := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if block == nil || seen[block] {
			continue
		}
		seen[block] = true
		stack = append(stack, edges(block)...)
	}
	return seen
}

// cycleReachesItself reports whether block sits on a cycle confined to
// span. On such a cycle an instruction that appears after the use can
// run before it on a later iteration, which puts the write outside the
// range blockWritesField scans.
//
// Looking only at cycles inside span misses none, and that rests on how
// blocksBetween builds span: every block on a cycle through block
// reaches block and is reached from it, so the full forward-intersect-
// backward set contains the whole cycle. Narrowing blocksBetween — for
// speed, or to invalidate less — would let part of a cycle fall outside
// span and silently take a loop-carried write with it.
func cycleReachesItself(block *ssa.BasicBlock, span map[*ssa.BasicBlock]bool) bool {
	return slices.ContainsFunc(block.Succs, func(succ *ssa.BasicBlock) bool {
		return span[succ] &&
			(succ == block || reachesWithin(succ, block, span, map[*ssa.BasicBlock]bool{}))
	})
}

// reachesWithin reports whether target is reachable from start without
// leaving span.
func reachesWithin(start, target *ssa.BasicBlock, span, visiting map[*ssa.BasicBlock]bool) bool {
	if start == target {
		return true
	}
	if visiting[start] {
		return false
	}
	visiting[start] = true
	return slices.ContainsFunc(start.Succs, func(succ *ssa.BasicBlock) bool {
		return span[succ] && reachesWithin(succ, target, span, visiting)
	})
}

// Package nilDeclFieldNarrow pins the guard-driven discharge of a
// `?`-declared field read. A field is memory rather than a value, so
// the guard's read and the use site's read are separate loads of one
// address; the proof is carried by the address — the base value plus
// the field's index — and survives only while nothing writes through
// it in between.
package nilDeclFieldNarrow

// Holder carries a nillable field and a second one so a guard can be
// shown to speak about one field only.
type Holder struct {
	// vow:nil ?
	Maybe *int // want Maybe:"fieldNil\\(\\?\\)"

	// vow:nil ?
	Other *int // want Other:"fieldNil\\(\\?\\)"
}

// consume pins its parameter as non-nil, so a field read riding into it
// must be proved or reported.
//
// vow:nil (!)
func consume(p *int) {} // want consume:"nilDecl\\(!\\)"

// mutate receives the whole Holder, so it may write the field through
// the pointer it is handed.
func mutate(h *Holder) {}

// unrelated receives no Holder, so it cannot reach the field through an
// argument.
func unrelated(n int) {}

// ---------- proved ----------

// guardedThenUse guards the field immediately before reading it.
func guardedThenUse(h *Holder) {
	if h.Maybe != nil {
		consume(h.Maybe)
	}
}

// earlyReturnThenUse discharges the field with a short-circuiting
// guard and reads it past the guard.
func earlyReturnThenUse(h *Holder) {
	if h.Maybe == nil {
		return
	}
	consume(h.Maybe)
}

// elseBranchThenUse reads the field on the else side of an `== nil`
// guard.
func elseBranchThenUse(h *Holder) {
	if h.Maybe == nil {
		unrelated(0)
	} else {
		consume(h.Maybe)
	}
}

// guardedOnReceiver pins the receiver as the base value: the guard and
// the read both go through the method's receiver.
func (h *Holder) guardedOnReceiver() {
	if h.Maybe != nil {
		consume(h.Maybe)
	}
}

// unrelatedCallBetween places a call that receives no Holder between
// the guard and the read. It cannot reach the field through an
// argument, so the proof stands.
func unrelatedCallBetween(h *Holder) {
	if h.Maybe != nil {
		unrelated(1)
		consume(h.Maybe)
	}
}

// ---------- not proved ----------

// unguardedUse reads the field with no guard at all.
func unguardedUse(h *Holder) {
	consume(h.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
}

// guardedWrongSide reads the field on the side the guard proves nil.
func guardedWrongSide(h *Holder) {
	if h.Maybe == nil {
		consume(h.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
	}
}

// guardOnOtherField guards a different field of the same struct, which
// says nothing about this one.
func guardOnOtherField(h *Holder) {
	if h.Other != nil {
		consume(h.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
	}
}

// guardOnOtherBase guards the same field of a different struct value.
func guardOnOtherBase(h *Holder, g *Holder) {
	if g.Maybe != nil {
		consume(h.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
	}
}

// writeBetween writes the field after the guard, which replaces what
// the guard read.
func writeBetween(h *Holder) {
	if h.Maybe != nil {
		h.Maybe = nil
		consume(h.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
	}
}

// structWriteBetween replaces the whole struct after the guard, which
// carries the field with it.
func structWriteBetween(h *Holder) {
	if h.Maybe != nil {
		*h = Holder{}
		consume(h.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
	}
}

// callReceivingBaseBetween hands the struct to a callee between the
// guard and the read. The callee holds a pointer to it and this pass
// does not read callee bodies, so the proof ends there.
func callReceivingBaseBetween(h *Holder) {
	if h.Maybe != nil {
		mutate(h)
		consume(h.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
	}
}

// writeOnLoopBackEdge reads the field inside a loop whose next
// iteration writes it. The write sits after the read in the block, so
// bounding the scan at the read would miss it; a cycle through the read
// declines instead.
func writeOnLoopBackEdge(h *Holder, n int) {
	if h.Maybe != nil {
		for i := 0; i < n; i++ {
			consume(h.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
			h.Maybe = nil
		}
	}
}

// writeBeforeGuard writes the field ahead of the guard, so the guard
// reads what the write left and the proof is about that value.
func writeBeforeGuard(h *Holder, x *int) {
	h.Maybe = x
	if h.Maybe != nil {
		consume(h.Maybe)
	}
}

// writeThroughFieldPtr receives the field's own address, so it can
// write the field without ever seeing the struct.
func writeThroughFieldPtr(p **int) {}

// storeThroughFieldPtrBetween takes the field's address and stores
// through it. The store lands on the same address the guard read, so
// the proof ends there.
func storeThroughFieldPtrBetween(h *Holder) {
	if h.Maybe != nil {
		p := &h.Maybe
		*p = nil
		consume(h.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
	}
}

// callReceivingFieldPtrBetween hands the same address to a callee
// instead of storing through it here. The callee can write the field
// through it, so the proof has to end for the same reason.
func callReceivingFieldPtrBetween(h *Holder) {
	if h.Maybe != nil {
		writeThroughFieldPtr(&h.Maybe)
		consume(h.Maybe) // want `vow\[nil-safety\]: argument 1 \(p\) is potentially nil \(field Maybe declared nillable via vow:nil\)`
	}
}

// callReceivingOtherFieldPtrBetween hands a different field's address
// to the callee, which cannot reach this field through it.
func callReceivingOtherFieldPtrBetween(h *Holder) {
	if h.Maybe != nil {
		writeThroughFieldPtr(&h.Other)
		consume(h.Maybe)
	}
}

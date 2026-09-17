// Package nilDeclNarrow pins the guard-driven discharge of the
// vow:nil flow check. A value whose declaration admits nil stops
// being a violation once a nil guard proves it non-nil where the use
// site runs, so the flow check consults the SSA guard matcher before
// reporting. Without the discharge an author has no way to satisfy a
// `!`-required position from a `?`-declared source other than
// suppressing the diagnostic.
package nilDeclNarrow

// Container carries a `?` field so a field read can seed a nillable
// flow into an alias.
type Container struct {
	// vow:nil ?
	Maybe *int // want Maybe:"fieldNil\\(\\?\\)"
}

// consume pins its parameter as non-nil, so every argument reaching
// it must be proved non-nil or reported.
//
// vow:nil (!)
func consume(a *int) {} // want consume:"nilDecl\\(!\\)"

// ---------- parameter subjects ----------

// guardedParam proves the nillable parameter non-nil inside the
// then-branch of a `!= nil` guard.
//
// vow:nil (?)
func guardedParam(p *int) { // want guardedParam:"nilDecl\\(\\?\\)"
	if p != nil {
		consume(p)
	}
}

// earlyReturnParam proves the nillable parameter non-nil past an
// `== nil` guard whose branch short-circuits.
//
// vow:nil (?)
func earlyReturnParam(p *int) { // want earlyReturnParam:"nilDecl\\(\\?\\)"
	if p == nil {
		return
	}
	consume(p)
}

// elseBranchParam proves the nillable parameter non-nil on the else
// side of an `== nil` guard.
//
// vow:nil (?)
func elseBranchParam(p *int) { // want elseBranchParam:"nilDecl\\(\\?\\)"
	if p == nil {
		consume(nil) // want `vow\[nil-safety\]: argument 1 \(a\) is nil`
	} else {
		consume(p)
	}
}

// joinedGuardParam proves the nillable parameter non-nil at a merge
// block both of whose predecessors guard it independently.
//
// vow:nil (?,)
func joinedGuardParam(p *int, flag bool) { // want joinedGuardParam:"nilDecl\\(\\?,\\)"
	if flag {
		if p == nil {
			return
		}
	} else {
		if p == nil {
			return
		}
	}
	consume(p)
}

// unguardedParam reaches the `!` position with no proof, so the flow
// check reports.
//
// vow:nil (?)
func unguardedParam(p *int) { // want unguardedParam:"nilDecl\\(\\?\\)"
	consume(p) // want `vow\[nil-safety\]: argument 1 \(a\) may be nil through flow`
}

// guardedWrongSideParam calls into the `!` position on the side the
// guard proves nil, so the report stands.
//
// vow:nil (?)
func guardedWrongSideParam(p *int) { // want guardedWrongSideParam:"nilDecl\\(\\?\\)"
	if p == nil {
		consume(p) // want `vow\[nil-safety\]: argument 1 \(a\) may be nil through flow`
	}
}

// guardOnOtherValueParam guards a different value, which proves
// nothing about p.
//
// vow:nil (?,)
func guardOnOtherValueParam(p *int, q *int) { // want guardOnOtherValueParam:"nilDecl\\(\\?,\\)"
	if q != nil {
		consume(p) // want `vow\[nil-safety\]: argument 1 \(a\) may be nil through flow`
	}
}

// ---------- alias subjects ----------

// guardedAlias guards an alias of a `?`-declared field read. The
// subject is the value the call passes, so the guard on the alias
// discharges the position.
func guardedAlias(c *Container) {
	m := c.Maybe
	if m != nil {
		consume(m)
	}
}

// unguardedAlias passes the same alias with no guard.
func unguardedAlias(c *Container) {
	m := c.Maybe
	consume(m) // want `vow\[nil-safety\]: argument 1 \(a\) may be nil through flow`
}

// ---------- receiver offset ----------

// Sink is the receiver type whose method pins a `!` parameter, so the
// argument index has to step past the receiver operand on the SSA
// call.
type Sink struct{}

// Take pins its regular parameter as non-nil.
//
// vow:nil (!)
func (Sink) Take(a *int) {} // want Take:"nilDecl\\(!\\)"

// guardedMethodArg proves the nillable parameter non-nil at a method
// call site, where the SSA operand list carries the receiver ahead of
// the regular arguments.
//
// vow:nil (,?)
func guardedMethodArg(s Sink, p *int) { // want guardedMethodArg:"nilDecl\\(,\\?\\)"
	if p != nil {
		s.Take(p)
	}
}

// unguardedMethodArg pins that the same index mapping still reports
// when no guard stands.
//
// vow:nil (,?)
func unguardedMethodArg(s Sink, p *int) { // want unguardedMethodArg:"nilDecl\\(,\\?\\)"
	s.Take(p) // want `vow\[nil-safety\]: argument 1 \(a\) may be nil through flow`
}

// ---------- return subjects ----------

// guardedReturn proves the nillable parameter non-nil before handing
// it to a `!` return slot.
//
// vow:nil (?) !
func guardedReturn(p *int) *int { // want guardedReturn:"nilDecl\\(\\?\\) !"
	if p == nil {
		return new(int)
	}
	return p
}

// unguardedReturn returns the nillable parameter with no proof.
//
// vow:nil (?) !
func unguardedReturn(p *int) *int { // want unguardedReturn:"nilDecl\\(\\?\\) !"
	return p // want `vow\[nil-safety\]: return position 1 may be nil through flow`
}

// ---------- operand-list reshaping ----------

// Recv is the receiver type whose variadic method reshapes the SSA
// operand list: the trailing arguments fold into one slice, so the
// operand count stops tracking the syntactic argument count.
type Recv struct{}

// takesVariadic pins its variadic parameter as non-nil.
//
// vow:nil (!)
func (r *Recv) takesVariadic(rest ...*int) {} // want takesVariadic:"nilDecl\\(!\\)"

// takesOne pins one regular parameter as non-nil.
//
// vow:nil (!)
func (r *Recv) takesOne(a *int) {} // want takesOne:"nilDecl\\(!\\)"

// guardedRecvNonVariadic is the control for the shape that does map:
// the operand list is the receiver followed by the one argument, so
// guarding the receiver proves nothing about the argument.
//
// vow:nil (?,?)
func guardedRecvNonVariadic(r *Recv, p *int) { // want guardedRecvNonVariadic:"nilDecl\\(\\?,\\?\\)"
	if r != nil {
		r.takesOne(p) // want `vow\[nil-safety\]: argument 1 \(a\) may be nil through flow`
	}
}

// unguardedRecvVariadic is the control for the variadic shape with no
// receiver guard standing.
//
// vow:nil (?,?)
func unguardedRecvVariadic(r *Recv, p *int) { // want unguardedRecvVariadic:"nilDecl\\(\\?,\\?\\)"
	r.takesVariadic(p, p) // want `vow\[nil-safety\]: argument 1 \(rest\) may be nil through flow`
}

// guardedRecvVariadic guards the receiver of a variadic method call.
// The folded operand list makes the receiver sit where the first
// argument's operand would, so a subject resolved by counting would
// read the receiver and let the receiver's guard discharge a
// diagnostic about the argument. The argument carries no proof, so it
// must still report.
//
// vow:nil (?,?)
func guardedRecvVariadic(r *Recv, p *int) { // want guardedRecvVariadic:"nilDecl\\(\\?,\\?\\)"
	if r != nil {
		r.takesVariadic(p, p) // want `vow\[nil-safety\]: argument 1 \(rest\) may be nil through flow`
	}
}

// takesVariadicFree is a free function whose variadic parameter is
// pinned non-nil, so the folded operand list appears without a
// receiver in front of it.
//
// vow:nil (!)
func takesVariadicFree(rest ...*int) {} // want takesVariadicFree:"nilDecl\\(!\\)"

// unguardedVariadicFree reaches the variadic position with no proof.
//
// vow:nil (?)
func unguardedVariadicFree(p *int) { // want unguardedVariadicFree:"nilDecl\\(\\?\\)"
	takesVariadicFree(p) // want `vow\[nil-safety\]: argument 1 \(rest\) may be nil through flow`
}

// guardedVariadicFree pins the residual over-report at a variadic
// position: the operand the call passes is the folded slice, not p, so
// no operand corresponds to p and the guard on p cannot be matched
// against one. Declining to narrow keeps the receiver-shaped
// mis-narrow impossible, at the cost of this report standing.
//
// vow:nil (?)
func guardedVariadicFree(p *int) { // want guardedVariadicFree:"nilDecl\\(\\?\\)"
	if p != nil {
		takesVariadicFree(p) // want `vow\[nil-safety\]: argument 1 \(rest\) may be nil through flow`
	}
}

// takesTwo pins both regular parameters non-nil so a multi-value
// spread lands on two `!` positions at once.
//
// vow:nil (!,!)
func takesTwo(a, b *int) {} // want takesTwo:"nilDecl\\(!,!\\)"

// pair supplies two results so one syntactic argument expands into two
// operands.
func pair() (*int, *int) { return nil, nil }

// spreadMultiValue pins that a call whose single syntactic argument
// expands into two operands resolves no subject at either index. The
// nilness resolver does not classify a call result, so nothing is
// reported here either way; the case guards the index correspondence
// against a future resolver that does classify one.
func spreadMultiValue() {
	takesTwo(pair())
}

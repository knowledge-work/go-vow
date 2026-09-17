package analysis

import (
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// isLifecyclePairRule reports whether rule binds two distinct
// nil-check operands to signature positions whose types form a
// Closer / paired-resource lifecycle invariant. The recogniser
// pins the canonical pair shape: both operands carry an
// ExprNilCheck, one operand resolves to a signature position whose
// type satisfies isClosableType, the other operand resolves to a
// separate signature position whose type is a reference-like
// nil-bearing shape (pointer, interface, map, channel, slice, or
// function). The direction must be DirImply or DirEquiv so the
// rule expresses a lifecycle linkage; DirStructural rules ride the
// pre-existing structural pipeline and stay outside this axis.
//
// The caller in evaluateLogicalRuleAtCall gates the predicate on an
// empty descOverride so a transitive-closure synthetic rule that
// happens to land on a Closer / reference-like pair reports through
// the generic chain rendering rather than mixing the lifecycle-pair
// qualifier into the chain description.
func isLifecyclePairRule(pass *analysis.Pass, state *passState, rule *dsl.ArrowRule, sig *types.Signature, paramNames []string, recvName string) bool {
	if rule == nil || rule.Left == nil || rule.Right == nil {
		return false
	}
	if rule.Direction != dsl.DirImply && rule.Direction != dsl.DirEquiv {
		return false
	}
	if rule.Left.Kind != dsl.ExprNilCheck || rule.Right.Kind != dsl.ExprNilCheck {
		return false
	}
	if rule.Left.Ref.Name == "" || rule.Right.Ref.Name == "" {
		return false
	}
	if rule.Left.Ref.Name == rule.Right.Ref.Name {
		return false
	}
	leftType, leftOk := resolveSignatureReferenceType(sig, rule.Left.Ref, paramNames, recvName)
	if !leftOk {
		return false
	}
	rightType, rightOk := resolveSignatureReferenceType(sig, rule.Right.Ref, paramNames, recvName)
	if !rightOk {
		return false
	}
	closables := state.closableTypeSet(pass)
	leftClosable := isClosableType(leftType, closables)
	rightClosable := isClosableType(rightType, closables)
	// The non-Closer leg — whichever orientation carries it — must be
	// nil-bearing, because the pair-lifecycle narrative speaks of a
	// "non-nil resource": a value-typed paired resource (a struct
	// holding nil-bearing fields, an integer counter) falls through to
	// the generic logical-arrow diagnostic instead.
	if leftClosable && !rightClosable && typeIsNillable(rightType) {
		return true
	}
	if rightClosable && !leftClosable && typeIsNillable(leftType) {
		return true
	}
	return false
}

// resolveSignatureReferenceType binds ref to the matching signature
// position's type. The receiver wins when ref names the callee's
// declared receiver identifier; regular parameters resolve through
// the index-aligned paramNames list. Path-bearing references,
// positional `$N` slots, and identifiers that match no signature
// position report ok=false because the lifecycle-pair recogniser
// only operates on bare parameter / receiver bindings.
func resolveSignatureReferenceType(sig *types.Signature, ref dsl.Reference, paramNames []string, recvName string) (types.Type, bool) {
	if sig == nil || ref.Kind != dsl.RefIdent || ref.Name == "" || len(ref.Path) > 0 {
		return nil, false
	}
	if recvName != "" && ref.Name == recvName {
		if recv := sig.Recv(); recv != nil {
			return recv.Type(), true
		}
	}
	params := sig.Params()
	if params == nil {
		return nil, false
	}
	for i, name := range paramNames {
		if name == "" || name != ref.Name {
			continue
		}
		if i >= params.Len() {
			return nil, false
		}
		return params.At(i).Type(), true
	}
	return nil, false
}

// describeLifecyclePairCallViolation renders the call-site diagnostic
// for a Closer / paired-resource lifecycle violation. The message
// inserts a `lifecycle pair` qualifier before the rule rendering so
// the author distinguishes a pair-lifecycle offence from a generic
// logical-arrow contradiction while the rule text and the literal-
// asymmetry reason remain identical to the generic diagnostic.
func describeLifecyclePairCallViolation(desc string, reason string) string {
	return "vow[sentinel-error]: call violates lifecycle pair " + desc + ": " + reason
}

package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// checkNilDeclLayers reports a `vow:nil` decl whose token run does not
// match the number of layers the value it describes can hold nil at,
// in either direction. reportLayerCountMismatch holds what each
// direction means.
//
// A run is judged wherever one is written: at the position itself, and
// at each layer its nest body reaches. A position with no token of its
// own claims nothing about its own type and is not judged for it, while
// the runs inside its nest body still are.
func checkNilDeclLayers(pass *analysis.Pass, fn *ast.FuncDecl, sig *dsl.NilSignature) {
	if sig == nil || fn == nil {
		return
	}
	if recvExpr := receiverTypeExpr(fn); recvExpr != nil {
		checkPositionLayers(pass, fn.Pos(), positionLabel("recv", 0), recvExpr, sig.Recv)
	}
	checkNilDeclSignatureLayers(pass, fn.Pos(), fn.Type, sig)
}

// checkNilDeclSignatureLayers walks the parameter and return axes of
// one signature, reporting at anchor. A caller with no enclosing
// function declaration to point at passes its own declaration
// position.
func checkNilDeclSignatureLayers(pass *analysis.Pass, anchor token.Pos, ft *ast.FuncType, sig *dsl.NilSignature) {
	if sig == nil || ft == nil {
		return
	}
	walkFieldListPositions(ft.Params, sig.Params, func(idx int, expr ast.Expr, decl *dsl.PositionDecl) {
		checkPositionLayers(pass, anchor, positionLabel("param", idx), expr, decl)
	})
	walkFieldListPositions(ft.Results, sig.Returns, func(idx int, expr ast.Expr, decl *dsl.PositionDecl) {
		checkPositionLayers(pass, anchor, positionLabel("return", idx), expr, decl)
	})
}

// positionLabel names one signature position for a diagnostic message,
// as in `param position 0`. reportNestArity spells that same surface
// inline in its own format string, so the two diagnostics name a
// position alike against one marker; a change to the shape here leaves
// that one behind.
func positionLabel(axis string, idx int) string {
	return fmt.Sprintf("%s position %d", axis, idx)
}

// checkPositionLayers resolves the Go type standing at expr and checks
// decl against it.
func checkPositionLayers(pass *analysis.Pass, anchor token.Pos, label string, expr ast.Expr, decl *dsl.PositionDecl) {
	if expr == nil {
		return
	}
	checkDeclLayers(pass, anchor, label, pass.TypesInfo.TypeOf(expr), decl)
}

// checkDeclLayers judges decl's own token run against t, then descends
// through the nest body into the layers the container holds.
func checkDeclLayers(pass *analysis.Pass, anchor token.Pos, label string, t types.Type, decl *dsl.PositionDecl) {
	if decl == nil || t == nil {
		return
	}
	if decl.Nullness != dsl.NullnessNone {
		reportLayerCountMismatch(pass, anchor, label, t, 1+len(decl.InnerLayers))
	}
	if decl.Nest == nil {
		return
	}
	for _, layer := range nestLayers(t, decl.Nest) {
		checkDeclLayers(pass, anchor, label+" "+layer.name, layer.t, layer.decl)
	}
}

// reportLayerCountMismatch emits the diagnostic when declared names a
// different number of layers than t can hold nil at, in either
// direction. A surplus token names a layer that is not there, and a
// run that stops short leaves the layers below it unnamed while
// reading as the whole contract.
//
// The two directions rest on different halves of the count nilLayerCount
// returns. Naming more layers than exist needs its upper end, so an
// unsettled count reports nothing; naming fewer needs its lower end,
// which an unsettled count still carries.
func reportLayerCountMismatch(pass *analysis.Pass, anchor token.Pos, label string, t types.Type, declared int) {
	carried, settled := nilLayerCount(t)
	if declared == carried || (declared > carried && !settled) {
		return
	}
	vowReportNilSafetyf(
		pass,
		anchor,
		"vow[nil-safety]: %s %s declares %s but %s has %s that can hold nil",
		nilDeclMarker,
		label,
		pluralizeCount(declared, "nullness layer", "nullness layers"),
		types.TypeString(t, types.RelativeTo(pass.Pkg)),
		layerCountText(carried, settled),
	)
}

// layerCountText renders the layer count a type carries. Zero reads as
// a word so the message stays prose; any other unsettled count reads as
// the lower bound it is.
func layerCountText(n int, settled bool) string {
	if n == 0 {
		return "none"
	}
	if !settled {
		return "at least " + pluralizeCount(n, "layer", "layers")
	}
	return pluralizeCount(n, "layer", "layers")
}

// nestLayer pairs one layer a nest body reaches with the Go type
// standing at it.
type nestLayer struct {
	name string
	t    types.Type
	decl *dsl.PositionDecl
}

// nestLayers returns the layers t's container shape exposes to the
// nest body, each paired with the decl that describes it. A slice's
// inner decl is not among them: the element layer is written after the
// brackets, so `[?]` on a slice describes no layer of its own.
func nestLayers(t types.Type, nest *dsl.NestDecl) []nestLayer {
	switch resolveNestKind(t) {
	case nestKindSlice:
		slice, ok := peelPointers(t).Underlying().(*types.Slice)
		if !ok {
			return nil
		}
		return []nestLayer{{"element", slice.Elem(), nest.Value}}
	case nestKindMap:
		m, ok := peelPointers(t).Underlying().(*types.Map)
		if !ok {
			return nil
		}
		return []nestLayer{
			{"map key", m.Key(), mapKeyDecl(nest)},
			{"map value", m.Elem(), nest.Value},
		}
	case nestKindGeneric:
		return typeArgLayers(t, nest)
	}
	return nil
}

// mapKeyDecl returns the decl read as the key layer, or nil when the
// nest body does not carry exactly one inner decl. checkNestArity
// reports that shape, so this check stays out of it.
func mapKeyDecl(nest *dsl.NestDecl) *dsl.PositionDecl {
	if len(nest.InnerDecls) != 1 {
		return nil
	}
	return nest.InnerDecls[0]
}

// typeArgLayers pairs the nest's inner decls with the value the
// container holds at each type-argument position. An inner decl whose
// position typeArgLayerType cannot type is dropped rather than paired,
// so the result is not index-aligned with the nest body.
func typeArgLayers(t types.Type, nest *dsl.NestDecl) []nestLayer {
	var out []nestLayer
	for i, inner := range nest.InnerDecls {
		layerType, ok := typeArgLayerType(t, i)
		if !ok {
			continue
		}
		out = append(out, nestLayer{fmt.Sprintf("type argument %d", i), layerType, inner})
	}
	return out
}

// typeArgLayerType returns the type an inner decl at type-argument
// position i describes: the value a field of container holds at that
// position. That is the type argument wrapped in whatever the field
// declaration puts around it, and where several fields reach the
// parameter at different depths the deepest wins.
//
// ok is false where the count would be a guess: a field this walk
// cannot attribute to a type parameter, or one whose own layer count
// is unsettled. Either answer covers every position of the container
// rather than the one field alone, because a field the walk cannot
// read may be the one that reaches i. A container that is not a
// generic struct, or whose fields never reach parameter i, falls back
// to the type argument itself.
func typeArgLayerType(container types.Type, i int) (types.Type, bool) {
	origin, instance, ok := genericStructPair(container)
	if !ok {
		return typeParamConcreteType(container, i), true
	}
	var deepest types.Type
	best := 0
	for f := 0; f < origin.NumFields(); f++ {
		originType := origin.Field(f).Type()
		instanceType := instance.Field(f).Type()
		if types.Identical(originType, instanceType) {
			continue
		}
		paramIdx := typeParamIndex(container, peelPointers(originType))
		if paramIdx < 0 {
			return nil, false
		}
		if paramIdx != i {
			continue
		}
		count, settled := nilLayerCount(instanceType)
		if !settled {
			return nil, false
		}
		if deepest == nil || count > best {
			deepest, best = instanceType, count
		}
	}
	if deepest == nil {
		return typeParamConcreteType(container, i), true
	}
	return deepest, true
}

// genericStructPair returns the origin and instantiated struct bodies
// of a generic container, so a caller can read a field's declared type
// alongside the type the instantiation substituted into it. Pointers
// on container are peeled first. ok is false unless what they wrap is
// a named type whose body is a struct in both forms with the same
// field count, which is what lets a caller pair the two by index.
func genericStructPair(container types.Type) (origin, instance *types.Struct, ok bool) {
	named, ok := peelPointers(container).(*types.Named)
	if !ok {
		return nil, nil, false
	}
	origin, originOK := named.Origin().Underlying().(*types.Struct)
	instance, instanceOK := named.Underlying().(*types.Struct)
	if !originOK || !instanceOK || origin.NumFields() != instance.NumFields() {
		return nil, nil, false
	}
	return origin, instance, true
}

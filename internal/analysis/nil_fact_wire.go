package analysis

import (
	"bytes"
	"encoding/gob"

	"github.com/knowledge-work/go-vow/internal/dsl"
)

// The nil-decl facts carry dsl shapes that spell a platform
// position as a nil *dsl.PositionDecl, and both the parameter list
// and the nest body hold those positions in a slice. gob refuses a
// nil pointer as a slice element ("gob: encodeArray: nil element"),
// so a fact for a signature with any platform slot fails to encode
// the moment the driver serialises facts across packages.
//
// The wire mirror below replaces the nil-pointer-as-platform
// convention with an explicit Present flag for every slice element,
// which is the one shape gob can carry. Pointers survive where they
// sit in a struct field rather than a slice: gob omits a nil field
// and decodes it back to nil, and the recursion between a decl and
// its nest body needs the indirection anyway.
//
// The mirror is lossy in exactly one way that carries no meaning
// here: gob does not distinguish an empty slice from a nil one, so
// an empty Params round-trips to nil. Every consumer reads these
// through len and indexing, for which the two are the same.
type wireNilSignature struct {
	Present bool
	Recv    *wireNilDecl
	Params  []wireNilDecl
	Returns []wireNilDecl
	Subject dsl.SubjectChain
}

// wireNilDecl is the gob-safe mirror of *dsl.PositionDecl. Present
// distinguishes a declared position from a platform one, taking
// over the job the nil pointer does in the dsl shape.
type wireNilDecl struct {
	Present     bool
	Nullness    dsl.Nullness
	InnerLayers []dsl.Nullness
	Nest        *wireNilNest
}

// wireNilNest is the gob-safe mirror of *dsl.NestDecl. Value stays
// a pointer so the decl/nest recursion has an indirection to close
// over; it sits in a struct field, where gob handles nil.
type wireNilNest struct {
	InnerDecls []wireNilDecl
	Value      *wireNilDecl
}

// GobEncode serialises the fact through the wire mirror so a
// signature carrying platform positions survives the driver's
// cross-package fact encoding.
//
// vow:cond * -> _, error | nil
func (f *signatureNilFact) GobEncode() ([]byte, error) {
	return encodeGob(newWireNilSignature(f.Signature))
}

// GobDecode restores the fact from the wire mirror written by
// GobEncode.
//
// vow:cond * -> error | nil
func (f *signatureNilFact) GobDecode(data []byte) error {
	var wire wireNilSignature
	if err := decodeGob(data, &wire); err != nil {
		return err
	}
	f.Signature = wire.nilSignature()
	return nil
}

// GobEncode serialises the fact through the wire mirror. A field
// decl carries no slice of its own until it has a nest body, but
// the nest body holds the same nil-as-platform positions the
// signature mirror does, so it needs the same treatment.
//
// vow:cond * -> _, error | nil
func (f *fieldNilFact) GobEncode() ([]byte, error) {
	return encodeGob(newWireNilDecl(f.Decl))
}

// GobDecode restores the fact from the wire mirror written by
// GobEncode.
//
// vow:cond * -> error | nil
func (f *fieldNilFact) GobDecode(data []byte) error {
	var wire wireNilDecl
	if err := decodeGob(data, &wire); err != nil {
		return err
	}
	f.Decl = wire.positionDecl()
	return nil
}

// encodeGob renders v as a self-contained gob payload. The fact
// encoders share it so both sides of a round-trip agree on the
// framing.
//
// vow:cond * -> _, error | nil
func encodeGob(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeGob reads a payload written by encodeGob into v.
//
// vow:cond * -> error | nil
func decodeGob(data []byte, v any) error {
	return gob.NewDecoder(bytes.NewReader(data)).Decode(v)
}

// newWireNilSignature converts sig to its wire mirror. A nil sig
// yields the zero value, whose cleared Present flag decodes back to
// a nil signature.
func newWireNilSignature(sig *dsl.NilSignature) wireNilSignature {
	if sig == nil {
		return wireNilSignature{}
	}
	wire := wireNilSignature{
		Present: true,
		Params:  newWireNilDecls(sig.Params),
		Returns: newWireNilDecls(sig.Returns),
		Subject: sig.Subject,
	}
	if sig.Recv != nil {
		recv := newWireNilDecl(sig.Recv)
		wire.Recv = &recv
	}
	return wire
}

// nilSignature converts the wire mirror back to the dsl shape.
func (w wireNilSignature) nilSignature() *dsl.NilSignature {
	if !w.Present {
		return nil
	}
	return &dsl.NilSignature{
		Recv:    w.Recv.positionDecl(),
		Params:  positionDecls(w.Params),
		Returns: positionDecls(w.Returns),
		Subject: w.Subject,
	}
}

// newWireNilDecls converts an index-aligned decl list to its wire
// mirror, keeping the platform positions in place as cleared
// entries rather than dropping them: the index carries the meaning.
func newWireNilDecls(decls []*dsl.PositionDecl) []wireNilDecl {
	if decls == nil {
		return nil
	}
	out := make([]wireNilDecl, len(decls))
	for i, d := range decls {
		out[i] = newWireNilDecl(d)
	}
	return out
}

// positionDecls converts a wire decl list back to the dsl shape,
// restoring a nil pointer for every cleared entry.
func positionDecls(wires []wireNilDecl) []*dsl.PositionDecl {
	if wires == nil {
		return nil
	}
	out := make([]*dsl.PositionDecl, len(wires))
	for i, w := range wires {
		out[i] = w.positionDecl()
	}
	return out
}

// newWireNilDecl converts one decl to its wire mirror.
func newWireNilDecl(decl *dsl.PositionDecl) wireNilDecl {
	if decl == nil {
		return wireNilDecl{}
	}
	return wireNilDecl{
		Present:     true,
		Nullness:    decl.Nullness,
		InnerLayers: decl.InnerLayers,
		Nest:        newWireNilNest(decl.Nest),
	}
}

// positionDecl converts one wire decl back to the dsl shape. The
// pointer receiver lets a wire-side nil (an absent recv or nest
// value) collapse to the platform state through the same call.
func (w *wireNilDecl) positionDecl() *dsl.PositionDecl {
	if w == nil || !w.Present {
		return nil
	}
	return &dsl.PositionDecl{
		Nullness:    w.Nullness,
		InnerLayers: w.InnerLayers,
		Nest:        w.Nest.nestDecl(),
	}
}

// newWireNilNest converts a nest body to its wire mirror.
func newWireNilNest(nest *dsl.NestDecl) *wireNilNest {
	if nest == nil {
		return nil
	}
	wire := &wireNilNest{InnerDecls: newWireNilDecls(nest.InnerDecls)}
	if nest.Value != nil {
		value := newWireNilDecl(nest.Value)
		wire.Value = &value
	}
	return wire
}

// nestDecl converts a wire nest body back to the dsl shape.
func (w *wireNilNest) nestDecl() *dsl.NestDecl {
	if w == nil {
		return nil
	}
	return &dsl.NestDecl{
		InnerDecls: positionDecls(w.InnerDecls),
		Value:      w.Value.positionDecl(),
	}
}

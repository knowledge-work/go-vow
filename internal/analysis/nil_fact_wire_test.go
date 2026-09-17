package analysis

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/dsl"
)

// platformHeavySignature is a signature whose parameter list, return
// list, and nest body all leave a position at platform. Every one of
// those is a nil element inside a slice, which is precisely what the
// unmirrored fact could not hand to gob.
func platformHeavySignature() *dsl.NilSignature {
	return &dsl.NilSignature{
		Recv: &dsl.PositionDecl{Nullness: dsl.NullnessNonNil},
		Params: []*dsl.PositionDecl{
			nil,
			{
				Nullness:    dsl.NullnessNillable,
				InnerLayers: []dsl.Nullness{dsl.NullnessNonNil},
			},
			nil,
		},
		Returns: []*dsl.PositionDecl{
			{
				Nullness: dsl.NullnessNonNil,
				Nest: &dsl.NestDecl{
					InnerDecls: []*dsl.PositionDecl{nil, {Nullness: dsl.NullnessNillable}},
					Value:      &dsl.PositionDecl{Nullness: dsl.NullnessNonNil},
				},
			},
			nil,
		},
		Subject: dsl.SubjectChain{"cb"},
	}
}

func TestSignatureNilFactGobRoundTrip(t *testing.T) {
	in := &signatureNilFact{Signature: platformHeavySignature()}

	var buf bytes.Buffer
	assert.MustNoError(t, "Encode", gob.NewEncoder(&buf).Encode(in))
	var got signatureNilFact
	assert.MustNoError(t, "Decode", gob.NewDecoder(&buf).Decode(&got))
	assert.DeepEqual(t, "round-tripped Signature", got.Signature, in.Signature)
}

func TestSignatureNilFactGobRoundTripEmpty(t *testing.T) {
	var buf bytes.Buffer
	assert.MustNoError(t, "Encode", gob.NewEncoder(&buf).Encode(&signatureNilFact{}))
	var got signatureNilFact
	assert.MustNoError(t, "Decode", gob.NewDecoder(&buf).Decode(&got))
	assert.Nil(t, "round-tripped Signature", got.Signature)
}

func TestFieldNilFactGobRoundTrip(t *testing.T) {
	in := &fieldNilFact{Decl: &dsl.PositionDecl{
		Nullness: dsl.NullnessNillable,
		Nest: &dsl.NestDecl{
			InnerDecls: []*dsl.PositionDecl{nil, {Nullness: dsl.NullnessNonNil}},
		},
	}}

	var buf bytes.Buffer
	assert.MustNoError(t, "Encode", gob.NewEncoder(&buf).Encode(in))
	var got fieldNilFact
	assert.MustNoError(t, "Decode", gob.NewDecoder(&buf).Decode(&got))
	// Raw, not assert.DeepEqual: the platform positions this test is
	// about sit at Decl.Nest.InnerDecls, and %+v expands one level
	// through a pointer before printing the next as an address — so a
	// whole-Decl render stops at Nest. Printing Nest itself reaches
	// them. (The signature round trip above needs no such care: its
	// platform positions sit one level up, at Signature.Params.)
	if !reflect.DeepEqual(got.Decl, in.Decl) {
		t.Errorf("round-tripped Nest = %+v, want %+v", got.Decl.Nest, in.Decl.Nest)
	}
}

func TestFieldNilFactGobRoundTripEmpty(t *testing.T) {
	var buf bytes.Buffer
	assert.MustNoError(t, "Encode", gob.NewEncoder(&buf).Encode(&fieldNilFact{}))
	var got fieldNilFact
	assert.MustNoError(t, "Decode", gob.NewDecoder(&buf).Decode(&got))
	assert.Nil(t, "round-tripped Decl", got.Decl)
}

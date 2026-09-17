package analysis

import (
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/dsl"
)

// These tests pin the sentinel-chain contract for analysis-package
// failures: every error returned from a parser exposed to the
// annotation pipeline must wrap one of the package-level sentinels so
// callers can discriminate failure modes with errors.Is instead of
// substring matching on the rendered message.

func TestErrUnknownMarker_wrappedByParsePayloadByMarker(t *testing.T) {
	_, err := parsePayloadByMarker("not-a-real-marker", "payload", dsl.RuleScope{})
	assert.ErrorIs(t, `parsePayloadByMarker("not-a-real-marker", ...)`, err, ErrUnknownMarker)
}

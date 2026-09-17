package analysis

import (
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

// TestStripMarkerScopeChain pins the parser behaviour for the
// chain-aware helper that drives every marker family's scope
// recognition. The table covers bare markers, single-level scopes,
// chained scopes, and malformed bracket shapes the helper rejects.
func TestStripMarkerScopeChain(t *testing.T) {
	const marker = "vow:emit"
	type chainCase struct {
		line        string
		wantChain   []string
		wantPayload string
		wantOK      bool
	}

	tabletest.Run(t, map[string]chainCase{
		"bare marker":                           {"vow:emit", nil, "", true},
		"bare with payload":                     {"vow:emit X", nil, "X", true},
		"tab separator":                         {"vow:emit\tX", nil, "X", true},
		"single scope":                          {"vow:emit[cb] X", []string{"cb"}, "X", true},
		"single scope empty payload":            {"vow:emit[cb]", []string{"cb"}, "", true},
		"chain two":                             {"vow:emit[outer][inner] X", []string{"outer", "inner"}, "X", true},
		"chain three":                           {"vow:emit[a][b][c] X", []string{"a", "b", "c"}, "X", true},
		"chain with whitespace inside brackets": {"vow:emit[ a ][ b ] X", []string{"a", "b"}, "X", true},
		"chain empty payload":                   {"vow:emit[a][b]", []string{"a", "b"}, "", true},
		"missing marker prefix":                 {"vow:use X", nil, "", false},
		"missing close bracket":                 {"vow:emit[cb X", nil, "", false},
		"empty bracket":                         {"vow:emit[] X", nil, "", false},
		"empty inner bracket":                   {"vow:emit[a][] X", nil, "", false},
		"trailing open bracket":                 {"vow:emit[a][ X", nil, "", false},
		"unsupported separator":                 {"vow:emitX", nil, "", false},
	}, func(t *testing.T, c chainCase) {
		gotChain, gotPayload, gotOK := stripMarkerScopeChain(c.line, marker)
		assert.MustEqual(t, "ok", gotOK, c.wantOK)
		if !c.wantOK {
			return
		}
		assert.DeepEqual(t, "chain", gotChain, c.wantChain)
		assert.Equal(t, "payload", gotPayload, c.wantPayload)
	})
}

// TestStripMarkerScope pins the single-subject wrapper's behaviour
// so chain syntax routes to ok=false at the caller boundary.
func TestStripMarkerScope(t *testing.T) {
	const marker = "vow:emit"
	type scopeCase struct {
		line        string
		wantSubject string
		wantPayload string
		wantOK      bool
	}

	tabletest.Run(t, map[string]scopeCase{
		"bare":              {"vow:emit", "", "", true},
		"bare with payload": {"vow:emit X", "", "X", true},
		"single scope":      {"vow:emit[cb] X", "cb", "X", true},
		"chain rejected":    {"vow:emit[a][b] X", "", "", false},
		"malformed bracket": {"vow:emit[ X", "", "", false},
	}, func(t *testing.T, c scopeCase) {
		gotSubject, gotPayload, gotOK := stripMarkerScope(c.line, marker)
		assert.MustEqual(t, "ok", gotOK, c.wantOK)
		if !c.wantOK {
			return
		}
		assert.Equal(t, "subject", gotSubject, c.wantSubject)
		assert.Equal(t, "payload", gotPayload, c.wantPayload)
	})
}

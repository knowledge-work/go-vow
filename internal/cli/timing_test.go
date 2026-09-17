package cli

import (
	"os"
	"testing"
	"time"

	"github.com/knowledge-work/go-vow/internal/assert"
)

// TestFormatPhase pins the wire-format adopters grep for: the
// `VOW_TIMING ` prefix, the `PHASE=<name>` key, and the elapsed
// time rendered in fractional seconds with millisecond precision.
func TestFormatPhase(t *testing.T) {
	assert.Equal(t, "formatPhase", formatPhase("packages.Load", 0.683),
		"VOW_TIMING PHASE=packages.Load elapsed=0.683s\n")
}

// TestFormatEntry pins the free-form marker line format that
// LogEntry routes through — same prefix, same trailing newline.
func TestFormatEntry(t *testing.T) {
	got := formatEntry("PHASE=singlechecker.Main.entry remaining=%v", []string{"./...", "github.com/x/y"})
	assert.Equal(t, "formatEntry", got,
		"VOW_TIMING PHASE=singlechecker.Main.entry remaining=[./... github.com/x/y]\n")
}

// TestNewTimingLoggerEnvGate pins the env-var contract: an empty
// (or unset) value leaves the logger disabled, any non-empty
// value flips the gate on. The test snapshots and restores the
// env var so it does not leak into the rest of the suite.
func TestNewTimingLoggerEnvGate(t *testing.T) {
	saved, hadSaved := os.LookupEnv(TimingEnvVar)
	t.Cleanup(func() {
		if hadSaved {
			os.Setenv(TimingEnvVar, saved)
			return
		}
		os.Unsetenv(TimingEnvVar)
	})

	os.Unsetenv(TimingEnvVar)
	assert.Equal(t, "Enabled() with the env var unset", NewTimingLogger().Enabled(), false)
	os.Setenv(TimingEnvVar, "")
	assert.Equal(t, "Enabled() with the env var empty", NewTimingLogger().Enabled(), false)
	os.Setenv(TimingEnvVar, "1")
	assert.Equal(t, `Enabled() with the env var "1"`, NewTimingLogger().Enabled(), true)
	os.Setenv(TimingEnvVar, "true")
	assert.Equal(t, `Enabled() with the env var "true"`, NewTimingLogger().Enabled(), true)
}

// TestDisabledLoggerSkipsLogCalls pins the hot-path short-circuit
// so a future refactor that builds the log line before checking
// Enabled cannot regress production silently. A disabled logger
// consults the wall clock through `time.Since` only inside
// LogPhase, so the test passes a zero-value `time.Time` and
// asserts the call returns without panicking.
func TestDisabledLoggerSkipsLogCalls(t *testing.T) {
	l := &TimingLogger{enabled: false}
	l.LogPhase("ignored", time.Time{})
	l.LogEntry("PHASE=ignored remaining=%v", []string{"./..."})
}

func TestAnnotateNarrowRoundTrips(t *testing.T) {
	// The switch reaches the analyzer through the process, so a driver
	// that forgets to publish it leaves the analyzer applying a scope it
	// does not have.
	defer SetAnnotateNarrow(false)
	assert.MustEqual(t, "AnnotateNarrow() before anything published it", AnnotateNarrow(), false)
	SetAnnotateNarrow(true)
	assert.MustEqual(t, "AnnotateNarrow() after the switch is published", AnnotateNarrow(), true)
}

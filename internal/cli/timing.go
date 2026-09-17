package cli

import (
	"fmt"
	"os"
	"time"
)

// TimingEnvVar names the environment variable that opts into
// per-phase wall-time logging on the vow driver. Setting the
// variable to any non-empty value enables the logger; the default
// (unset or empty) keeps the run silent so users who do not opt in
// see no extra stderr output.
const TimingEnvVar = "VOW_DEBUG_TIMING"

// TimingLogger emits per-phase wall-time lines to stderr when the
// `VOW_DEBUG_TIMING` env var is set. Lines carry the `VOW_TIMING`
// prefix so log scrapers can grep them out of mixed output.
//
// The logger writes through `os.Stderr.WriteString` so the
// underlying `*os.File` close obligation stays with the
// process-owned stderr handle; the helper never wraps stderr in
// an interface, mirroring the way the driver's other diagnostic
// messages reach the user.
type TimingLogger struct {
	enabled bool
}

// NewTimingLogger reads the `VOW_DEBUG_TIMING` env var and returns
// a logger whose calls write only when the gate is enabled. The
// returned value is always non-nil; a disabled logger
// short-circuits every call so the driver hot path stays cheap.
func NewTimingLogger() *TimingLogger {
	return &TimingLogger{enabled: os.Getenv(TimingEnvVar) != ""}
}

// LogPhase emits a `PHASE=<name> elapsed=<seconds>` line with the
// elapsed wall time rendered in fractional seconds. The call is a
// no-op when the env-var gate is disabled.
func (l *TimingLogger) LogPhase(phase string, start time.Time) {
	if !l.enabled {
		return
	}
	os.Stderr.WriteString(formatPhase(phase, time.Since(start).Seconds()))
}

// LogEntry emits a free-form entry line through the same prefix.
// Phase records flow through LogPhase; this helper is for one-off
// markers (e.g. the `singlechecker.Main.entry` boundary the
// resolver does not own). The call is a no-op when the gate is
// disabled.
func (l *TimingLogger) LogEntry(format string, args ...any) {
	if !l.enabled {
		return
	}
	os.Stderr.WriteString(formatEntry(format, args...))
}

// Enabled reports whether the env-var gate enabled the logger.
// Callers consult it when they want to skip preparatory work that
// is only useful for the logger output.
func (l *TimingLogger) Enabled() bool {
	return l.enabled
}

// formatPhase renders a phase line in the documented
// `VOW_TIMING PHASE=<name> elapsed=<seconds>s\n` shape. Pure
// formatting keeps the output testable without intercepting
// stderr.
func formatPhase(phase string, seconds float64) string {
	return fmt.Sprintf("VOW_TIMING PHASE=%s elapsed=%.3fs\n", phase, seconds)
}

// formatEntry renders a free-form entry line with the same
// `VOW_TIMING ` prefix and a trailing newline so log scrapers
// can grep against it the same way they grep phase records.
func formatEntry(format string, args ...any) string {
	return "VOW_TIMING " + fmt.Sprintf(format, args...) + "\n"
}

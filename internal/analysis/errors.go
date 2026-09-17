package analysis

import "errors"

// ErrUnknownMarker marks an internal dispatch failure: the payload
// classifier handed parsePayloadByMarker a marker name that the
// switch arms do not cover. splitDocMarker always returns a known
// marker, so this error path only surfaces when an internal caller
// drifts out of sync with the marker enumeration.
//
// vow:define @Sentinel
var ErrUnknownMarker = errors.New("unknown annotation marker")

// ErrSuppressMissingReason marks a malformed vow:suppress marker:
// the bare form `vow:suppress`, the empty-body form
// `vow:suppress:`, or a whitespace-only body. Callers can
// discriminate suppress-annotation failures from other analyzer
// errors with errors.Is(err, ErrSuppressMissingReason).
//
// vow:define @Sentinel
var ErrSuppressMissingReason = errors.New("vow:suppress requires a reason")

package definition

import (
	"encoding/json"
	"fmt"
	"time"
)

// definitionTimestampLayouts is the CLOSED set of layouts accepted for the
// two Definition catalog timestamps — TemplateMetadata.created_timestamp
// and StoredQueryMetadata.saved (REQ-144, ADR 0019). Deployments emit more
// than the RFC 3339 that encoding/json accepts, and an unreadable
// timestamp fails the containing item and with it the whole list. The set
// widens what is readable; it does not isolate a bad entry, so a value
// outside the set still costs the caller the list.
//
// The set is closed on purpose: decode never reaches this tolerance
// through a format-guessing parser, because an open set has no reviewable
// boundary and would absorb the next malformed input instead of reporting
// it. Adding or removing a layout amends § REQ-144.
//
// No fractional-second variants are listed: time.Parse absorbs a
// fractional second following a seconds element even when the layout omits
// it. The minute-precision layout carries no seconds element, so it
// accepts neither seconds nor fractions — a value carrying them matches
// one of the seconds-bearing layouts instead.
var definitionTimestampLayouts = []string{
	time.RFC3339Nano,
	// RFC3339Nano's fractional field is optional, so it already subsumes
	// RFC3339; the entry is kept because § REQ-144 names both.
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	// Space-separated: deployment interop only. Not ISO 8601 extended
	// (no `T`), not a REST-legal or pin-example form; accepted solely
	// because deployments emit it.
	"2006-01-02 15:04:05",
}

// parseDefinitionTimestamp decodes a Definition metadata timestamp
// leniently across definitionTimestampLayouts (REQ-144). field names the
// wire key, so a failure says which one could not be read.
//
// An absent key, a JSON null, and an empty string each yield the zero
// time.Time with no error — a descriptor whose timestamp the server did
// not populate is still usable, and the caller distinguishes the case with
// IsZero. A value carrying no zone indicator decodes as UTC: the client
// host's local zone would read one response as different instants on
// different machines.
//
// A non-empty value matching no layout is an error, never a silent zero —
// a zero would present an instant the server never sent as though it had.
// The offending value appears in the message: catalog timestamps are
// design-time metadata, not clinical content (§ REQ-144).
func parseDefinitionTimestamp(field string, raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, fmt.Errorf("%s: %w", field, err)
	}
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range definitionTimestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%s: cannot parse %q as a known timestamp layout", field, s)
}

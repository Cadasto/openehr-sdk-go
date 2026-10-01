package rm

// REQ-123 — validity of the ISO 8601-backed temporal data values.
//
// Each method reports whether the receiver's `value` parses with the same
// parser the component accessors and Go-bridge conversions use, so the
// partial forms (`2024`, `2024-03`) and DV_DURATION's documented deviations
// are valid. It stands in for the BASE predicates valid_iso8601_date,
// valid_iso8601_time, valid_iso8601_date_time and valid_iso8601_duration.
// The date, time and date-time parse accepts the forms those predicates
// define: extended and compact layouts, a comma or dot fraction, and a zone
// of Z, ±hh, ±hhmm or ±hh:mm. The methods never panic; a nil receiver is not
// valid. Hour 24 is refused, as BASE says "24:00:00" is not allowed. A
// duration follows P[nnY][nnM][nnW][nnD][T[nnH][nnM][nnS]] with each
// designator at most once and in that order.

// ValidISO8601 reports whether the date's value is a valid ISO 8601 date,
// partial forms included.
func (d *DVDate) ValidISO8601() bool {
	if d == nil {
		return false
	}
	_, err := parseDate(d.Value)
	return err == nil
}

// ValidISO8601 reports whether the time's value is a valid ISO 8601 time of
// day, partial forms included.
func (d *DVTime) ValidISO8601() bool {
	if d == nil {
		return false
	}
	_, err := parseTime(d.Value)
	return err == nil
}

// ValidISO8601 reports whether the date-time's value is a valid ISO 8601
// date-time, partial forms included.
func (d *DVDateTime) ValidISO8601() bool {
	if d == nil {
		return false
	}
	_, _, err := d.split()
	return err == nil
}

// ValidISO8601 reports whether the duration's value is a valid ISO 8601
// duration, including openEHR's leading sign and mixed `W` designator.
func (d *DVDuration) ValidISO8601() bool {
	if d == nil {
		return false
	}
	_, err := parseDuration(d.Value)
	return err == nil
}

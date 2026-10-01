package rm

// REQ-123 — validity of the ISO 8601-backed temporal data values.
//
// Each method reports whether the receiver's `value` parses with the same
// parser the component accessors and Go-bridge conversions use, so the
// partial forms (`2024`, `2024-03`) and DV_DURATION's documented deviations
// are valid. It stands in for the BASE predicates valid_iso8601_date,
// valid_iso8601_time, valid_iso8601_date_time and valid_iso8601_duration.
// The methods never panic.

// ValidISO8601 reports whether the date's value is a valid ISO 8601 date,
// partial forms included.
func (d *DVDate) ValidISO8601() bool {
	_, err := parseDate(d.Value)
	return err == nil
}

// ValidISO8601 reports whether the time's value is a valid ISO 8601 time of
// day, partial forms included.
func (d *DVTime) ValidISO8601() bool {
	_, err := parseTime(d.Value)
	return err == nil
}

// ValidISO8601 reports whether the date-time's value is a valid ISO 8601
// date-time, partial forms included.
func (d *DVDateTime) ValidISO8601() bool {
	_, _, err := d.split()
	return err == nil
}

// ValidISO8601 reports whether the duration's value is a valid ISO 8601
// duration, including openEHR's leading sign and mixed `W` designator.
func (d *DVDuration) ValidISO8601() bool {
	_, err := parseDuration(d.Value)
	return err == nil
}

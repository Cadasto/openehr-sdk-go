package ehr

import "time"

// FormatVersionAtTime returns t as the value of a version_at_time query
// parameter: the instant in UTC, in RFC 3339 format, with its fraction
// of a second. Trailing zeros of the fraction are dropped and a whole
// second has none, so 19:30:22.765 at +01:00 on 20 January 2015 becomes
// "2015-01-20T18:30:22.765Z". The leaf clients use it for every
// version_at_time they send; a caller building such a request by hand
// can use it too.
func FormatVersionAtTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

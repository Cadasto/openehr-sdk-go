package ehr

import (
	"fmt"
	"time"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// FormatVersionAtTime returns t as the value of a version_at_time query
// parameter: the instant in UTC, in RFC 3339 format, with its fraction
// of a second. Trailing zeros of the fraction are dropped and a whole
// second has none, so 19:30:22.765 at +01:00 on 20 January 2015 becomes
// "2015-01-20T18:30:22.765Z".
//
// RFC 3339 writes the years 0000 to 9999 only. A time whose UTC year
// falls outside them has no such form, so FormatVersionAtTime returns ""
// and an error wrapping [transport.ErrInvalidConfig]. The year is taken
// in UTC: 31 December 9999 at -05:00 is already the year 10000.
//
// The leaf clients use it for every version_at_time they send, and
// refuse such a time before any request; a caller building such a
// request by hand can use it too.
func FormatVersionAtTime(t time.Time) (string, error) {
	u := t.UTC()
	if y := u.Year(); y < 0 || y > 9999 {
		return "", fmt.Errorf("%w: version_at_time year %d is outside 0000-9999, which RFC 3339 cannot write", transport.ErrInvalidConfig, y)
	}
	return u.Format(time.RFC3339Nano), nil
}

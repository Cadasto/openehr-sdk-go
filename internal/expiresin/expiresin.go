// Package expiresin reads the expires_in member of an OAuth 2.0 token
// response as whole seconds, the one way every token source in the SDK reads
// it.
package expiresin

import (
	"math"
	"strconv"
)

// Max is the longest lifetime, in seconds, that an expires_in gives an
// access token: 100 years. A larger value counts as this one, so that
// turning it into a time.Duration cannot overflow into an expiry in the
// past, which would have every call refresh the token.
const Max = 100 * 365 * 24 * 60 * 60

// Seconds reads s, the expires_in member of a token response, as whole
// seconds. RFC 6749 §5.1 makes it "the lifetime in seconds of the access
// token", a JSON number, so every form of one counts. An integer is read as
// it is. Any other number, such as 3600.0, 3.6e3, 3600.5 or an integer
// beyond the int64 range, counts as its whole seconds rounded down. Every
// form is then kept between -Max and Max, so the result always turns into a
// time.Duration of that many seconds without overflow. A negative value
// stays negative, so a caller can tell it gives no expiry. ok is false when
// s is not a number, or is one a float64 cannot hold (it reads as NaN or
// ±Inf).
func Seconds(s string) (secs int64, ok bool) {
	if secs, err := strconv.ParseInt(s, 10, 64); err == nil {
		return min(max(secs, -Max), Max), true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	// Bounding f first keeps the conversion inside the int64 range, where
	// it is defined.
	return int64(math.Floor(min(max(f, -Max), Max))), true
}

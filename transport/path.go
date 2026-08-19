package transport

import (
	"fmt"
	"strings"
)

// ValidatePathSegment checks that s is usable as a single decoded path
// parameter (REQ-150). It returns nil when s is legal, and otherwise an
// error wrapping both [ErrInvalidPathSegment] and [ErrInvalidConfig].
//
// Refused: the empty string, `.` and `..` (a segment that IS a traversal
// element — not one that merely contains a dot, so `Blood Pressure.v1`
// passes), and any segment carrying `/`, `\`, a C0 control byte, or DEL.
// A `/` is a violation here precisely because s is one segment: the
// separator would silently become path structure.
//
// Everything else is accepted, including spaces and any other
// percent-encodable octet — encoding remains the transport's job
// (REQ-095). A literal `%2e%2e` that was never decoded is an ordinary
// segment, not a separator.
//
// C1 controls (0x80–0x9F) and invalid UTF-8 are deliberately out of
// scope: they are not path-structural, and percent-encoding neutralises
// them on the way out.
func ValidatePathSegment(s string) error {
	if s == "" || s == "." || s == ".." {
		return fmt.Errorf("%w: %w: %q", ErrInvalidConfig, ErrInvalidPathSegment, s)
	}
	for _, r := range s {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7F {
			return fmt.Errorf("%w: %w: %q", ErrInvalidConfig, ErrInvalidPathSegment, s)
		}
	}
	return nil
}

// ValidateRequestPath checks a whole decoded [Request.Path] (REQ-150).
// The transport calls it before building the request URL; a caller
// assembling a raw [Request] can call it to preflight.
//
// The leading empty segment of an absolute path is ignored. A path of
// exactly "/" is the service root — it carries no segments and passes,
// which is what keeps the System API's only operation (`OPTIONS /`)
// working. Every remaining segment of any other path goes through
// [ValidatePathSegment], so a trailing slash fails as an empty segment.
//
// Splitting on `/` cannot see a separator smuggled inside a single
// parameter — those segments are individually legal. That case is caught
// by the route-arity check at the enforcement point, which needs
// [Request.Route] and so cannot live in this function.
func ValidateRequestPath(path string) error {
	if path == "/" {
		return nil
	}
	for i, seg := range strings.Split(path, "/") {
		if i == 0 && seg == "" {
			continue
		}
		if err := ValidatePathSegment(seg); err != nil {
			return err
		}
	}
	return nil
}

// segmentCount counts the `/`-separated segments of p, ignoring the
// leading empty one produced by an absolute path. The service root has
// zero segments.
func segmentCount(p string) int {
	if p == "/" {
		return 0
	}
	n := len(strings.Split(p, "/"))
	if strings.HasPrefix(p, "/") {
		n--
	}
	return n
}

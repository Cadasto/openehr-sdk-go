package transport

import (
	"fmt"
	"strings"
)

// ValidatePathSegment checks that s is usable as a single decoded path
// parameter. It returns nil when s is legal, and otherwise an
// error wrapping both [ErrInvalidPathSegment] and [ErrInvalidConfig].
//
// Refused: the empty string, `.` and `..` (a segment that is a traversal
// element; one that merely contains a dot, such as `Blood Pressure.v1`,
// passes), and any segment carrying `/`, `\`, a C0 control byte, or DEL.
// A `/` is a violation here precisely because s is one segment: the
// separator would silently become path structure.
//
// The error names the rule s breaks and the byte index where it breaks
// it, never s itself, which is usually a caller-supplied identifier.
//
// Everything else is accepted, including spaces and any other
// percent-encodable octet, because encoding remains the transport's job.
// A literal `%2e%2e` that was never decoded is an ordinary
// segment, not a separator.
//
// C1 controls (0x80–0x9F) and invalid UTF-8 are deliberately out of
// scope: they are not path-structural, and percent-encoding neutralises
// them on the way out.
func ValidatePathSegment(s string) error {
	if fault := segmentFault(s); fault != "" {
		return fmt.Errorf("%w: %w: %s", ErrInvalidConfig, ErrInvalidPathSegment, fault)
	}
	return nil
}

// segmentFault names the REQ-150 rule s breaks as one decoded path segment,
// and the byte index where, or returns "" when s is legal. It never quotes s:
// a segment is usually a caller-supplied identifier, and transport
// diagnostics stay value-free (REQ-093).
func segmentFault(s string) string {
	switch s {
	case "":
		return "empty segment"
	case ".", "..":
		return `dot segment ("." or "..")`
	}
	// idiom.md § String iteration: a check defined over bytes ranges the
	// bytes. Every byte it looks for is ASCII, so a byte of a multi-byte
	// character or of invalid UTF-8 never matches.
	for i := range len(s) {
		switch b := s[i]; {
		case b == '/':
			return fmt.Sprintf(`separator "/" at index %d`, i)
		case b == '\\':
			return fmt.Sprintf("backslash at index %d", i)
		case b < 0x20 || b == 0x7F:
			return fmt.Sprintf("control byte at index %d", i)
		}
	}
	return ""
}

// ValidateRequestPath checks a whole decoded [Request.Path].
// The transport calls it before building the request URL; a caller
// assembling a raw [Request] can call it to preflight.
//
// The leading empty segment of an absolute path is ignored; an empty path
// is refused, since it is not the service root but an alias for the bare
// service base URL. A path of exactly "/" is the service root: it carries
// no segments and passes, which keeps the System API's only operation
// (`OPTIONS /`) working. Every remaining segment of any other path is held
// to the rules of [ValidatePathSegment], so a trailing slash fails as an
// empty segment. The error counts the failing segment from 1 and names the
// rule it breaks, never the path or the segment.
//
// Splitting on `/` cannot see a separator smuggled inside a single
// parameter, because those segments are individually legal. That case is
// caught by the route-arity check at the enforcement point, which needs
// [Request.Route] and so cannot live in this function.
func ValidateRequestPath(path string) error {
	if path == "/" {
		return nil
	}
	// Only an ABSOLUTE path has a leading empty segment to skip. An empty
	// path is not the service root — it aliases to the service base URL — so
	// it still yields one empty segment, which is refused.
	rest, _ := strings.CutPrefix(path, "/")
	n := 0
	for seg := range strings.SplitSeq(rest, "/") {
		n++
		if fault := segmentFault(seg); fault != "" {
			return fmt.Errorf("%w: %w: segment %d: %s", ErrInvalidConfig, ErrInvalidPathSegment, n, fault)
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
	// Split would return Count+1 pieces; counting them needs no slice.
	n := strings.Count(p, "/") + 1
	if strings.HasPrefix(p, "/") {
		n--
	}
	return n
}

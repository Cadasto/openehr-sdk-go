package lint

import (
	"errors"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// ErrEmptyPath is returned by [Normalise] for an identified path with no
// alias root. A well-formed parse never yields one (the grammar requires a
// leading IDENTIFIER); it guards against zero-value misuse.
var ErrEmptyPath = errors.New("lint: identified path has no alias")

// Path is an alias-stripped identified path. Suffix is the canonical,
// alias-free path string (`/attr[pred]/attr...`) used to match against a
// compiled template's AQL paths in Layer 3.
type Path struct {
	// Alias is the original root binding (e.g. "o").
	Alias string
	// Segments are the path steps after the alias, copied from the source.
	Segments []parse.PathSegment
	// Suffix is the canonical alias-free path; "" for a bare alias.
	Suffix string
}

// Normalise strips the alias from an identified path and yields the canonical
// alias-free segment list and suffix string. It is purely structural — it
// does not resolve the alias or consult a template (that is Layer 3's job).
func Normalise(p parse.IdentifiedPath) (Path, error) {
	if p.Alias == "" {
		return Path{}, ErrEmptyPath
	}
	var sb strings.Builder
	for _, seg := range p.Segments {
		sb.WriteByte('/')
		sb.WriteString(seg.Name)
		if seg.Predicate != "" {
			sb.WriteByte('[')
			sb.WriteString(seg.Predicate)
			sb.WriteByte(']')
		}
	}
	return Path{Alias: p.Alias, Segments: p.Segments, Suffix: sb.String()}, nil
}

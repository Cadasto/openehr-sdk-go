package definition

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// listConfig holds resolved [ListTemplates] options. The *Set flags record
// whether the caller supplied an explicit value, so an explicit
// WithOffset(0) / WithFetch(0) is distinguishable from "unset" and reaches
// the wire instead of being silently dropped (REQ-143).
type listConfig struct {
	templateID string
	concept    string
	version    string
	offset     int
	offsetSet  bool
	fetch      int
	fetchSet   bool
}

// ListOption filters [ListTemplates] through the ITS-REST list query
// parameters of `definition_template_adl1.4_list` (REQ-143).
//
// These options carry unqualified names — WithVersion, WithOffset,
// WithFetch — where the rest of this package names options after their
// operation (WithUploadVersion, WithExampleType, WithQueryType). That is
// deliberate: REQ-143 pins the names, and ListOption is a distinct type
// from UploadOption, StoreOption, and ExampleOption, so passing an upload
// option to a list call is a compile error rather than a silent no-op. A
// second paged list endpoint in this package would need qualified names;
// the template catalog is the only one today.
type ListOption func(*listConfig)

// WithTemplateID filters the catalog by template id. The pin specifies a
// wildcard pattern (`vital*`), matched by the server, not the SDK.
func WithTemplateID(id string) ListOption {
	return func(c *listConfig) { c.templateID = id }
}

// WithConcept filters the catalog by concept name. The pin specifies a
// wildcard pattern (`*signs*`), matched by the server, not the SDK.
func WithConcept(concept string) ListOption {
	return func(c *listConfig) { c.concept = concept }
}

// WithVersion filters by the version taken from the template id (`1.2.*`,
// or `*` for all versions). When unset, the server returns only the latest
// version of each template.
//
// This is the list filter; [WithUploadVersion] is the unrelated upload
// option that pins the version of a template being stored.
func WithVersion(version string) ListOption {
	return func(c *listConfig) { c.version = version }
}

// WithOffset sets the 0-based row offset into the result set. An explicit
// WithOffset(0) is honoured and sent on the wire, not treated as unset.
//
// A negative offset is refused by [ListTemplates] with
// [transport.ErrInvalidConfig] and issues no request. That is an SDK floor,
// not a wire rule: the pin gives offset no negative semantics and declares
// no minimum. It deliberately diverges from query.WithOffset, which accepts
// a negative and forwards it — do not "align" the two without changing
// REQ-143.
func WithOffset(n int) ListOption {
	return func(c *listConfig) { c.offset = n; c.offsetSet = true }
}

// WithFetch limits the number of rows returned. An explicit WithFetch(0)
// requests zero rows; when unset the server applies its implementation-
// defined default (the pin declares none).
//
// A negative fetch is refused on the same terms as [WithOffset].
func WithFetch(n int) ListOption {
	return func(c *listConfig) { c.fetch = n; c.fetchSet = true }
}

// resolveListOptions applies opts and validates the result. It returns the
// query parameters to send; unset options contribute no key (REQ-143).
func resolveListOptions(opts []ListOption) (url.Values, error) {
	var cfg listConfig
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	// Validation lives here rather than in the option funcs: an option
	// cannot return an error, so a negative would otherwise only surface
	// as a rejected request (or worse, a forwarded one).
	if cfg.offsetSet && cfg.offset < 0 {
		return nil, fmt.Errorf("%w: offset %d is negative", transport.ErrInvalidConfig, cfg.offset)
	}
	if cfg.fetchSet && cfg.fetch < 0 {
		return nil, fmt.Errorf("%w: fetch %d is negative", transport.ErrInvalidConfig, cfg.fetch)
	}

	q := url.Values{}
	if cfg.templateID != "" {
		q.Set("template_id", cfg.templateID)
	}
	if cfg.concept != "" {
		q.Set("concept", cfg.concept)
	}
	if cfg.version != "" {
		q.Set("version", cfg.version)
	}
	if cfg.offsetSet {
		q.Set("offset", strconv.Itoa(cfg.offset))
	}
	if cfg.fetchSet {
		q.Set("fetch", strconv.Itoa(cfg.fetch))
	}
	return q, nil
}

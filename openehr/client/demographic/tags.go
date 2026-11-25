package demographic

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// routeTags is the demographic-wide ITEM_TAG listing.
const routeTags = "/demographic/tags"

// tagsTarget returns the request path and route template of a PARTY's tags
// resource, refusing a target the tags routes cannot address.
func tagsTarget(op string, t Type, target openehrclient.Ref) (path, route string, err error) {
	if !t.valid() {
		return "", "", fmt.Errorf("%s: %w: invalid PARTY type %q", op, transport.ErrInvalidConfig, t)
	}
	if err := openehrclient.ValidateRef(target); err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}
	// REQ-059: the tags routes take no version_at_time, so a Ref that
	// carries a time is refused rather than sent without it.
	if k, _ := target.Query(); k != "" {
		return "", "", fmt.Errorf("%s: %w: the tags routes take no version time", op, transport.ErrInvalidConfig)
	}
	seg := target.PathSegment()
	// The uid goes into Path raw; the transport encodes it once (REQ-150).
	return basePath(t) + "/" + seg + "/tags", basePath(t) + "/{uid_based_id}/tags", nil
}

// GetTags retrieves the ITEM_TAGs attached to a PARTY of the given type. The
// Ref names the target: [openehrclient.VersionOf](uid) for one version, or
// [openehrclient.LatestOf](voID) for the versioned party itself. A Ref that
// [openehrclient.ValidateRef] refuses, or one that carries a time
// ([openehrclient.LatestAtTime]), fails with [transport.ErrInvalidConfig]
// before any request, since the tags routes take none. A target with no
// tags returns an empty list.
//
// Wire: GET /demographic/{type}/{uid_based_id}/tags. An unknown uid is a 404,
// surfaced as [transport.ErrNotFound]. A 2xx with an empty body fails with
// [transport.ErrInvalidShape], and one whose body does not decode as a list
// with a [*transport.DecodeError].
func GetTags(ctx context.Context, c *transport.Client, t Type, target openehrclient.Ref) ([]openehrclient.StoredItemTag, error) {
	path, route, err := tagsTarget("demographic.GetTags", t, target)
	if err != nil {
		return nil, err
	}
	return getTagList(ctx, c, &transport.Request{Method: http.MethodGet, Path: path, Route: route})
}

// setTagsConfig is the resolved option set for [SetTags].
type setTagsConfig struct {
	prefer transport.Prefer
}

// SetTagsOption mutates [SetTags]'s request shape. It is its own type, not a
// [WriteOption]: a tags write commits no PARTY version, so it has no audit
// details or lifecycle state to send.
type SetTagsOption func(*setTagsConfig)

// WithTagsPrefer sets the response shape [SetTags] asks for. The default is
// [transport.PreferMinimal], answered with 204 and no list.
// [transport.PreferRepresentation] returns the list the server stored; every
// other mode returns a nil list. [transport.PreferDefault] sends no Prefer
// header.
func WithTagsPrefer(p transport.Prefer) SetTagsOption {
	return func(c *setTagsConfig) { c.prefer = p }
}

// updateItemTag is one UPDATE_ITEM_TAG in the tags PUT body.
type updateItemTag struct {
	Key        string `json:"key"`
	Value      string `json:"value,omitempty"`
	TargetPath string `json:"target_path,omitempty"`
}

// SetTags replaces the whole list of ITEM_TAGs attached to a PARTY of the
// given type, addressed as for [GetTags]. Tags on one target are told apart
// by their key and target path together. A nil or empty list removes every
// tag from the target. A tag with an empty key, or with a string that is not
// valid UTF-8, fails with [transport.ErrInvalidConfig] before any request.
//
// Wire: PUT /demographic/{type}/{uid_based_id}/tags with the list as a JSON
// array of UPDATE_ITEM_TAG (key, value, target_path) and no If-Match. The
// response follows [WithTagsPrefer]: by default a 204 and a nil list. With
// [transport.PreferRepresentation], a 2xx without a body fails with
// [transport.ErrInvalidShape], and one whose body does not decode with a
// [*transport.DecodeError]. An unknown uid is a 404, surfaced as
// [transport.ErrNotFound]; a 400 has no sentinel and surfaces as a bare
// [transport.WireError].
func SetTags(ctx context.Context, c *transport.Client, t Type, target openehrclient.Ref, tags []openehrclient.ItemTag, opts ...SetTagsOption) ([]openehrclient.StoredItemTag, error) {
	const op = "demographic.SetTags"
	path, route, err := tagsTarget(op, t, target)
	if err != nil {
		return nil, err
	}
	cfg := setTagsConfig{prefer: transport.PreferMinimal}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	// REQ-059: the whole list goes out every time, and an empty one as `[]`,
	// which clears the target's tags.
	wire := make([]updateItemTag, 0, len(tags))
	for i, tag := range tags {
		if tag.Key == "" {
			return nil, fmt.Errorf("%s: %w: item tag[%d]: empty key", op, transport.ErrInvalidConfig, i)
		}
		wire = append(wire, updateItemTag{Key: tag.Key, Value: tag.Value, TargetPath: tag.TargetPath})
	}
	body, err := json.Marshal(wire)
	if err != nil {
		// The encoder's error names no tag value, only where it failed.
		return nil, fmt.Errorf("%s: %w: tags body: %w", op, transport.ErrInvalidConfig, err)
	}
	req := &transport.Request{
		Method: http.MethodPut,
		Path:   path,
		Route:  route,
		Body:   body,
		Prefer: cfg.prefer,
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if cfg.prefer != transport.PreferRepresentation {
		return nil, nil
	}
	// REQ-094: return=representation is never downgraded to "no list".
	if transport.IsNoRepresentationBody(resp.Body) {
		return nil, fmt.Errorf("%s: %w: %d response with empty or null body", op, transport.ErrInvalidShape, resp.StatusCode)
	}
	var out []openehrclient.StoredItemTag
	if err := canjson.Unmarshal(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("%s: ITEM_TAG list: %w", op, &transport.DecodeError{
			Method: req.Method, Route: req.Route, Body: resp.Body, Inner: err,
		})
	}
	return out, nil
}

// DeleteTag deletes the ITEM_TAGs with the given key from a PARTY of the
// given type, addressed as for [GetTags]: every tag with that key, whatever
// its target path. An empty key fails with [transport.ErrInvalidConfig], and a
// key that is not one path segment (one holding `/`, or `.` or `..`) with
// [transport.ErrInvalidPathSegment] as well, before any request.
//
// Wire: DELETE /demographic/{type}/{uid_based_id}/tags/{key}, with the key
// percent-encoded once. The server answers 204; an unknown uid or key is a
// 404, surfaced as [transport.ErrNotFound].
func DeleteTag(ctx context.Context, c *transport.Client, t Type, target openehrclient.Ref, key string) error {
	const op = "demographic.DeleteTag"
	path, route, err := tagsTarget(op, t, target)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("%s: %w: empty key", op, transport.ErrInvalidConfig)
	}
	// The key goes into Path raw; the transport encodes it once and refuses
	// one that is not a single segment (REQ-150).
	_, err = c.Do(ctx, &transport.Request{
		Method: http.MethodDelete,
		Path:   path + "/" + key,
		Route:  route + "/{key}",
	})
	return err
}

// listTagsConfig is the resolved option set for [ListTags].
type listTagsConfig struct {
	key, value, targetPath string
}

// ListTagsOption narrows the tags [ListTags] returns.
type ListTagsOption func(*listTagsConfig)

// WithTagKey keeps the tags with this key. Empty sends no filter.
func WithTagKey(key string) ListTagsOption {
	return func(c *listTagsConfig) { c.key = key }
}

// WithTagValue keeps the tags with this value. Empty sends no filter.
func WithTagValue(value string) ListTagsOption {
	return func(c *listTagsConfig) { c.value = value }
}

// WithTagTargetPath keeps the tags with this target path. Empty sends no
// filter.
func WithTagTargetPath(path string) ListTagsOption {
	return func(c *listTagsConfig) { c.targetPath = path }
}

// ListTags retrieves the ITEM_TAGs attached to any PARTY version or versioned
// party in the demographic service, narrowed by the options given; with none
// it returns every tag. A tag's Target.Type names the class of the party it
// is attached to.
//
// Wire: GET /demographic/tags [?tag_key=...&tag_value=...&tag_target_path=...].
// A 400 has no sentinel and surfaces as a bare [transport.WireError]. A 2xx
// with an empty body fails with [transport.ErrInvalidShape], and one whose
// body does not decode as a list with a [*transport.DecodeError].
func ListTags(ctx context.Context, c *transport.Client, opts ...ListTagsOption) ([]openehrclient.StoredItemTag, error) {
	var cfg listTagsConfig
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	req := &transport.Request{Method: http.MethodGet, Path: routeTags, Route: routeTags}
	q := url.Values{}
	if cfg.key != "" {
		q.Set("tag_key", cfg.key)
	}
	if cfg.value != "" {
		q.Set("tag_value", cfg.value)
	}
	if cfg.targetPath != "" {
		q.Set("tag_target_path", cfg.targetPath)
	}
	if len(q) > 0 {
		req.Query = q
	}
	return getTagList(ctx, c, req)
}

// getTagList issues a read whose 2xx body is an ITEM_TAG list.
func getTagList(ctx context.Context, c *transport.Client, req *transport.Request) ([]openehrclient.StoredItemTag, error) {
	out, _, err := transport.Decode[[]openehrclient.StoredItemTag](ctx, c, req)
	if err != nil {
		return nil, err
	}
	return *out, nil
}

func (r *repository) GetTags(ctx context.Context, t Type, target openehrclient.Ref) ([]openehrclient.StoredItemTag, error) {
	return GetTags(ctx, r.c, t, target)
}

func (r *repository) SetTags(ctx context.Context, t Type, target openehrclient.Ref, tags []openehrclient.ItemTag, opts ...SetTagsOption) ([]openehrclient.StoredItemTag, error) {
	return SetTags(ctx, r.c, t, target, tags, opts...)
}

func (r *repository) DeleteTag(ctx context.Context, t Type, target openehrclient.Ref, key string) error {
	return DeleteTag(ctx, r.c, t, target, key)
}

func (r *repository) ListTags(ctx context.Context, opts ...ListTagsOption) ([]openehrclient.StoredItemTag, error) {
	return ListTags(ctx, r.c, opts...)
}

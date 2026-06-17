package care

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func (c *Client) partyCodec() PartyCodec {
	if c.party != nil {
		return c.party
	}
	if pc, ok := c.codec.(PartyCodec); ok {
		return pc
	}
	return nil
}

// partyTypeFromOPT maps an OPT root RM type to the Demographic REST segment.
func partyTypeFromOPT(opt *template.OperationalTemplate) (demographic.Type, error) {
	if opt == nil {
		return "", errors.New("care: nil template")
	}
	return partyTypeFromRM(opt.Root().RMTypeName())
}

func partyTypeFromRM(rmType string) (demographic.Type, error) {
	switch rmType {
	case "PERSON":
		return demographic.Person, nil
	case "ORGANISATION":
		return demographic.Organisation, nil
	case "GROUP":
		return demographic.Group, nil
	case "AGENT":
		return demographic.Agent, nil
	case "ROLE":
		return demographic.Role, nil
	default:
		return "", fmt.Errorf("care: template root %q is not a demographic PARTY type", rmType)
	}
}

func versionUIDFromMeta(meta *openehrclient.VersionMetadata) string {
	if meta == nil {
		return ""
	}
	if meta.VersionUID != "" {
		return string(meta.VersionUID)
	}
	if meta.Metadata != nil && meta.ETag != "" {
		return cleanVersionUID(meta.ETag)
	}
	return ""
}

// SavePartyData writes a datamap payload for a demographic template: fetch the
// OPT, encode via PartyCodec, and POST the PARTY to the Demographics API.
// Returns the new party version uid.
func (c *Client) SavePartyData(ctx context.Context, templateID string, datamap map[string]any) (string, error) {
	pc := c.partyCodec()
	if pc == nil {
		return "", errors.New("care: no PartyCodec configured")
	}
	opt, err := c.resolveOPT(ctx, templateID)
	if err != nil {
		return "", err
	}
	partyMap, err := pc.ToParty(opt, datamap)
	if err != nil {
		return "", fmt.Errorf("care: encode party: %w", err)
	}
	return c.SavePartyRaw(ctx, partyMap)
}

// UpdatePartyData mirrors SavePartyData but writes via PUT against an existing
// versioned PARTY family. voID is the versioned-object uuid; ifMatch is the
// full preceding version uid (ETag).
func (c *Client) UpdatePartyData(ctx context.Context, voID, ifMatch, templateID string, datamap map[string]any) (string, error) {
	pc := c.partyCodec()
	if pc == nil {
		return "", errors.New("care: no PartyCodec configured")
	}
	opt, err := c.resolveOPT(ctx, templateID)
	if err != nil {
		return "", err
	}
	partyMap, err := pc.ToParty(opt, datamap)
	if err != nil {
		return "", fmt.Errorf("care: encode party: %w", err)
	}
	t, err := partyTypeFromOPT(opt)
	if err != nil {
		return "", err
	}
	return c.UpdatePartyRaw(ctx, t, voID, ifMatch, partyMap)
}

// GetPartyData reads a stored PARTY back as a datamap payload.
func (c *Client) GetPartyData(ctx context.Context, partyType demographic.Type, voID, templateID string) (map[string]any, error) {
	pc := c.partyCodec()
	if pc == nil {
		return nil, errors.New("care: no PartyCodec configured")
	}
	partyMap, err := c.GetPartyRaw(ctx, partyType, voID)
	if err != nil {
		return nil, err
	}
	opt, err := c.resolveOPT(ctx, templateID)
	if err != nil {
		return nil, err
	}
	dm, err := pc.FromParty(opt, partyMap)
	if err != nil {
		return nil, fmt.Errorf("care: decode party datamap: %w", err)
	}
	return dm, nil
}

// PartyETag GETs the latest PARTY version for voID and returns the ETag
// (full version uid) suitable as If-Match on update/delete.
func (c *Client) PartyETag(ctx context.Context, partyType demographic.Type, voID string) (string, error) {
	_, meta, err := c.getPartyWithRefFallback(ctx, partyType, voID)
	if err != nil {
		return "", fmt.Errorf("care: get party etag %s: %w", voID, err)
	}
	etag := versionUIDFromMeta(meta)
	if etag == "" {
		return "", fmt.Errorf("care: party %s: empty etag", voID)
	}
	return etag, nil
}

// SavePartyRaw POSTs a canonical PARTY map via the Demographics API.
func (c *Client) SavePartyRaw(ctx context.Context, partyMap map[string]any) (string, error) {
	party, err := partyFromMap(partyMap)
	if err != nil {
		return "", err
	}
	_, meta, err := demographic.Create(ctx, c.rest, party, demographic.WithPrefer(transport.PreferMinimal))
	if err != nil {
		return "", fmt.Errorf("care: create party: %w", err)
	}
	if uid := versionUIDFromMeta(meta); uid != "" {
		return uid, nil
	}
	return "", nil
}

// UpdatePartyRaw PUTs a canonical PARTY map against voID with If-Match.
func (c *Client) UpdatePartyRaw(ctx context.Context, partyType demographic.Type, voID, ifMatch string, partyMap map[string]any) (string, error) {
	party, err := partyFromMap(partyMap)
	if err != nil {
		return "", err
	}
	_, meta, err := demographic.Update(ctx, c.rest, partyType, openehrclient.VersionedObjectID(versionedObjectID(voID)), ifMatch, party, demographic.WithPrefer(transport.PreferMinimal))
	if err != nil {
		return "", fmt.Errorf("care: update party: %w", err)
	}
	if uid := versionUIDFromMeta(meta); uid != "" {
		return uid, nil
	}
	return "", nil
}

// GetPartyRaw retrieves the latest PARTY for voID as canonical JSON.
func (c *Client) GetPartyRaw(ctx context.Context, partyType demographic.Type, voID string) (map[string]any, error) {
	party, _, err := c.getPartyWithRefFallback(ctx, partyType, voID)
	if err != nil {
		return nil, fmt.Errorf("care: get party %s: %w", voID, err)
	}
	if party == nil {
		return nil, transport.ErrNotFound
	}
	return partyToMap(party)
}

// getPartyWithRefFallback tries demographic GET refs per ITS-REST uid_based_id,
// then the VERSIONED_PARTY read family when typed GETs miss:
//   - OBJECT_VERSION_ID (uuid::system::n) on GET /demographic/{type}/{uid}
//   - bare VERSIONED_OBJECT uuid for latest on the same path
//   - GET /demographic/versioned_party/{vo_uid}/version[/{version_uid}]
//
// 400/404 on one id shape are retried — Cadasto rejects uuid::system on the
// typed person path with 400 while the bare uuid or versioned_party path works.
func (c *Client) getPartyWithRefFallback(ctx context.Context, partyType demographic.Type, voID string) (rm.Party, *openehrclient.VersionMetadata, error) {
	var attempts []string
	var lastErr error
	record := func(label string, err error) {
		attempts = append(attempts, label+" -> "+attemptStatus(err))
		if err != nil {
			lastErr = err
		}
	}

	for _, ref := range partyGetRefs(voID) {
		label := "GET /demographic/" + string(partyType) + "/" + ref.PathSegment()
		party, meta, err := demographic.Get(ctx, c.rest, partyType, ref)
		if err == nil && party != nil {
			return party, meta, nil
		}
		if err != nil && !partyGetRetriable(err) {
			return nil, meta, err
		}
		record(label, err)
	}

	uid := cleanVersionUID(voID)
	for _, void := range voidUIDCandidates(uid) {
		if isFullVersionUID(uid) {
			label := "GET /demographic/versioned_party/" + void + "/version/" + uid
			party, meta, err := demographic.GetVersionedParty(ctx, c.rest, void, uid)
			if err == nil && party != nil {
				return party, meta, nil
			}
			if err != nil && !partyGetRetriable(err) {
				return nil, meta, err
			}
			record(label, err)
		}
		label := "GET /demographic/versioned_party/" + void + "/version"
		party, meta, err := demographic.GetVersionedParty(ctx, c.rest, void, "")
		if err == nil && party != nil {
			return party, meta, nil
		}
		if err != nil && !partyGetRetriable(err) {
			return nil, meta, err
		}
		record(label, err)
	}

	if lastErr != nil {
		if errors.Is(lastErr, transport.ErrNotFound) {
			return nil, nil, fmt.Errorf("%w (tried: %s). %s", lastErr, strings.Join(attempts, "; "), partyRESTMissingHint)
		}
		return nil, nil, fmt.Errorf("%w (tried: %s)", lastErr, strings.Join(attempts, "; "))
	}
	return nil, nil, transport.ErrNotFound
}

// partyRESTMissingHint explains the common AQL-vs-REST mismatch on Cadasto.
const partyRESTMissingHint = "no demographics REST resource at any id shape — AQL FROM PERSON often lists subject/index refs (e.g. from EHR_STATUS external_ref) that were never POSTed to /demographic/person; create via Person.v2 save or open a uid returned from POST Location/ETag"

func partyGetRefs(voID string) []openehrclient.Ref {
	uid := cleanVersionUID(voID)
	var refs []openehrclient.Ref
	if isFullVersionUID(uid) {
		refs = append(refs, openehrclient.VersionOf(openehrclient.VersionUID(uid)))
	}
	for _, seg := range partyUIDCandidates(uid) {
		if isFullVersionUID(seg) {
			continue
		}
		refs = append(refs, openehrclient.LatestOf(openehrclient.VersionedObjectID(seg)))
	}
	return refs
}

// partyUIDCandidates returns uid_based_id segments for LatestOf on the typed
// PARTY path. Cadasto documents HIER_OBJECT_ID there as the bare
// VERSIONED_OBJECT uuid only — not uuid::creating_system.
func partyUIDCandidates(uid string) []string {
	if void := versionedObjectID(cleanVersionUID(uid)); void != "" {
		return []string{void}
	}
	return nil
}

// voidUIDCandidates returns the VERSIONED_OBJECT id to try on versioned_party
// reads. Cadasto's {versioned_object_uid} path segment MUST be the bare object
// UUID — a uuid::creating_system value is rejected with 400 "This is not a
// valid UUID", which would otherwise mask the real 404 from the valid shapes.
func voidUIDCandidates(uid string) []string {
	if void := versionedObjectID(cleanVersionUID(uid)); void != "" {
		return []string{void}
	}
	return nil
}

// partyGetRetriable reports whether a failed PARTY read should fall through to
// the next id shape. Cadasto may answer 400 (malformed uid_based_id) or 404.
// attemptStatus renders a fallback attempt's outcome for the "tried:" trace:
// "ok", an HTTP status code, "404" for a not-found sentinel, or "err". Keeps
// each shape's individual result visible instead of only the last error.
func attemptStatus(err error) string {
	if err == nil {
		return "ok"
	}
	var we *transport.WireError
	if errors.As(err, &we) {
		return strconv.Itoa(we.StatusCode)
	}
	if errors.Is(err, transport.ErrNotFound) {
		return "404"
	}
	return "err"
}

func partyGetRetriable(err error) bool {
	if errors.Is(err, transport.ErrNotFound) {
		return true
	}
	var we *transport.WireError
	if errors.As(err, &we) && (we.StatusCode == 400 || we.StatusCode == 404) {
		return true
	}
	return false
}

func isFullVersionUID(uid string) bool {
	parts := strings.Split(uid, "::")
	return len(parts) >= 3 && parts[0] != "" && parts[len(parts)-1] != ""
}

// DeleteParty logically deletes the PARTY version addressed by versionUID.
func (c *Client) DeleteParty(ctx context.Context, partyType demographic.Type, versionUID, ifMatch string) error {
	_, err := demographic.Delete(ctx, c.rest, partyType, openehrclient.VersionUID(cleanVersionUID(versionUID)), ifMatch)
	if err != nil {
		return fmt.Errorf("care: delete party %s: %w", versionUID, err)
	}
	return nil
}

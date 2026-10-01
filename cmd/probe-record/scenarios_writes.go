package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// cassetteSourceTemplateID is the EHRbase-origin fixture the composition
// scenario rewrites. The replacement id is a fixed constant: a fresh
// random id in a path would miss on replay, because the key is method
// and stripped path (REQ-082).
const (
	cassetteSourceTemplateID = "terminology_test.ehrbase.org.v1"
	cassetteTemplateID       = "sdk_cassette_minimal.v1"
	cassetteStoredQueryName  = "org.cadasto.sdk::cassette_stored"
	cassetteStoredQueryAQL   = "SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = $target_ehr"
)

// cassetteEHRStatusJSON is a fixed EHR_STATUS. is_modifiable is true so
// the recorded EHR is not locked. The name is a constant, not a per-run id.
const cassetteEHRStatusJSON = `{"_type":"EHR_STATUS","name":{"_type":"DV_TEXT","value":"cassette EHR status"},"archetype_node_id":"openEHR-EHR-EHR_STATUS.generic.v1","subject":{"_type":"PARTY_SELF"},"is_queryable":true,"is_modifiable":true}`

// captureEHRStatus records POST /ehr with an initial status and no client
// id, then GET /ehr/{id}/ehr_status of the id the create returned.
func captureEHRStatus(ctx context.Context, c *transport.Client) error {
	var st rm.EHRStatus
	if err := canjson.Unmarshal([]byte(cassetteEHRStatusJSON), &st); err != nil {
		return fmt.Errorf("decode EHR status: %w", err)
	}
	rec, _, err := ehr.Create(ctx, c, ehr.WithInitialStatus(&st))
	if err != nil {
		return fmt.Errorf("create EHR: %w", err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		return errors.New("create EHR returned no ehr_id")
	}
	if _, _, err := ehrstatus.Get(ctx, c, ehr.EHRID(rec.EHRID.Value)); err != nil {
		return fmt.Errorf("get EHR status: %w", err)
	}
	return nil
}

// captureCompositionMinimal records a server-assigned EHR create, an OPT
// upload under one fixed template id, a minimal composition save, and a
// GET of the version uid that save returned.
func captureCompositionMinimal(ctx context.Context, c *transport.Client) error {
	rec, _, err := ehr.Create(ctx, c)
	if err != nil {
		return fmt.Errorf("create EHR: %w", err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		return errors.New("create EHR returned no ehr_id")
	}
	opt, comp, err := cassetteMinimalComposition()
	if err != nil {
		return err
	}
	if _, _, err := definition.UploadTemplate(ctx, c, definition.FormatADL14, bytes.NewReader(opt)); err != nil {
		return fmt.Errorf("upload template: %w", err)
	}
	_, meta, err := composition.Save(ctx, c, ehr.EHRID(rec.EHRID.Value), comp, composition.WithTemplateID(cassetteTemplateID))
	if err != nil {
		return fmt.Errorf("save composition: %w", err)
	}
	if meta == nil || meta.VersionUID == "" {
		return errors.New("save composition returned no version uid")
	}
	if _, _, err := composition.Get(ctx, c, ehr.EHRID(rec.EHRID.Value), ehr.VersionOf(meta.VersionUID)); err != nil {
		return fmt.Errorf("get composition: %w", err)
	}
	return nil
}

// captureStoredQuery records a server-assigned EHR create, a PUT of one
// fixed stored query, and the stored execution that follows it. The
// qualified name is a constant. The EHR id in the query parameter comes
// from the create response, so it is not a fresh path segment.
func captureStoredQuery(ctx context.Context, c *transport.Client) error {
	rec, _, err := ehr.Create(ctx, c)
	if err != nil {
		return fmt.Errorf("create EHR: %w", err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		return errors.New("create EHR returned no ehr_id")
	}
	if _, _, err := definition.PutStoredQuery(ctx, c, cassetteStoredQueryName, cassetteStoredQueryAQL); err != nil {
		return fmt.Errorf("put stored query: %w", err)
	}
	rs, _, err := query.RunStored(ctx, c, cassetteStoredQueryName, map[string]any{"target_ehr": rec.EHRID.Value}, query.WithFetch(100))
	if err != nil {
		return fmt.Errorf("run stored query: %w", err)
	}
	if rs == nil {
		return errors.New("stored query returned a nil result set")
	}
	return nil
}

// cassetteMinimalComposition rewrites the terminology_test OPT and
// canonical composition to cassetteTemplateID. The replacement must not
// leave the source id in place, and the source id must not be a
// substring of the replacement or the check cannot tell them apart.
func cassetteMinimalComposition() ([]byte, *rm.Composition, error) {
	src := []byte(cassetteSourceTemplateID)
	dst := []byte(cassetteTemplateID)
	if bytes.Contains(dst, src) {
		return nil, nil, fmt.Errorf("cassette template id %s contains the source id", cassetteTemplateID)
	}
	opt, err := os.ReadFile(fixtures.TemplateOpt(cassetteSourceTemplateID))
	if err != nil {
		return nil, nil, fmt.Errorf("read OPT: %w", err)
	}
	raw, err := os.ReadFile(fixtures.CompositionJSON(cassetteSourceTemplateID))
	if err != nil {
		return nil, nil, fmt.Errorf("read composition: %w", err)
	}
	opt = bytes.ReplaceAll(opt, src, dst)
	raw = bytes.ReplaceAll(raw, src, dst)
	if bytes.Contains(opt, src) || !bytes.Contains(opt, dst) {
		return nil, nil, fmt.Errorf("OPT does not carry the fixed template id %s", cassetteTemplateID)
	}
	if bytes.Contains(raw, src) || !bytes.Contains(raw, dst) {
		return nil, nil, fmt.Errorf("composition fixture does not carry the fixed template id %s", cassetteTemplateID)
	}
	comp := &rm.Composition{}
	if err := canjson.Unmarshal(raw, comp); err != nil {
		return nil, nil, fmt.Errorf("decode composition fixture: %w", err)
	}
	return opt, comp, nil
}

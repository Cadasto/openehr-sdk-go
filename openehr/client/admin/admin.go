package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// DeleteEHR issues an admin-mode delete of the EHR identified by id.
//
// Wire: DELETE /admin/ehr/{ehr_id}. A 404 surfaces as
// transport.ErrNotFound; callers that treat absence as success can
// errors.Is against it without unwrapping the *WireError envelope.
func DeleteEHR(ctx context.Context, c *transport.Client, id ehr.EHRID) error {
	if id == "" {
		return fmt.Errorf("admin.DeleteEHR: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	_, err := c.Do(ctx, &transport.Request{
		Method: http.MethodDelete,
		Path:   "/admin/ehr/" + string(id),
		Route:  "/admin/ehr/{ehr_id}",
	})
	return err
}

// DeleteAllEHRs wipes EHRs on the deployment. Deployment policy decides
// whether it is allowed: a deployment that disables it returns 405 Method
// Not Allowed per the admin contract (some tenants instead return
// 403/404). With no ids it resets every EHR; passing one or more ids
// restricts the delete to that subset via the repeatable ehr_id query
// parameter.
//
// Wire: DELETE /admin/ehr/all{?ehr_id*}, with the literal /all segment
// the ITS-REST admin contract defines. A 4xx response surfaces as the
// typed *transport.WireError envelope (caller can errors.Is for the typed
// sentinels); any 2xx, including 202 Accepted (async) and 204 No Content
// (sync), is treated as success. The upstream Admin API has x-status:
// DEVELOPMENT, so this surface is a draft and may change between minor
// versions.
func DeleteAllEHRs(ctx context.Context, c *transport.Client, ids ...ehr.EHRID) error {
	req := &transport.Request{
		Method: http.MethodDelete,
		Path:   "/admin/ehr/all",
		Route:  "/admin/ehr/all",
	}
	if len(ids) > 0 {
		q := make(url.Values, 1)
		for _, id := range ids {
			q.Add("ehr_id", string(id))
		}
		req.Query = q
	}
	_, err := c.Do(ctx, req)
	return err
}

// PurgeTemplates clears the template registry on the deployment. Used by
// integration test suites that need a clean slate between scenarios.
//
// This is not part of the openEHR ITS-REST Admin contract (which defines
// only /admin/ehr/{ehr_id} and /admin/ehr/all). It is an EHRbase-specific
// extension: EHRbase exposes "delete all templates" as
// DELETE /admin/template/all (operationId deleteAllTemplates). See
// https://docs.ehrbase.org/docs/EHRbase/Explore/Admin-REST.
// Against a non-EHRbase deployment the endpoint is absent and a 404
// surfaces as transport.ErrNotFound (safe to ignore). The admin base
// path is deployment-specific; this assumes the same base as the other
// /admin/* calls.
func PurgeTemplates(ctx context.Context, c *transport.Client) error {
	_, err := c.Do(ctx, &transport.Request{
		Method: http.MethodDelete,
		Path:   "/admin/template/all",
		Route:  "/admin/template/all",
	})
	return err
}

// Repository mirrors the package-level admin functions as a method set
// bound to a single *transport.Client. Useful as a dependency-injection
// seam in integration test suites.
type Repository interface {
	DeleteEHR(ctx context.Context, id ehr.EHRID) error
	DeleteAllEHRs(ctx context.Context, ids ...ehr.EHRID) error
	PurgeTemplates(ctx context.Context) error
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) DeleteEHR(ctx context.Context, id ehr.EHRID) error {
	return DeleteEHR(ctx, r.c, id)
}

func (r *repository) DeleteAllEHRs(ctx context.Context, ids ...ehr.EHRID) error {
	return DeleteAllEHRs(ctx, r.c, ids...)
}

func (r *repository) PurgeTemplates(ctx context.Context) error {
	return PurgeTemplates(ctx, r.c)
}

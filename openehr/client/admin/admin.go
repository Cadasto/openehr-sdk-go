package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// DeleteEHR issues an admin-mode delete of the EHR identified by id.
//
// Wire: DELETE /admin/ehr/{ehr_id}. A 404 surfaces as
// transport.ErrNotFound (idempotent — absence is not an error and
// callers may choose to treat it as success).
func DeleteEHR(ctx context.Context, c *transport.Client, id ehr.EHRID) error {
	if id == "" {
		return fmt.Errorf("admin.DeleteEHR: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	_, err := c.Do(ctx, &transport.Request{
		Method: http.MethodDelete,
		Path:   "/admin/ehr/" + url.PathEscape(string(id)),
		Route:  "/admin/ehr/{ehr_id}",
	})
	if err != nil {
		// Preserve the typed sentinel so callers can errors.Is
		// without unwrapping the *WireError envelope.
		if errors.Is(err, transport.ErrNotFound) {
			return err
		}
		return err
	}
	return nil
}

// DeleteAllEHRs wipes every EHR on the deployment. Gated by deployment
// policy — many production tenants disable this surface entirely and
// return 403/404.
//
// Wire: DELETE /admin/ehr. A 4xx response surfaces as the typed
// *transport.WireError envelope (caller can errors.Is for the typed
// sentinels). 2xx — including 204 — is treated as success.
func DeleteAllEHRs(ctx context.Context, c *transport.Client) error {
	_, err := c.Do(ctx, &transport.Request{
		Method: http.MethodDelete,
		Path:   "/admin/ehr",
		Route:  "/admin/ehr",
	})
	return err
}

// PurgeTemplates clears the template registry on the deployment. Used
// by integration test suites that need a clean slate between scenarios.
//
// Wire: DELETE /admin/template. A 404 surfaces as transport.ErrNotFound
// (deployments that do not expose this endpoint) and is safe to ignore.
func PurgeTemplates(ctx context.Context, c *transport.Client) error {
	_, err := c.Do(ctx, &transport.Request{
		Method: http.MethodDelete,
		Path:   "/admin/template",
		Route:  "/admin/template",
	})
	return err
}

// Repository mirrors the package-level admin functions as a method set
// bound to a single *transport.Client. Useful for dependency-injection
// seams (REQ-023) in integration test suites.
type Repository interface {
	DeleteEHR(ctx context.Context, id ehr.EHRID) error
	DeleteAllEHRs(ctx context.Context) error
	PurgeTemplates(ctx context.Context) error
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) DeleteEHR(ctx context.Context, id ehr.EHRID) error {
	return DeleteEHR(ctx, r.c, id)
}

func (r *repository) DeleteAllEHRs(ctx context.Context) error {
	return DeleteAllEHRs(ctx, r.c)
}

func (r *repository) PurgeTemplates(ctx context.Context) error {
	return PurgeTemplates(ctx, r.c)
}

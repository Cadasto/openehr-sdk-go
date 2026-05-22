// Package discovery resolves the SMART-on-openEHR service catalog:
// fetches the SMART configuration document, validates that required
// services (org.openehr.rest, Cadasto extras when present) are
// advertised, and exposes a cached, refresh-able ServiceCatalog to the
// typed clients.
//
// Discovery is a first-class step: SDK constructors take a
// ServiceCatalog, not a single base URL. For non-discovering openEHR
// backends (e.g. a static EHRbase deployment), callers inject a
// hand-built catalog.
//
// Implements REQ-070, REQ-071, REQ-072, and REQ-092 per
// docs/specifications/service-discovery.md and docs/specifications/transport.md.
package discovery

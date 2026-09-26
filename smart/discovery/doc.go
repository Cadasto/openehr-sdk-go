// Package discovery resolves the SMART-on-openEHR service catalog:
// fetches the SMART configuration document, validates that required
// services (org.openehr.rest, Cadasto extras when present) are
// advertised, and exposes a cached, refresh-able ServiceCatalog to the
// typed clients.
//
// SDK constructors take a ServiceCatalog instead of a single base URL.
// For openEHR backends that do not publish a discovery document (e.g. a
// static EHRbase deployment), callers inject a hand-built catalog.
package discovery

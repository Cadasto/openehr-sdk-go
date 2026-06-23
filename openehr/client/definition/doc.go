// Package definition is the openEHR REST 1.1.0-development Definition
// API client. Phase 6 of the REST API client plan lands ADL 1.4
// templates (Operational Templates — OPT XML). ADL 2 source-form
// templates and stored AQL queries follow in later commits (the
// latter is gated on the AQL builder design in `openehr/aql/`).
//
// Endpoints implemented:
//
//   - POST   /definition/template/adl1.4
//   - GET    /definition/template/adl1.4
//   - GET    /definition/template/adl1.4/{template_id}
//   - GET    /definition/template/adl1.4/{template_id}/example
//   - DELETE /definition/template/adl1.4/{template_id}   (where supported)
//
// Templates are stored on the deployment as XML; the SDK transports
// raw bytes so consumers can supply OPTs verbatim from .opt files or
// from an in-memory build. Example COMPOSITION responses are decoded
// via the canjson codec into *rm.Composition.
package definition

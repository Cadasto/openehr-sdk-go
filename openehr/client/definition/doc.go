// Package definition is the openEHR REST 1.1.0-development Definition
// API client. It covers ADL 1.4 templates (Operational Templates — OPT
// XML) and stored AQL queries. ADL 2 source-form templates are not
// implemented.
//
// Endpoints implemented:
//
//   - POST   /definition/template/adl1.4
//   - GET    /definition/template/adl1.4
//   - GET    /definition/template/adl1.4/{template_id}
//   - GET    /definition/template/adl1.4/{template_id}/example
//   - DELETE /definition/template/adl1.4/{template_id}   (where supported)
//   - PUT    /definition/query/{qualified_query_name}
//   - PUT    /definition/query/{qualified_query_name}/{version}
//   - GET    /definition/query/{qualified_query_name}
//   - GET    /definition/query/{qualified_query_name}/{version}
//   - DELETE /definition/query/{qualified_query_name}/{version}   (where supported)
//
// Templates are stored on the deployment as XML; the SDK transports
// raw bytes so consumers can supply OPTs verbatim from .opt files or
// from an in-memory build. Example COMPOSITION responses are decoded
// via the canjson codec into *rm.Composition.
package definition

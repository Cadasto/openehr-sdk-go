// Package template parses ADL 1.4 operational templates (OPT): XML
// OPERATIONAL_TEMPLATE artifacts, typically with the .opt filename suffix.
//
// v1 scope is OPT parse and openEHR path utilities only. Authoring-time
// templates (.oet) are out of scope. The primary parsed type is
// OperationalTemplate (not "Template", which is ambiguous with OET).
//
// Package name template aligns with the openEHR REST Definition API
// "template" resource; this package operates on deployment artifacts
// locally without HTTP. Upload to a CDR uses openehr/client/definition.
package template

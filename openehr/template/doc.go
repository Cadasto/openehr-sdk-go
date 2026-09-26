// Package template parses ADL 1.4 operational templates (OPT) into
// an in-memory model with openEHR path utilities.
//
// An OPT is an XML artifact with root element <template> in namespace
// http://schemas.openehr.org/v1 (the Ocean Template Designer XSD form),
// typically with the .opt filename suffix. The primary parsed type is
// [OperationalTemplate]; the definition-tree nodes implement the sealed
// [Node] interface.
//
// The package covers OPT parsing and path resolution against the parsed
// tree. Authoring-time templates (.oet) are out of scope. ADL 2
// operational templates, archetype-slot linkage against a remote
// repository, and terminology expansion are not supported.
//
// In openEHR terminology, "template" without qualification often means
// the authoring OET; in this SDK, "template" means operational
// template (OPT) unless stated otherwise. The package name template
// aligns with the openEHR REST Definition API "template" resource;
// this package operates on deployment artifacts locally without HTTP.
// Upload to a CDR uses openehr/client/definition.
//
// This package is stdlib-only. It does not depend on transport/, auth/,
// openehr/client/*, openehr/rm/, or openehr/aom/aom14/.
package template

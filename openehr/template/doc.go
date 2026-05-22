// Package template parses ADL 1.4 operational templates (OPT) into
// an in-memory model with openEHR path utilities.
//
// An OPT is an XML artifact with root element <template> in namespace
// http://schemas.openehr.org/v1 (the Ocean Template Designer XSD form),
// typically with the .opt filename suffix. The primary parsed type is
// [OperationalTemplate]; the definition-tree nodes implement the sealed
// [Node] interface.
//
// v1 scope is OPT parse + path resolution against the parsed tree.
// Authoring-time templates (.oet) are out of scope. ADL 2 operational
// templates, archetype-slot linkage against a remote repository, and
// terminology expansion are deferred.
//
// In openEHR terminology, "template" without qualification often means
// the authoring OET; in this SDK v1, "template" means operational
// template (OPT) unless stated otherwise. The package name template
// aligns with the openEHR REST Definition API "template" resource;
// this package operates on deployment artifacts locally without HTTP.
// Upload to a CDR uses openehr/client/definition.
//
// Building-block use: this package depends only on the standard library
// and openehr/rm/. It does not depend on transport/, auth/, or
// openehr/client/*. See docs/specifications/clinical-modeling.md
// REQ-100 for the normative contract.
package template

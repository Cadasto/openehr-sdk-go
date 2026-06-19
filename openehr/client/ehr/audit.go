package ehr

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// MarshalAuditDetails encodes an openEHR AUDIT_DETAILS into the
// `openehr-audit-details` request-header value defined by openEHR REST
// 1.1.0-development (REQ-059): a comma-separated list of dotted-attribute
// assignments — e.g.
//
//	change_type.code_string="249",committer.name="Alice",committer.external_ref.id="…",system_id="…"
//
// It is NOT a JSON object. Canonical-JSON / canonical-XML serialisation of
// AUDIT_DETAILS applies only to the contribution request *body* (the
// commit_audit / UpdateAudit field, REQ-057), never to this header.
//
// The grammar and worked examples are normative in the upstream contract —
// resources/its-rest/overview-validation.openapi.yaml, the
// "openehr-version and openehr-audit-details" section. Per REQ-095 that
// OpenAPI contract is authoritative.
//
// Returns "" for a nil input. Values containing control characters are
// rejected (header-injection guard, as for openehr-item-tag).
func MarshalAuditDetails(a *rm.AuditDetails) (string, error) {
	if a == nil {
		return "", nil
	}

	var attrs []string
	add := func(key, val string) error {
		if val == "" {
			return nil
		}
		if hasCtrlChars(val) {
			return fmt.Errorf("ehr: audit detail %s contains control characters", key)
		}
		attrs = append(attrs, key+`="`+escapeItemTagValue(val)+`"`)
		return nil
	}

	if err := add("change_type.code_string", a.ChangeType.DefiningCode.CodeString); err != nil {
		return "", err
	}
	if a.Description != nil {
		if err := add("description.value", a.Description.GetValue()); err != nil {
			return "", err
		}
	}
	name, ext := committerParts(a.Committer)
	if err := add("committer.name", name); err != nil {
		return "", err
	}
	if ext != nil {
		if err := add("committer.external_ref.id", objectIDValue(ext.ID)); err != nil {
			return "", err
		}
		if err := add("committer.external_ref.namespace", ext.Namespace); err != nil {
			return "", err
		}
		if err := add("committer.external_ref.type", ext.Type); err != nil {
			return "", err
		}
	}
	if err := add("system_id", a.SystemID); err != nil {
		return "", err
	}

	return strings.Join(attrs, ","), nil
}

// committerParts extracts the human-readable name and optional external
// reference from a PARTY_PROXY committer, handling both value and pointer
// forms of PARTY_IDENTIFIED / PARTY_RELATED / PARTY_SELF. PARTY_SELF carries
// no name. A nil or unrecognised proxy yields ("", nil).
func committerParts(p rm.PartyProxy) (name string, ext *rm.PartyRef) {
	switch v := p.(type) {
	case rm.PartyIdentified:
		return derefString(v.Name), v.ExternalRef
	case *rm.PartyIdentified:
		return derefString(v.Name), v.ExternalRef
	case rm.PartyRelated:
		return derefString(v.Name), v.ExternalRef
	case *rm.PartyRelated:
		return derefString(v.Name), v.ExternalRef
	case rm.PartySelf:
		return "", v.ExternalRef
	case *rm.PartySelf:
		return "", v.ExternalRef
	default:
		return "", nil
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// objectIDValue returns the raw string value of any concrete ObjectID.
func objectIDValue(id rm.ObjectID) string {
	switch v := id.(type) {
	case rm.HierObjectID:
		return v.Value
	case rm.ObjectVersionID:
		return v.Value
	case rm.GenericID:
		return v.Value
	case rm.ArchetypeID:
		return v.Value
	case rm.TemplateID:
		return v.Value
	case rm.TerminologyID:
		return v.Value
	default:
		return ""
	}
}

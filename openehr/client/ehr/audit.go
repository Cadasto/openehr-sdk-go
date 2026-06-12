package ehr

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// MarshalAuditDetails serialises an openEHR AUDIT_DETAILS to the
// canonical-JSON string used for the openehr-audit-details header;
// returns "" for a nil input.
func MarshalAuditDetails(a *rm.AuditDetails) (string, error) {
	if a == nil {
		return "", nil
	}
	b, err := canjson.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("marshal audit details: %w", err)
	}
	return string(b), nil
}

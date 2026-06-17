package datamap

import (
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// partyRMTypes are openEHR Demographic RM roots that datamap Option B supports
// via ToParty / FromParty (REQ-058 demographics profile).
var partyRMTypes = map[string]bool{
	"PERSON":             true,
	"ORGANISATION":       true,
	"AGENT":              true,
	"GROUP":              true,
	"ROLE":               true,
	"PARTY_IDENTITY":     true,
	"PARTY_RELATIONSHIP": true,
	"CONTACT":            true,
	"ADDRESS":            true,
}

// actorRMTypes are PARTY subtypes that inherit from Actor (people, orgs, devices, groups).
var actorRMTypes = map[string]bool{
	"PERSON":       true,
	"ORGANISATION": true,
	"AGENT":        true,
	"GROUP":        true,
}

// IsPartyTemplate reports whether opt roots a demographic PARTY (or nested
// demographic archetype such as ADDRESS) rather than a clinical COMPOSITION.
func IsPartyTemplate(opt *template.OperationalTemplate) bool {
	if opt == nil {
		return false
	}
	return partyRMTypes[opt.Root().RMTypeName()]
}

// IsActorParty reports whether rmType is one of the Actor PARTY subtypes.
func IsActorParty(rmType string) bool {
	return actorRMTypes[strings.ToUpper(rmType)]
}

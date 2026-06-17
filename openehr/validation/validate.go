package validation

import (
	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// Validate validates an in-memory archetypeable RM root against a
// compiled OPT and returns every issue in one pass — REQ-110.
//
// It is the generic form of [ValidateComposition]: the compiled OPT
// drives the lockstep walk and `root` is the value source. `root` must
// be one of the RM LOCATABLE concretes the walker recognises — the
// COMPOSITION content closed set (REQ-102) plus the demographic PARTY
// hierarchy (PERSON / ORGANISATION / GROUP / AGENT / ROLE and the
// archetypeable sub-components ADDRESS / CONTACT / PARTY_IDENTITY /
// PARTY_RELATIONSHIP / CAPABILITY) and the EHR-IM roots FOLDER /
// EHR_STATUS. A root whose concrete RM type does not match the OPT root
// surfaces as an rm_type_mismatch at "/", not a silent pass.
//
// Returns a [Result] whose Issues slice is never nil. A nil root or nil
// compiled template yields a single guard issue (nil_root /
// nil_template).
func Validate(root any, c *templatecompile.Compiled) Result {
	if root == nil || rmread.IsTypedNilPointer(root) {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_root",
			Detail:   "Validate called with a nil RM root argument",
			Severity: Error,
		}})
	}
	if c == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_template",
			Detail:   "validation called with a nil compiled template argument",
			Severity: Error,
		}})
	}
	if c.Root() == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_template",
			Detail:   "compiled template has no root node",
			Severity: Error,
		}})
	}

	w := newWalker(c)
	w.walkNode(c.Root(), root, "/")
	return resultFromIssues(w.issues)
}

// ValidateDemographic validates an in-memory demographic PARTY (PERSON,
// ORGANISATION, GROUP, AGENT or ROLE) against a compiled OPT — REQ-110.
// It guards the nil party (yielding nil_party), then delegates to
// [Validate]. The PARTY sub-components (ADDRESS, CONTACT, PARTY_IDENTITY,
// PARTY_RELATIONSHIP, CAPABILITY) are validated in place as the walk
// descends, or as roots in their own right via [Validate].
func ValidateDemographic(party rm.Party, c *templatecompile.Compiled) Result {
	if party == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_party",
			Detail:   "ValidateDemographic called with a nil rm.Party argument",
			Severity: Error,
		}})
	}
	return Validate(party, c)
}

// ValidateFolder validates an in-memory FOLDER (a directory tree root or
// sub-folder) against a compiled OPT — REQ-110.
func ValidateFolder(folder *rm.Folder, c *templatecompile.Compiled) Result {
	if folder == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_folder",
			Detail:   "ValidateFolder called with a nil *rm.Folder argument",
			Severity: Error,
		}})
	}
	return Validate(folder, c)
}

// ValidateEHRStatus validates an in-memory EHR_STATUS against a compiled
// OPT — REQ-110.
func ValidateEHRStatus(status *rm.EHRStatus, c *templatecompile.Compiled) Result {
	if status == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_ehr_status",
			Detail:   "ValidateEHRStatus called with a nil *rm.EHRStatus argument",
			Severity: Error,
		}})
	}
	return Validate(status, c)
}
